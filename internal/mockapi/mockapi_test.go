package mockapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func get(t *testing.T, h http.Handler, path string, headers map[string]string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

var auth = map[string]string{"Authorization": "Bearer k", "Accept": "application/json"}

func TestMock(t *testing.T) {
	srv, err := New("../../fixtures", "k")
	if err != nil {
		t.Fatal(err)
	}

	if rec, body := get(t, srv, "/api/v1/content/sites/42", map[string]string{"Accept": "application/json"}); rec.Code != 401 || body["code"] != "missing_api_token" {
		t.Errorf("no key: got %d %v", rec.Code, body)
	}
	if rec, _ := get(t, srv, "/api/v1/content/sites/42", map[string]string{"Authorization": "Bearer k"}); rec.Code != 406 {
		t.Errorf("no Accept header: got %d", rec.Code)
	}
	if rec, body := get(t, srv, "/api/v1/content/sites/42/courses", auth); rec.Code != 404 || body["code"] != "section_not_enabled" {
		t.Errorf("personal site courses: got %d %v", rec.Code, body)
	}
	if rec, body := get(t, srv, "/api/v1/content/sites/42/study", auth); rec.Code != 404 || body["code"] != "section_not_enabled" {
		t.Errorf("personal site study: got %d %v", rec.Code, body)
	}
	if rec, body := get(t, srv, "/api/v1/content/sites/44/profile", auth); rec.Code != 404 || body["code"] != "section_not_enabled" {
		t.Errorf("course site profile: got %d %v", rec.Code, body)
	}
	if rec, body := get(t, srv, "/api/v1/content/sites/7", auth); rec.Code != 404 || body["code"] != "site_not_found" {
		t.Errorf("unknown site: got %d %v", rec.Code, body)
	}
	if rec, _ := get(t, srv, "/api/v1/content/sites/42/posts?per_page=101", auth); rec.Code != 422 {
		t.Errorf("per_page over 100: got %d", rec.Code)
	}

	rec, body := get(t, srv, "/api/v1/content/sites/42/posts?per_page=3&page=2", auth)
	if rec.Code != 200 {
		t.Fatalf("posts page 2: %d", rec.Code)
	}
	links := body["links"].(map[string]any)
	if next := links["next"].(string); !strings.HasSuffix(next, "/api/v1/content/sites/42/posts?per_page=3&page=3") {
		t.Errorf("links.next = %s", next)
	}
	if meta := body["meta"].(map[string]any); meta["total"] != float64(7) || meta["last_page"] != float64(3) {
		t.Errorf("meta = %v", meta)
	}
	_, body = get(t, srv, "/api/v1/content/sites/42/posts?per_page=3&page=3", auth)
	if body["links"].(map[string]any)["next"] != nil {
		t.Errorf("last page has a next link")
	}

	_, body = get(t, srv, "/api/v1/content/sites/42/talks?featured=true", auth)
	if n := len(body["data"].([]any)); n != 2 {
		t.Errorf("featured talks = %d, want 2", n)
	}
	_, body = get(t, srv, "/api/v1/content/sites/43/lab-members?status=alumni", auth)
	if n := len(body["data"].([]any)); n != 1 {
		t.Errorf("alumni = %d, want 1", n)
	}
	_, body = get(t, srv, "/api/v1/content/sites/42/posts/1002", auth)
	if body["data"].(map[string]any)["slug"] != "multiple-imputation-chained-equations" {
		t.Errorf("single post: %v", body)
	}

	// Placeholders become this server's address.
	_, body = get(t, srv, "/api/v1/content/sites/42/profile", auth)
	photo := body["data"].(map[string]any)["photo"].(map[string]any)["url"].(string)
	if !strings.HasPrefix(photo, "http://example.com/storage/") {
		t.Errorf("photo url = %s", photo)
	}

	// ETag round trip.
	rec, _ = get(t, srv, "/api/v1/content/sites/42/tags", auth)
	etag := rec.Header().Get("ETag")
	h := map[string]string{"If-None-Match": etag}
	for k, v := range auth {
		h[k] = v
	}
	if rec, _ := get(t, srv, "/api/v1/content/sites/42/tags", h); rec.Code != 304 {
		t.Errorf("If-None-Match: got %d", rec.Code)
	}
}
