// Package tests builds the Hugo site against the mock API and checks the
// output: every section's pages exist for each site type, rich text renders
// in both modes, pagination follows links.next, and the API key never lands
// in the generated files or Hugo's cache.
//
// Run with `make test`, which builds the pinned Hugo first. Set HUGO to use
// another binary.
package tests

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/OnPequod/academic.page-hugo/internal/mockapi"
)

const testKey = "test-key-5f0c2a9e7d" // distinctive, so a leak is easy to find

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(wd)
}

func hugoBin(t *testing.T) string {
	t.Helper()
	if h := os.Getenv("HUGO"); h != "" {
		return h
	}
	h := filepath.Join(repoRoot(t), "bin", "hugo")
	if _, err := os.Stat(h); err != nil {
		t.Fatalf("no Hugo binary at %s; run `make test`, which installs the pinned version", h)
	}
	return h
}

// recorder wraps the mock and remembers each request's path and query.
type recorder struct {
	h    http.Handler
	mu   sync.Mutex
	seen []string
}

func (r *recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	r.seen = append(r.seen, req.URL.RequestURI())
	r.mu.Unlock()
	r.h.ServeHTTP(w, req)
}

func (r *recorder) requested(sub string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.seen {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func startMock(t *testing.T) (*httptest.Server, *recorder) {
	t.Helper()
	srv, err := mockapi.New(filepath.Join(repoRoot(t), "fixtures"), testKey)
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{h: srv}
	ts := httptest.NewServer(rec)
	t.Cleanup(ts.Close)
	return ts, rec
}

// build runs `hugo build` for one site into a temporary directory and
// returns that directory and Hugo's cache directory.
func build(t *testing.T, apiURL, site string, extraEnv ...string) (string, string) {
	t.Helper()
	out := t.TempDir()
	cache := t.TempDir()
	cmd := exec.Command(hugoBin(t), "build", "--environment", "demo", "--destination", out, "--cacheDir", cache, "--quiet")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(),
		"ACADEMIC_PAGE_BASE_URL="+apiURL+"/api/v1",
		"ACADEMIC_PAGE_TOKEN="+testKey,
		"ACADEMIC_PAGE_SITE="+site,
		"HUGO_BASEURL=https://example.org/",
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hugo build for site %s failed: %v\n%s", site, err, b)
	}
	if strings.Contains(string(b), testKey) {
		t.Fatalf("hugo printed the API key")
	}
	return out, cache
}

func read(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatalf("missing %s: %v", rel, err)
	}
	return string(b)
}

func mustContain(t *testing.T, dir, rel string, wants ...string) {
	t.Helper()
	s := read(t, dir, rel)
	for _, w := range wants {
		if !strings.Contains(s, w) {
			t.Errorf("%s: want %q", rel, w)
		}
	}
}

// mustAppearInOrder checks that each string appears after the one before it.
func mustAppearInOrder(t *testing.T, dir, rel string, wants ...string) {
	t.Helper()
	body := read(t, dir, rel)
	at := 0
	for _, want := range wants {
		i := strings.Index(body[at:], want)
		if i < 0 {
			t.Errorf("%s: %q missing or out of order", rel, want)
			return
		}
		at += i + len(want)
	}
}

func mustNotContain(t *testing.T, dir, rel string, unwanted ...string) {
	t.Helper()
	body := read(t, dir, rel)
	for _, u := range unwanted {
		if strings.Contains(body, u) {
			t.Errorf("%s: unexpected %q", rel, u)
		}
	}
}

func mustNotExist(t *testing.T, dir string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		if _, err := os.Stat(filepath.Join(dir, rel)); err == nil {
			t.Errorf("%s exists, but this site type does not have that section", rel)
		}
	}
}

// noKeyIn fails if the key appears in any file under dir.
func noKeyIn(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := os.ReadFile(p)
			if err == nil && strings.Contains(string(b), testKey) {
				t.Errorf("API key found in %s", p)
			}
			return nil
		})
	}
}

func TestPersonalSite(t *testing.T) {
	ts, _ := startMock(t)
	out, cache := build(t, ts.URL, "42")
	noKeyIn(t, out, cache)

	// Home page from the profile, navigation from the site settings.
	mustContain(t, out, "index.html",
		"<h1>Imani Castellanos</h1>",
		"Associate Professor of Biostatistics",
		`<strong>causal inference</strong>`,
		`castellanos-portrait-thumb.svg`,
		`<a href="/research/">Research</a>`,
		`<a href="/research/methods/">Methods</a>`,
		`href="https://github.com/example" target="_blank" rel="noopener">GitHub</a>`,
		"Selected publications",
		"Upcoming talks",
		"Target trial emulation with messy EHR data",
		"Content licensed CC BY 4.0 unless noted.",
		"--accent: #1d4ed8",
		`<link rel="alternate" type="application/rss+xml"`,
	)
	// About and CV pages from the profile.
	mustContain(t, out, "about/index.html", "<h2>About me</h2>", "<h2>Appointments</h2>", "<h2>Education</h2>", "magna cum laude")
	mustContain(t, out, "cv/index.html", "castellanos-cv.pdf")

	// Posts: rich text from the API's HTML, tags, featured image, pagination.
	mustContain(t, out, "posts/why-we-preregister-ehr-analyses/index.html",
		"<h2>What goes in the plan</h2>",
		"<strong>preregister</strong>",
		"<em>missing</em>",
		"<blockquote><p>A plan written after the results is a description of the results.</p></blockquote>",
		`<figcaption>The causal diagram we commit to before looking at outcomes.</figcaption>`,
		`<a href="/tags/causal-inference/">Causal inference</a>`,
		`dag-confounding-large.svg`,
	)
	mustContain(t, out, "posts/multiple-imputation-chained-equations/index.html",
		`<pre><code class="language-r">library(mice)`, "imp &lt;- mice(dat")
	mustContain(t, out, "posts/index.html", "Page 1 of 2", "Older posts")
	mustContain(t, out, "posts/page/2/index.html", "Page 2 of 2", "Hierarchical models for small-area rates")
	rss := read(t, out, "posts/index.xml")
	if n := strings.Count(rss, "<item>"); n != 7 {
		t.Errorf("RSS has %d items, want 7", n)
	}
	if !strings.Contains(rss, "<title>Posts | Imani Castellanos</title>") || !strings.Contains(rss, "&lt;h2&gt;What goes in the plan&lt;/h2&gt;") {
		t.Errorf("RSS is missing the channel title or escaped post HTML")
	}

	// Pages keep their nested paths.
	mustContain(t, out, "research/index.html", "<h1>Research</h1>", `href="/research/methods/"`)
	mustContain(t, out, "research/methods/index.html", "<h1>Methods we use</h1>")
	mustContain(t, out, "teaching/index.html", "<h1>Teaching</h1>")

	// Projects.
	mustContain(t, out, "projects/index.html", "Target Trial Emulation Toolkit", "toolkit-cover-thumb.svg")
	mustContain(t, out, "projects/target-trial-toolkit/index.html",
		`<span class="badge">Active</span>`, "September 2024 – present", "<h2>Gallery</h2>", "toolkit-output-medium.svg",
		`<a href="https://github.com/example/ttetools">Repository</a>`)

	mustNotContain(t, out, "projects/target-trial-toolkit/index.html", "Technologies:")

	// Publications: grouped by year, typed, with citations, DOIs, and BibTeX.
	pubs := read(t, out, "publications/index.html")
	for _, w := range []string{`<h2 id="year-2026">2026</h2>`, `<h2 id="year-2019">2019</h2>`, "Journal article", "Preprint", "Conference paper", "Book chapter", "Software", "Report", "Thesis",
		"<i>American Journal of Epidemiology</i>", `<a href="https://doi.org/10.5555/aje.2026.0142">DOI</a>`, `href="/publications/publications.bib"`} {
		if !strings.Contains(pubs, w) {
			t.Errorf("publications/index.html: want %q", w)
		}
	}
	if strings.Index(pubs, "year-2026") > strings.Index(pubs, "year-2019\">") {
		t.Errorf("publication years are not newest first")
	}
	mustContain(t, out, "publications/publications.bib", "@article{castellanos2026emulating,", "@phdthesis{")
	mustContain(t, out, "publications/emulating-trials-ehr-delays/index.html",
		"<h2>Abstract</h2>", "DOI: 10.5555/aje.2026.0142", `<a href="https://github.com/example/delay-emulation">Code</a>`, "emulating-trials-ehr-delays.csl.json")
	// CSL-JSON keeps its case-sensitive keys.
	mustContain(t, out, "publications/emulating-trials-ehr-delays.csl.json", `"DOI": "10.5555/aje.2026.0142"`, `"container-title"`)
	mustContain(t, out, "publications/dissertation/index.html", "<em>Prof. Helen Ward</em>")

	// Talks: upcoming and past, with links and files.
	mustContain(t, out, "talks/index.html", "<h2>Upcoming</h2>", "<h2>Past</h2>", "Invited talk", "Poster")
	mustContain(t, out, "talks/jsm-2026-delay-emulation/index.html", "with Daniel Reyes", `>Recording</a>`, `>Slides</a>`)
	mustContain(t, out, "talks/missing-stage-seminar/index.html", "missing-stage-slides.pdf")
	// Past talks keep the API's order, including the hand order within 2025,
	// and a talk known only by its month shows the month.
	mustAppearInOrder(t, out, "talks/index.html",
		"<h2>Past</h2>", "jsm-2026-delay-emulation", "missing-stage-seminar", "renv-workshop", "small-area-poster")
	mustContain(t, out, "talks/small-area-poster/index.html", `<time datetime="2025-11-01">November 2025</time>`)
	// Favicons under the API's keys.
	mustContain(t, out, "index.html", `<link rel="shortcut icon" href="`, `/favicons/42/favicon.ico">`, `sizes="96x96"`)

	// Tags: names and descriptions from the API, content grouped by section.
	mustContain(t, out, "tags/index.html", "Causal inference", "Survival analysis")
	mustContain(t, out, "tags/causal-inference/index.html",
		"<h1>Causal inference</h1>", "Methods for estimating effects from observational data.", "<h2>Posts</h2>", "<h2>Publications</h2>", "<h2>Talks</h2>", "<h2>Projects</h2>", "<h2>Pages</h2>")

	mustNotExist(t, out, "courses/index.html", "members/index.html", "team/index.html")
}

func TestLabSite(t *testing.T) {
	ts, _ := startMock(t)
	out, cache := build(t, ts.URL, "43")
	noKeyIn(t, out, cache)
	mustContain(t, out, "index.html", "<h1>Castellanos Lab</h1>", "<h2>About the lab</h2>", `<a href="/members/">Members</a>`, `<a href="/posts/">News</a>`)
	mustContain(t, out, "members/index.html",
		"<h1>Members</h1>", "<h2>Principal investigators</h2>", "<h2>Postdoctoral fellows</h2>", "<h2>Graduate students</h2>", "<h2>Alumni</h2>",
		`id="daniel-reyes"`, "reyes-thumb.svg", "Interests:</strong> missing data, registries", "Until 2024")
	// The section title follows the site's own menu label.
	mustContain(t, out, "posts/index.html", "<h1>News</h1>")
	mustContain(t, out, "join/index.html", "<h1>Join the lab</h1>")
	mustContain(t, out, "talks/lab-methods-workshop/index.html", "Daniel Reyes, with Imani Castellanos")
	mustNotExist(t, out, "courses/index.html", "about/index.html", "cv/index.html")
}

func TestCourseSite(t *testing.T) {
	ts, _ := startMock(t)
	out, cache := build(t, ts.URL, "44")
	noKeyIn(t, out, cache)
	mustContain(t, out, "index.html", "BIOS 621: Applied Survival Analysis", `<span class="badge">Current</span> Fall 2026`)
	mustContain(t, out, "teaching/applied-survival-analysis/index.html",
		`<h2 id="offerings">Offerings</h2>`, "BIOS 621, Fall 2026, Northbridge University", "bios621-syllabus-fall-2026.pdf",
		"<h3>Lesson 1: Time, censoring, and the Kaplan-Meier estimator</h3>", "lecture-01.pdf",
		"<cite>Survival Analysis: A Self-Learning Text</cite>", "(Required: chapters 1 and 2)",
		`<a href="#assignment-problem-set-1">Problem set 1: Kaplan-Meier by hand</a>, due September 17, 2026`,
		"Thursday, September 17, 2026, 11:59 PM EDT", "<code>cox.zph()</code>",
		`<h2 id="materials">Course materials</h2>`, `<h2 id="texts-resources">References</h2>`)
	// The course lives under /teaching, as the API's paths say, and the
	// section that lists it is there too.
	mustContain(t, out, "teaching/index.html", "applied-survival-analysis")
	// Menu items for Academic.page's built-in course pages go to the course
	// page. A group heading is a button. The instructor page has no data in
	// the API, so its item is left out.
	mustContain(t, out, "index.html",
		`<a href="/teaching/applied-survival-analysis/#syllabus">Syllabus</a>`,
		`<button type="button" class="menu-heading">Course details</button>`,
		`<a href="/teaching/applied-survival-analysis/#lessons">Lessons</a>`,
		`<a href="/teaching/applied-survival-analysis/#assignments">Assignments</a>`)
	mustNotContain(t, out, "index.html", "About the instructor")
	mustContain(t, out, "office-hours/index.html", "<li><p>Tuesdays, 2 to 4 p.m., Room 412.</p></li>")
	mustNotExist(t, out, "publications/index.html", "talks/index.html", "projects/index.html", "members/index.html")
}

func TestStudySite(t *testing.T) {
	ts, _ := startMock(t)
	out, cache := build(t, ts.URL, "45")
	noKeyIn(t, out, cache)
	mustContain(t, out, "index.html",
		"<h2>About the study</h2>", "<dt>Status</dt><dd>Recruiting</dd>", `<a href="https://clinicaltrials.gov/study/NCT00000000">NCT00000000</a>`,
		"Participant information sheet", "Call 555-0142", `<a href="/team/">Team</a>`)
	// The study's built-in materials and contact pages go to those parts of
	// the home page.
	mustContain(t, out, "index.html",
		`<a href="/#resources">Study materials</a>`, `<a href="/#contact">Contact</a>`,
		`<h3 id="resources">Resources</h3>`, `<h3 id="contact">Contact</h3>`)
	mustContain(t, out, "team/index.html", "<h1>Team</h1>", `id="grace-liu"`, "<h2>Faculty</h2>", "<h2>Staff</h2>")
	mustContain(t, out, "publications/index.html", "BMJ Open")
	mustContain(t, out, "publications/publications.bib", "@article{liu2025protocol,")
	mustNotExist(t, out, "members/index.html", "courses/index.html", "projects/index.html", "talks/index.html")
}

// TestTiptapRenderer builds with richText = "json", so the site renders the
// Tiptap documents itself instead of using the API's HTML.
func TestTiptapRenderer(t *testing.T) {
	ts, _ := startMock(t)
	out, _ := build(t, ts.URL, "42", "HUGO_PARAMS_ACADEMICPAGE_RICHTEXT=json")
	mustContain(t, out, "posts/why-we-preregister-ehr-analyses/index.html",
		"<h2>What goes in the plan</h2>",
		"<ul><li><p>The target trial: eligibility, treatment strategies, and time zero.</p></li>",
		"<strong>preregister</strong>", "<em>missing</em>",
		`<a href="https://osf.io/">OSF</a>`,
		"<blockquote><p>A plan written after the results is a description of the results.</p></blockquote>",
		// The image resolves through /media: URL, size, and caption.
		`dag-confounding-large.svg" alt="A directed acyclic graph with treatment, outcome, and two confounders" width="1200" height="675"`,
		"<figcaption>The causal diagram we commit to before looking at outcomes.</figcaption>",
	)
	mustContain(t, out, "posts/multiple-imputation-chained-equations/index.html",
		`<pre><code class="language-r">library(mice)`, "imp &lt;- mice(dat", "<h3>Check convergence</h3>", "<code>plot(imp)</code>")
	mustContain(t, out, "posts/reviewing-forty-survival-analyses/index.html",
		"<strong><em>same three problems</em></strong>", "<ol><li>", "cheap.<br>The fix", "<s>Reporting a sensitivity analysis is enough.</s>")
	mustContain(t, out, "posts/sharing-code-with-papers/index.html", "<hr>")
	// Text is escaped, never passed through as markup.
	mustContain(t, out, "posts/hierarchical-models-small-area-rates/index.html",
		"<code>rate &lt; 0.05 &amp; n &gt; 30</code>", `<pre><code class="language-stan">`)
	mustContain(t, out, "about/index.html", `<a href="/research">research page</a>`)
}

// TestPagination lists with two records per request, so every collection
// with more than two records needs the adapter to follow links.next.
func TestPagination(t *testing.T) {
	ts, rec := startMock(t)
	out, _ := build(t, ts.URL, "42", "ACADEMIC_PAGE_PER_PAGE=2")
	if !rec.requested("/posts?per_page=2&page=4") {
		t.Errorf("the build did not follow links.next to the fourth page of posts")
	}
	for _, slug := range []string{"why-we-preregister-ehr-analyses", "welcome-fall-cohort", "hierarchical-models-small-area-rates"} {
		read(t, out, "posts/"+slug+"/index.html")
	}
	pubs := read(t, out, "publications/index.html")
	if n := strings.Count(pubs, `<li class="pub">`); n != 8 {
		t.Errorf("publications page lists %d entries, want 8", n)
	}
}

func TestBadKeyFailsWithoutLeaking(t *testing.T) {
	ts, _ := startMock(t)
	cmd := exec.Command(hugoBin(t), "build", "--environment", "demo", "--destination", t.TempDir(), "--cacheDir", t.TempDir())
	cmd.Dir = repoRoot(t)
	const wrong = "wrong-key-8c1d77"
	cmd.Env = append(os.Environ(), "ACADEMIC_PAGE_BASE_URL="+ts.URL+"/api/v1", "ACADEMIC_PAGE_TOKEN="+wrong, "ACADEMIC_PAGE_SITE=42")
	b, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("build succeeded with a wrong key")
	}
	if !strings.Contains(string(b), "Unauthorized") {
		t.Errorf("error does not say Unauthorized:\n%s", b)
	}
	if strings.Contains(string(b), wrong) {
		t.Errorf("error output contains the key")
	}
}

// TestServedFromAFolder builds with a baseURL that has a path, as a GitHub
// Pages project site does. Menu, home, and rich-text links get the folder.
func TestServedFromAFolder(t *testing.T) {
	ts, _ := startMock(t)
	out, _ := build(t, ts.URL, "42", "HUGO_BASEURL=https://example.org/castellanos/")
	mustContain(t, out, "index.html",
		`<a class="site-name" href="/castellanos/">`,
		`<a href="/castellanos/research/">Research</a>`,
		`<a href="/castellanos/research/">My research</a>`)
	mustNotContain(t, out, "index.html", `castellanos.biostatistician.page/research`)
	mustContain(t, out, "posts/why-we-preregister-ehr-analyses/index.html",
		`href="/castellanos/projects/registry-linkage"`)
	mustNotContain(t, out, "posts/why-we-preregister-ehr-analyses/index.html", `href="/projects/`)
	mustContain(t, out, "404.html", `<a href="/castellanos/">home page</a>`)
}
