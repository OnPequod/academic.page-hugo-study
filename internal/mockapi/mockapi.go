// Package mockapi serves fixture data at the same paths and in the same
// shapes as the Academic.page Content API, so the Hugo site can be built and
// tested without a real key.
//
// It mirrors the behavior a frontend depends on: bearer-token auth, the
// Accept header, pagination with links.next, the slug, featured, status, and
// updated_since filters, section_not_enabled for sections a site type does
// not have, ETag and Last-Modified validators, and the BibTeX export. It does
// not enforce rate limits.
package mockapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// collections are the list endpoints a site can have, keyed by path segment.
// Sections are gated by the site's "sections" list; tags and media are not.
var collections = []string{"posts", "pages", "projects", "talks", "publications", "courses", "lab-members", "tags", "media"}

var gated = map[string]bool{"posts": true, "pages": true, "projects": true, "talks": true, "publications": true, "courses": true, "lab-members": true}

// Server serves the fixtures in a directory laid out as sites/<id>/*.json.
type Server struct {
	dir   string
	token string
	sites map[int]*site
	ids   []int
}

type site struct {
	id       int
	raw      []byte // site.json, with placeholders
	record   map[string]any
	sections map[string]bool
	files    map[string][]byte // name -> raw file, such as "posts.json"
}

// New loads every site under dir/sites. token is the only key the server
// accepts.
func New(dir, token string) (*Server, error) {
	s := &Server{dir: dir, token: token, sites: map[int]*site{}}
	entries, err := os.ReadDir(filepath.Join(dir, "sites"))
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		id, err := strconv.Atoi(e.Name())
		if err != nil || !e.IsDir() {
			continue
		}
		st := &site{id: id, files: map[string][]byte{}, sections: map[string]bool{}}
		files, err := os.ReadDir(filepath.Join(dir, "sites", e.Name()))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			b, err := os.ReadFile(filepath.Join(dir, "sites", e.Name(), f.Name()))
			if err != nil {
				return nil, err
			}
			st.files[f.Name()] = b
		}
		st.raw = st.files["site.json"]
		if st.raw == nil {
			return nil, fmt.Errorf("site %d has no site.json", id)
		}
		if err := json.Unmarshal(st.raw, &st.record); err != nil {
			return nil, fmt.Errorf("site %d: %w", id, err)
		}
		for _, sec := range st.record["sections"].([]any) {
			st.sections[sec.(string)] = true
		}
		s.sites[id] = st
		s.ids = append(s.ids, id)
	}
	sort.Ints(s.ids)
	return s, nil
}

const prefix = "/api/v1/content/sites"

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/storage/") {
		serveAsset(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "The content API is read-only.")
		return
	}
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+s.token {
		writeError(w, http.StatusUnauthorized, "missing_api_token", "Send a valid API key as a bearer token.")
		return
	}
	isBib := strings.HasSuffix(r.URL.Path, "/publications.bib")
	if !isBib && !strings.Contains(r.Header.Get("Accept"), "application/json") {
		writeError(w, http.StatusNotAcceptable, "invalid_parameter", "Send Accept: application/json.")
		return
	}

	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, prefix), "/")
	parts := []string{}
	if rest != "" {
		parts = strings.Split(rest, "/")
	}
	if len(parts) == 0 {
		s.listSites(w, r)
		return
	}
	id, err := strconv.Atoi(parts[0])
	st := s.sites[id]
	if err != nil || st == nil {
		writeError(w, http.StatusNotFound, "site_not_found", "No site with that ID for this key.")
		return
	}
	base := baseURL(r)
	switch {
	case len(parts) == 1:
		var rec map[string]any
		_ = json.Unmarshal(fill(st.raw, base), &rec)
		rec["content_version"] = contentVersion(st)
		s.respond(w, r, st, map[string]any{"data": rec})
	case len(parts) == 2 && parts[1] == "manifest":
		s.respond(w, r, st, map[string]any{"data": s.manifest(st)})
	case len(parts) == 2 && parts[1] == "profile":
		// A course site's homepage is the course, so it has no profile.
		if st.record["site_type"] == "course" {
			writeError(w, http.StatusNotFound, "section_not_enabled", "Course sites have no profile.")
			return
		}
		s.single(w, r, st, "profile.json")
	case len(parts) == 2 && parts[1] == "study":
		if st.record["site_type"] != "study" {
			writeError(w, http.StatusNotFound, "section_not_enabled", "Only study sites have a study.")
			return
		}
		s.single(w, r, st, "study.json")
	case len(parts) == 2 && parts[1] == "publications.bib":
		if !st.sections["publications"] {
			sectionNotEnabled(w, "publications")
			return
		}
		w.Header().Set("Content-Type", "application/x-bibtex; charset=UTF-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s-publications.bib"`, st.record["slug"]))
		w.Header().Set("Last-Modified", lastModified(st))
		_, _ = w.Write(st.files["publications.bib"])
	case len(parts) == 2 && isCollection(parts[1]):
		if gated[parts[1]] && !st.sections[parts[1]] {
			sectionNotEnabled(w, parts[1])
			return
		}
		s.list(w, r, st, parts[1])
	case len(parts) == 3 && isCollection(parts[1]):
		if gated[parts[1]] && !st.sections[parts[1]] {
			sectionNotEnabled(w, parts[1])
			return
		}
		rid, err := strconv.Atoi(parts[2])
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "Not found.")
			return
		}
		for _, rec := range s.records(st, parts[1], base) {
			if int(rec["id"].(float64)) == rid {
				s.respond(w, r, st, map[string]any{"data": rec})
				return
			}
		}
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
	default:
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
	}
}

func isCollection(name string) bool {
	for _, c := range collections {
		if c == name {
			return true
		}
	}
	return false
}

func (s *Server) listSites(w http.ResponseWriter, r *http.Request) {
	base := baseURL(r)
	summaryKeys := []string{"id", "name", "slug", "site_type", "domain", "url", "custom_domain", "is_published", "published_at", "content_version", "content_updated_at", "api_url"}
	var all []map[string]any
	for _, id := range s.ids {
		st := s.sites[id]
		var rec map[string]any
		_ = json.Unmarshal(fill(st.raw, base), &rec)
		rec["content_version"] = contentVersion(st)
		sum := map[string]any{}
		for _, k := range summaryKeys {
			sum[k] = rec[k]
		}
		all = append(all, sum)
	}
	s.paginate(w, r, nil, all)
}

func (s *Server) single(w http.ResponseWriter, r *http.Request, st *site, file string) {
	var rec any
	if err := json.Unmarshal(fill(st.files[file], baseURL(r)), &rec); err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	s.respond(w, r, st, map[string]any{"data": rec})
}

func (s *Server) records(st *site, name, base string) []map[string]any {
	var recs []map[string]any
	_ = json.Unmarshal(fill(st.files[name+".json"], base), &recs)
	return recs
}

func (s *Server) list(w http.ResponseWriter, r *http.Request, st *site, name string) {
	q := r.URL.Query()
	recs := s.records(st, name, baseURL(r))
	keep := recs[:0]
	var since time.Time
	if v := q.Get("updated_since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "invalid_parameter", "updated_since must be an ISO 8601 date-time.")
			return
		}
		since = t
	}
	featured := q.Get("featured")
	if featured != "" && name != "talks" && name != "publications" {
		featured = ""
	}
	for _, rec := range recs {
		if v := q.Get("slug"); v != "" && rec["slug"] != v {
			continue
		}
		if featured != "" {
			key := "is_featured"
			want := featured == "true" || featured == "1"
			if b, _ := rec[key].(bool); b != want {
				continue
			}
		}
		if v := q.Get("status"); v != "" && name == "lab-members" && rec["status"] != v {
			continue
		}
		if !since.IsZero() && !changedSince(rec, since) {
			continue
		}
		keep = append(keep, rec)
	}
	s.paginate(w, r, st, keep)
}

func changedSince(rec map[string]any, since time.Time) bool {
	for _, k := range []string{"updated_at", "published_at"} {
		if v, ok := rec[k].(string); ok {
			if t, err := time.Parse(time.RFC3339, v); err == nil && !t.Before(since) {
				return true
			}
		}
	}
	return false
}

// paginate writes one page of records with Laravel-style links and meta.
func (s *Server) paginate(w http.ResponseWriter, r *http.Request, st *site, recs []map[string]any) {
	q := r.URL.Query()
	perPage, page := 50, 1
	if v := q.Get("per_page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			writeError(w, http.StatusUnprocessableEntity, "invalid_parameter", "per_page must be between 1 and 100.")
			return
		}
		perPage = n
	}
	if v := q.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusUnprocessableEntity, "invalid_parameter", "page must be 1 or more.")
			return
		}
		page = n
	}
	total := len(recs)
	last := int(math.Max(1, math.Ceil(float64(total)/float64(perPage))))
	start := (page - 1) * perPage
	end := start + perPage
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	data := recs[start:end]
	if data == nil {
		data = []map[string]any{}
	}

	path := baseURL(r) + strings.TrimPrefix(r.URL.Path, "/api/v1")
	link := func(n int) string {
		qq := url.Values{}
		for k, v := range q {
			if k != "page" {
				qq[k] = v
			}
		}
		enc := qq.Encode()
		if enc != "" {
			enc += "&"
		}
		return fmt.Sprintf("%s?%spage=%d", path, enc, n)
	}
	var prev, next any
	if page > 1 {
		prev = link(page - 1)
	}
	if page < last {
		next = link(page + 1)
	}
	var from, to any
	if len(data) > 0 {
		from, to = start+1, end
	}
	body := map[string]any{
		"data":  data,
		"links": map[string]any{"first": link(1), "last": link(last), "prev": prev, "next": next},
		"meta":  map[string]any{"current_page": page, "last_page": last, "per_page": perPage, "total": total, "from": from, "to": to, "path": path},
	}
	s.respond(w, r, st, body)
}

func (s *Server) manifest(st *site) map[string]any {
	cols := map[string]any{}
	for _, name := range collections {
		if gated[name] && !st.sections[name] {
			continue
		}
		var out []map[string]any
		for _, rec := range s.records(st, name, "") {
			out = append(out, map[string]any{
				"id": rec["id"], "slug": rec["slug"], "path": rec["path"],
				"updated_at": rec["updated_at"], "published_at": rec["published_at"],
			})
		}
		if out == nil {
			out = []map[string]any{}
		}
		cols[name] = out
	}
	return map[string]any{
		"site_id":            st.id,
		"content_version":    contentVersion(st),
		"content_updated_at": st.record["content_updated_at"],
		"collections":        cols,
	}
}

// respond writes JSON with ETag and Last-Modified, answering a matching
// If-None-Match with 304.
func (s *Server) respond(w http.ResponseWriter, r *http.Request, st *site, body any) {
	b, err := json.Marshal(body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	sum := sha256.Sum256(b)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Vary", "Authorization, Accept-Encoding")
	if st != nil {
		w.Header().Set("Last-Modified", lastModified(st))
	}
	w.Header().Set("X-RateLimit-Limit", "600")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": message, "code": code})
}

func sectionNotEnabled(w http.ResponseWriter, section string) {
	writeError(w, http.StatusNotFound, "section_not_enabled", "This site does not use "+strings.ReplaceAll(section, "-", " ")+".")
}

func contentVersion(st *site) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%v", st.id, st.record["content_updated_at"])))
	return hex.EncodeToString(sum[:10])
}

func lastModified(st *site) string {
	t, err := time.Parse(time.RFC3339, fmt.Sprint(st.record["content_updated_at"]))
	if err != nil {
		t = time.Now()
	}
	return t.UTC().Format(http.TimeFormat)
}

func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/api/v1"
}

// fill replaces the fixture placeholders with this server's address.
func fill(b []byte, apiBase string) []byte {
	assets := strings.TrimSuffix(apiBase, "/api/v1") + "/storage"
	out := strings.ReplaceAll(string(b), "{{API}}", apiBase)
	out = strings.ReplaceAll(out, "{{ASSETS}}", assets)
	return []byte(out)
}

// serveAsset stands in for the media CDN: images come back as labeled SVG
// placeholders and PDFs as a one-page document.
func serveAsset(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Path)
	switch strings.ToLower(filepath.Ext(name)) {
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
		label := strings.TrimSuffix(name, ".svg")
		fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="675" viewBox="0 0 1200 675"><rect width="1200" height="675" fill="#e7ecf3"/><path d="M0 675 L400 300 L700 520 L900 380 L1200 675Z" fill="#c8d3e3"/><circle cx="950" cy="170" r="70" fill="#d6dfeb"/><text x="600" y="350" font-family="sans-serif" font-size="40" text-anchor="middle" fill="#41536b">%s</text></svg>`, htmlEscape(label))
	case ".pdf":
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte(minimalPDF))
	case ".ico", ".png":
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

const minimalPDF = "%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n"
