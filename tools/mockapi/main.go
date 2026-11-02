// Command mockapi serves the fixtures in ./fixtures at the Academic.page
// Content API's paths, for building the example site without a real key.
//
//	go run ./tools/mockapi -addr 127.0.0.1:8787 -token demo-token
package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/OnPequod/academic.page-hugo/internal/mockapi"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8787", "address to listen on")
	dir := flag.String("fixtures", "fixtures", "fixture directory")
	token := flag.String("token", envOr("MOCK_API_TOKEN", "demo-token"), "the only API key the mock accepts")
	flag.Parse()

	srv, err := mockapi.New(*dir, *token)
	if err != nil {
		log.Fatalf("mockapi: %v", err)
	}
	log.Printf("mock Academic.page API on http://%s/api/v1 (fixtures: %s)", *addr, *dir)
	log.Fatal(http.ListenAndServe(*addr, logRequests(srv)))
}

// logRequests logs the method and path. It never logs headers, so the key
// stays out of the log.
func logRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.RequestURI())
		h.ServeHTTP(w, r)
	})
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
