# Hugo is pinned and built from source with Go (standard edition, no SCSS),
# so nothing needs to be installed beyond Go itself.
HUGO_VERSION := v0.169.0
HUGO := bin/hugo

# The demo serves the fixtures through a local mock API. SITE picks the
# fixture: 42 personal, 43 lab, 44 course, 45 study.
SITE ?= 42
MOCK_ADDR ?= 127.0.0.1:8787
MOCK_ENV = ACADEMIC_PAGE_BASE_URL=http://$(MOCK_ADDR)/api/v1 ACADEMIC_PAGE_TOKEN=demo-token ACADEMIC_PAGE_SITE=$(SITE)

# Settings for a real site can live in .env (ignored by git):
#   ACADEMIC_PAGE_TOKEN=...
#   ACADEMIC_PAGE_SITE=42
-include .env
export ACADEMIC_PAGE_TOKEN ACADEMIC_PAGE_SITE ACADEMIC_PAGE_BASE_URL

.PHONY: build serve demo demo-build mock test clean

## build: build ./public from the real API (needs ACADEMIC_PAGE_TOKEN and ACADEMIC_PAGE_SITE)
build: $(HUGO)
	$(HUGO) build --gc --minify

## serve: run the Hugo dev server against the real API
serve: $(HUGO)
	$(HUGO) server

## demo: serve the site from the fixtures through the mock API, no key needed
demo: $(HUGO) bin/mockapi
	@bin/mockapi -addr $(MOCK_ADDR) -token demo-token & pid=$$!; \
	trap 'kill $$pid 2>/dev/null' EXIT INT TERM; \
	sleep 1; \
	$(MOCK_ENV) $(HUGO) server --environment demo

## demo-build: build ./public from the fixtures through the mock API
demo-build: $(HUGO) bin/mockapi
	@bin/mockapi -addr $(MOCK_ADDR) -token demo-token & pid=$$!; \
	trap 'kill $$pid 2>/dev/null' EXIT INT TERM; \
	sleep 1; \
	$(MOCK_ENV) $(HUGO) build --environment demo --gc

## mock: run only the mock API, in the foreground
mock: bin/mockapi
	bin/mockapi -addr $(MOCK_ADDR) -token demo-token

## test: build every fixture site against the mock and check the output
test: $(HUGO)
	HUGO=$(CURDIR)/$(HUGO) go test -count=1 ./...

$(HUGO):
	GOBIN=$(CURDIR)/bin go install github.com/gohugoio/hugo@$(HUGO_VERSION)

bin/mockapi: $(wildcard internal/mockapi/*.go tools/mockapi/*.go)
	go build -o bin/mockapi ./tools/mockapi

clean:
	rm -rf public resources/_gen .hugo_build.lock bin
