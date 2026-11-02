# Academic.page + Hugo: Study site example

This repository is a copy of the [Academic.page Hugo starter](https://github.com/OnPequod/academic.page-hugo), set up to build one example study site: Neighborhood Services Access Study. The site is written in [Academic.page](https://academic.page) and built by Hugo from the Academic.page content API.

- **The built site:** <https://onpequod.github.io/academic.page-hugo-study/>
- **The other examples:** [personal](https://github.com/OnPequod/academic.page-hugo-personal), [course](https://github.com/OnPequod/academic.page-hugo-course), [study](https://github.com/OnPequod/academic.page-hugo-study), and [lab](https://github.com/OnPequod/academic.page-hugo-lab)

The person, group, course, or study on this site is made up, and most of its text is placeholder text. It is the same content as the study site demo on Academic.page.

`.github/workflows/deploy.yml` builds the site every day and on each push to `main`, and publishes it with GitHub Pages. It reads the site with a content key stored in the `ACADEMIC_PAGE_TOKEN` secret. To start your own site, copy [the starter](https://github.com/OnPequod/academic.page-hugo) instead of this repository: this copy is regenerated from it with `tools/example-repo/make` and has no history of its own worth keeping.

The rest of this file is the starter's README.

---

# Academic.page + Hugo

This repository is an example Hugo site that uses [Academic.page](https://academic.page) as a headless content backend. You keep writing in the Academic.page editor, and Hugo builds a static site from the published content through the Academic.page Content API.

It covers every public section the API exposes: the profile, posts, pages, projects, publications (with BibTeX), talks, courses, lab and study team members, the study record, tags, and media. It handles all four site types (personal, lab, course, and study). The design is plain and accessible, and it is meant to be restyled.

You can run it without an account. `make demo` builds and serves the site from bundled fixture data through a small local mock of the API.

To see what it publishes, look at the four example sites, each built by a copy of this repository from an Academic.page site:

| Site type | Built site | Repository |
| --- | --- | --- |
| Personal | <https://onpequod.github.io/academic.page-hugo-personal/> | [academic.page-hugo-personal](https://github.com/OnPequod/academic.page-hugo-personal) |
| Course | <https://onpequod.github.io/academic.page-hugo-course/> | [academic.page-hugo-course](https://github.com/OnPequod/academic.page-hugo-course) |
| Study | <https://onpequod.github.io/academic.page-hugo-study/> | [academic.page-hugo-study](https://github.com/OnPequod/academic.page-hugo-study) |
| Lab | <https://onpequod.github.io/academic.page-hugo-lab/> | [academic.page-hugo-lab](https://github.com/OnPequod/academic.page-hugo-lab) |

## Contents

- [Quick start](#quick-start)
- [How the build pulls from the API](#how-the-build-pulls-from-the-api)
- [Use it with a real site](#use-it-with-a-real-site)
- [Site types and sections](#site-types-and-sections)
- [Pages the site builds](#pages-the-site-builds)
- [Rich text](#rich-text)
- [Deploying](#deploying)
- [Limits](#limits)
- [Tests](#tests)
- [The example sites](#the-example-sites)
- [Repository layout](#repository-layout)
- [Notes on the Hugo setup](#notes-on-the-hugo-setup)

## Quick start

You need Go 1.24 or later. Nothing else is installed: the Makefile builds the pinned Hugo release (v0.169.0, standard edition) from source into `bin/` on first use, which takes a minute or two.

```sh
make demo            # serve the personal-site fixture at http://localhost:1313
make demo SITE=43    # the lab site; 44 is the course site and 45 the study site
make test            # build all four fixture sites and check the output
```

## How the build pulls from the API

Hugo [content adapters](https://gohugo.io/content-management/content-adapters/) create pages from data while Hugo reads content. This site has one, `content/_content.gotmpl`. When you run `hugo build`, it does the following:

1. It fetches the site record (`GET /content/sites/{site}`), which lists the site's sections, navigation menus, and settings.
2. For each section in that list, it calls the matching partial in `layouts/_partials/adapters/`. Each partial fetches every record in its collection and adds one page per record at the record's public path, such as `/posts/<slug>` or `/publications/<slug>`.
3. It adds the about and CV pages from the profile, and one page per tag.

The templates then read the same API responses to render each page. Requests go through `resources.GetRemote` with the key in an `Authorization: Bearer` header. Lists are fetched 100 records at a time, and the adapter follows `links.next` until the last page. Hugo deduplicates identical requests within a build, so each URL is fetched once.

There is no sync step and no generated content in the repository. Every build reads the current published content.

The API helpers are in `layouts/_partials/api/`:

| Partial | Purpose |
| --- | --- |
| `config.html` | Reads the environment variables and builds the request headers. |
| `fetch.html` | Fetches one URL and decodes the JSON. Fails the build on errors other than 404. |
| `list.html` | Fetches a whole collection, following `links.next`. |
| `single.html` | Fetches the site record, the profile, or the study record. |
| `site.html` | Returns the site record, or stops the build if the key cannot see the site. |
| `index.html`, `record.html` | Look up the record behind a page by collection and ID. |
| `bibtex.html` | Downloads `publications.bib` and publishes it with the site. |

## Use it with a real site

1. Create a content key at <https://academic.page/admin/account/api>. Keys need a Pro plan.
2. Find the site's ID: it is the number in the site's admin address, such as 42 in `/sites/42/posts`.
3. Copy `.env.example` to `.env` and fill in both values, or export them in your shell.
4. Run `make serve` for the development server, or `make build` to write the site to `public/`.

| Variable | Required | Meaning |
| --- | --- | --- |
| `ACADEMIC_PAGE_TOKEN` | yes | The content key. It is sent only in the `Authorization` header. |
| `ACADEMIC_PAGE_SITE` | yes | The site's numeric ID. |
| `ACADEMIC_PAGE_BASE_URL` | no | Defaults to `https://academic.page/api/v1`. |
| `ACADEMIC_PAGE_PER_PAGE` | no | Records per list request, 1 to 100. Defaults to 100. |
| `HUGO_BASEURL` | for deploys | The address the built site will live at, such as `https://example.edu/`. |

The templates are the same for the mock and the real API. Only the base URL and the key change.

`.env` is listed in `.gitignore`. Never commit a key. Hugo's own cache is outside the repository by default, and this site turns off caching of API responses (see [Notes on the Hugo setup](#notes-on-the-hugo-setup)), so the key and the responses do not end up on disk in the project.

Two settings live in `config/_default/hugo.toml` under `[params.academicPage]`:

- `richText` is `"html"` (the default) or `"json"`. See [Rich text](#rich-text).
- `publicationsGroupBy` is `"year"` (the default) or `"type"`.

## Site types and sections

Academic.page sites have a type, and the type decides which sections exist. The API answers `404 section_not_enabled` for a section the site does not have. The adapter fetches only the sections listed in the site record's `sections` field, so a build never asks for a missing section.

| Section (API path) | Personal | Lab | Study | Course |
| --- | --- | --- | --- | --- |
| `pages` | yes | yes | yes | yes |
| `posts` | yes | yes | yes | yes |
| `projects` | yes | yes | | |
| `publications` (and `publications.bib`) | yes | yes | yes | |
| `talks` | yes | yes | | |
| `lab-members` | | yes | yes (the study team) | |
| `courses` | | | | yes |
| `study` | | | yes | |

Every type also has the site record, the profile, the manifest, tags, and media. Personal sites carry the full profile: the bio, photo, CV, degrees, positions, and links. The other types return a profile with the site name and whatever parts the site shows.

The fixtures include one site of each type: 42 is a personal site, 43 a lab, 44 a course, and 45 a study. The researcher, lab, course, and study are made up.

## Pages the site builds

Pages keep the paths they have on the Academic.page site, so links inside rich text still work.

| Path | Source | Notes |
| --- | --- | --- |
| `/` | profile, site, study, and featured records | Photo, summary, links, and contact. Personal sites without an about page show the full profile here. Study sites show the study record, and course sites list their courses. Then selected publications, upcoming talks, projects, and recent posts, for the sections the site has. |
| `/about/`, `/cv/` | profile | Only when the profile has an about page or a CV file. |
| `/posts/`, `/posts/<slug>/` | posts | Paginated, five per page. Tags, featured image, author, and previous and next links. RSS at `/posts/index.xml`. |
| `/<path>/` | pages | Nested pages keep their paths (`/research/methods/`). A page with children lists them. |
| `/projects/`, `/projects/<slug>/` | projects | Status, dates, links, technologies, featured image, and gallery. |
| `/publications/`, `/publications/<slug>/` | publications | Grouped by year (or type), with the formatted citation, type, DOI, and supplementary links. Each publication also gets a CSL-JSON file. |
| `/publications/publications.bib` | `publications.bib` | The whole list as one BibTeX file. |
| `/talks/`, `/talks/<slug>/` | talks | Split into upcoming and past by the build date, in the site's order: newest year first, and within a year the order the site sets. Dates known only by year or month show as such. Slides, paper, and recording links. |
| `/teaching/`, `/teaching/<slug>/` | courses | Offerings, syllabus, lessons with materials, readings, and assignments, then course-level materials, assignments, and references. Due times show in the site's timezone. The folder is the one the API's course paths use (`/teaching`, or `/courses` on older sites). |
| `/members/` or `/team/` | lab-members | One page. Current members are grouped by role, then alumni. Each person has an anchor that matches the API's path (`/members#slug`). |
| `/tags/`, `/tags/<slug>/` | tags | Tag names and descriptions from the API. A tag page groups its records by section. |

The header and footer menus come from the site's navigation settings (`navigation.header` and `navigation.footer`). If a site has no header menu, the site builds one from its sections.

Menus can link to pages that Academic.page builds in for a site type, which the API does not return as pages. These go to the place this site shows the same content: on a study site, `/resources` and `/contact` go to those parts of the home page, and on a course site, `/syllabus` and the course sections (`/#lessons`, `/#assignments`, `/#texts-resources`, `/#offerings`) go to the course's page. A menu item whose page this site does not build, such as a course site's instructor page, is left out. A menu group without a link of its own shows its label as a button that opens the group. The site's name, accent color, favicons, footer text, homepage title and description, and search-indexing setting (which controls `robots.txt` and a `noindex` tag) also come from the site record.

Media files are used where records reference them: profile and member photos, featured images, project galleries, talk slides and papers, syllabi, course files, and images inside rich text. Images load from the API's media URLs and use the resized versions in `conversions` when they exist.

## Rich text

Rich text fields (post content, abstracts, bios, and others) arrive in two forms: `html`, rendered and sanitized by Academic.page the way the public site shows it, and `json`, the editor's Tiptap (ProseMirror) document.

By default the site uses `html`. Set `richText = "json"` to render the Tiptap document with this repository's templates instead (`layouts/_partials/richtext/`). The JSON renderer is also the fallback when a field has no HTML. It supports paragraphs, headings (levels 1 to 3), text, hard breaks, bullet and ordered lists, blockquotes, code blocks with a language, horizontal rules, and images, with bold, italic, strike, code, and link marks. Other nodes keep their text and lose their markup. All text is escaped, and links are kept only for `http`, `https`, `mailto`, site-relative, and fragment URLs. Images that carry a `data-media-id` are resolved through the media library (`/media`) for their URL, alt text, size, and caption.

## Deploying

The output in `public/` is plain static files, so any static host works: Netlify, Cloudflare Pages, GitHub Pages, S3, or a university web server. Build with the key and site ID in the build environment and the production address in `HUGO_BASEURL`:

```sh
HUGO_BASEURL=https://example.edu/ make build
```

Because content is read at build time, the site changes only when it is rebuilt. There are two ways to trigger rebuilds.

**Webhooks.** Academic.page can call up to five HTTPS URLs per site when public content changes (Pro plan; set them up at <https://academic.page/admin/account/api>). Each call is a `POST` with a `content.updated` JSON event that carries the site ID, the new `content_version`, and the update time. Calls are debounced by about a minute, retried on failure, and signed with HMAC-SHA256 over the timestamp and the body. The settings page has a button that sends a test event. Saving a draft does not send an event. A check every five minutes sends one when a scheduled post goes live. The details are at <https://academic.page/api/docs/webhooks>.

Netlify and Cloudflare Pages give each site a build hook URL that starts a build on any `POST`. Point an Academic.page webhook at it. These hooks do not check the signature, so treat the hook URL as a secret. GitHub Actions needs an authenticated request to start a workflow, so it needs a small relay (a serverless function, for example) that checks the signature and calls the `repository_dispatch` API.

**A schedule.** Rebuild every hour or every night. This is simpler and works on any host with scheduled builds. To skip builds when nothing changed, compare the site record's `content_version` with the value from the last build (it changes whenever public content may have changed), or ask the manifest for records changed since then with `updated_since`.

A GitHub Actions workflow that builds on a schedule and on a dispatch event looks like this. Store the key as a repository secret.

```yaml
name: Build
on:
  schedule: [{ cron: "0 * * * *" }]
  repository_dispatch: { types: [academic-page-content-updated] }
  workflow_dispatch:
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: stable }
      - run: make build
        env:
          ACADEMIC_PAGE_TOKEN: ${{ secrets.ACADEMIC_PAGE_TOKEN }}
          ACADEMIC_PAGE_SITE: "42"
          HUGO_BASEURL: https://example.edu/
      # Then publish ./public with your host's deploy action.
```

## Limits

- **Content API rate limit.** Content reads are limited to 600 requests a minute per key. A build makes about one request per section (more for collections over 100 records), plus the site, profile, tags, and BibTeX requests. The fixture sites take 6 to 10 requests per build. The development server refetches everything on each rebuild.
- **Published content only.** The Content API never returns drafts, scheduled items before their time, or private notes. There is no preview of unpublished work.
- **No incremental builds.** Every build fetches every collection. The manifest and `updated_since` could support incremental fetching, but Hugo rebuilds the whole site anyway, and for academic sites the full fetch is small.
- **Changes need a rebuild.** `hugo server` does not watch the API. Restart it, or save a template, to pick up new content.
- **Media stays on Academic.page.** Images and files load from the API's media URLs. If you want copies on your own host, fetch them with `resources.GetRemote` in the templates; that adds build time.
- **Internal links in rich text.** Links inside content are site-relative paths without a trailing slash (`/research`). Most static hosts redirect these to `/research/`.
- **Serving from a folder.** A site can live in a folder of another site, such as a GitHub Pages project site at `https://<owner>.github.io/<repository>/`. Set `HUGO_BASEURL` to the full address with the folder. Menu links, the site's own links, and links and images in rich text that start with `/` get the folder in front.

## Tests

`make test` runs three groups of tests:

- `internal/mockapi`: the mock's auth, Accept, pagination, filter, `section_not_enabled`, and ETag behavior.
- `tests/fixtures_test.go`: checks every fixture record against its schema in `fixtures/openapi.json` (a copy of the live spec): required fields, unknown fields, types, enums, and consts.
- `tests/site_test.go`: builds each fixture site with Hugo against the mock and checks that every section's pages exist and contain the expected content. It also builds with the Tiptap renderer and checks its HTML and escaping, builds with two records per request to check that the adapter follows `links.next`, checks that a wrong key fails the build with `Unauthorized`, and checks that the key appears nowhere in the build output, Hugo's cache, or Hugo's console output.

## The example sites

Each example repository is this repository plus a GitHub Pages workflow that builds one site from Academic.page's API example account, and a few lines at the top of the README. `tools/example-repo/make` writes one from the current commit:

```sh
tools/example-repo/make personal 179 "Maya Brooks" ../academic.page-hugo-personal
```

The arguments are the site type, the site's ID, its name, and the example repository's checkout. Rerun it after changing the starter, then commit and push in the example repository. The workflow there reads the site with a content key stored in its `ACADEMIC_PAGE_TOKEN` secret.

## Repository layout

```
content/_content.gotmpl          the content adapter
layouts/_partials/api/           API requests
layouts/_partials/adapters/      one partial per section, called by the adapter
layouts/_partials/records/       the body of each record type's page
layouts/_partials/richtext/      the Tiptap renderer
layouts/                         page, list, home, tag, and RSS templates
assets/css/site.css              the stylesheet (plain CSS)
config/_default/hugo.toml        Hugo settings
config/demo/hugo.toml            lets Hugo reach the mock on 127.0.0.1
fixtures/                        fixture data for four sites, and the OpenAPI spec
internal/mockapi/, tools/mockapi the mock API
tests/                           build tests
tools/example-repo/              writes an example repository (see The example sites)
```

## Notes on the Hugo setup

These settings in `config/_default/hugo.toml` are needed and easy to miss:

- `security.funcs.getenv` allows `^ACADEMIC_PAGE_` so templates can read the settings.
- `security.allowContent` allows `text/html` page content. Hugo refuses HTML content by default. The HTML here comes from Academic.page's renderer, which sanitizes it.
- `security.http.mediaTypes` accepts `application/x-bibtex`, and `mediaTypes` maps it to `.bib`. Without both, Hugo cannot resolve the BibTeX response.
- `caches.getresource.maxAge = 0` turns off Hugo's cache of remote responses. The default keeps them forever, so rebuilds would show stale content.
- `buildFuture = true`, because the API returns only published records. A future date there is real, such as an upcoming talk.
- The `demo` environment (`config/demo/hugo.toml`) allows requests to `http://127.0.0.1`. Hugo's default policy blocks localhost and IP addresses, and the production settings keep that policy.

A few parts of the API do not map directly onto content adapter pages, and the site handles them this way:

- **BibTeX.** Adapters make pages, and the BibTeX export is a file. The publications templates download it with `resources.GetRemote` and publish it with `resources.Copy`.
- **Lab members.** Members have no pages of their own on Academic.page; their paths are anchors on one list page. The adapter adds that one page, and its template reads the member list.
- **Record data in front matter.** Hugo lowercases front matter keys, which would break case-sensitive fields such as CSL-JSON's `DOI` and `URL`. Adapter pages store only the collection and record ID, and templates look the record up from the cached API response.
- **Section pages.** An adapter cannot add its own directory's section page, so a single adapter at the content root adds both the sections and their records.

In Hugo 0.169, `{{ with partial "x" }}` is parsed as a partial decorator. The templates assign a partial's result to a variable before testing it with `with`.

## License

MIT. See [LICENSE](LICENSE).
