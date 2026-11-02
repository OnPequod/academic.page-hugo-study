package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestFixturesMatchSpec checks every fixture record against the OpenAPI
// schema it stands in for (fixtures/openapi.json, a copy of
// https://academic.page/api/v1/openapi.json). It checks required fields,
// unknown fields, JSON types, enums, and consts, following $ref and oneOf.
// It is a small subset of JSON Schema, enough to keep the fixtures honest.
func TestFixturesMatchSpec(t *testing.T) {
	root := filepath.Join(repoRoot(t), "fixtures")
	var spec map[string]any
	b, err := os.ReadFile(filepath.Join(root, "openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &spec); err != nil {
		t.Fatal(err)
	}
	schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)

	files := map[string]string{
		"site.json": "Site", "profile.json": "Profile", "study.json": "Study",
		"posts.json": "Post", "pages.json": "Page", "projects.json": "Project", "talks.json": "Talk",
		"publications.json": "Publication", "courses.json": "Course", "lab-members.json": "LabMember",
		"tags.json": "Tag", "media.json": "MediaItem",
	}
	sites, _ := filepath.Glob(filepath.Join(root, "sites", "*"))
	if len(sites) == 0 {
		t.Fatal("no fixture sites")
	}
	for _, dir := range sites {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			name := e.Name()
			if !strings.HasSuffix(name, ".json") {
				continue
			}
			schema, ok := files[name]
			if !ok {
				t.Errorf("%s/%s: no schema mapped for this file", filepath.Base(dir), name)
				continue
			}
			raw, _ := os.ReadFile(filepath.Join(dir, name))
			var doc any
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Errorf("%s/%s: %v", filepath.Base(dir), name, err)
				continue
			}
			v := validator{schemas: schemas}
			ref := map[string]any{"$ref": "#/components/schemas/" + schema}
			if list, ok := doc.([]any); ok {
				for i, rec := range list {
					v.check(fmt.Sprintf("%s/%s[%d]", filepath.Base(dir), name, i), ref, rec)
				}
			} else {
				v.check(filepath.Base(dir)+"/"+name, ref, doc)
			}
			for _, e := range v.errs {
				t.Error(e)
			}
		}
	}
}

type validator struct {
	schemas map[string]any
	errs    []string
}

func (v *validator) fail(path, format string, args ...any) {
	v.errs = append(v.errs, path+": "+fmt.Sprintf(format, args...))
}

func (v *validator) resolve(s map[string]any) map[string]any {
	for {
		ref, ok := s["$ref"].(string)
		if !ok {
			return s
		}
		s = v.schemas[strings.TrimPrefix(ref, "#/components/schemas/")].(map[string]any)
	}
}

// valid reports whether x matches s without recording errors.
func (v *validator) valid(s map[string]any, x any) bool {
	sub := validator{schemas: v.schemas}
	sub.check("", s, x)
	return len(sub.errs) == 0
}

func jsonType(x any) string {
	switch n := x.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case float64:
		if n == float64(int64(n)) {
			return "integer"
		}
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "unknown"
}

func (v *validator) check(path string, s map[string]any, x any) {
	s = v.resolve(s)
	if alts, ok := s["oneOf"].([]any); ok {
		for _, a := range alts {
			if v.valid(a.(map[string]any), x) {
				return
			}
		}
		v.fail(path, "matches none of oneOf")
		return
	}
	if c, ok := s["const"]; ok && c != x {
		v.fail(path, "want const %v, got %v", c, x)
	}
	if enum, ok := s["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			if e == x {
				found = true
			}
		}
		if !found {
			v.fail(path, "%v is not one of %v", x, enum)
		}
	}
	if t, ok := s["type"]; ok {
		var types []string
		switch tt := t.(type) {
		case string:
			types = []string{tt}
		case []any:
			for _, e := range tt {
				types = append(types, e.(string))
			}
		}
		got := jsonType(x)
		okType := false
		for _, want := range types {
			if want == got || (want == "number" && got == "integer") {
				okType = true
			}
		}
		if !okType {
			v.fail(path, "type %s, want %v", got, types)
			return
		}
	}
	switch val := x.(type) {
	case map[string]any:
		props, _ := s["properties"].(map[string]any)
		for _, r := range asList(s["required"]) {
			if _, ok := val[r.(string)]; !ok {
				v.fail(path, "missing required field %q", r)
			}
		}
		addl, hasAddl := s["additionalProperties"].(map[string]any)
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if ps, ok := props[k].(map[string]any); ok {
				v.check(path+"."+k, ps, val[k])
			} else if hasAddl {
				v.check(path+"."+k, addl, val[k])
			} else if props != nil {
				v.fail(path, "field %q is not in the schema", k)
			}
		}
	case []any:
		// Some arrays in the spec use "items": [], which is not a schema
		// (see the README); only object item schemas are checked.
		if items, ok := s["items"].(map[string]any); ok {
			for i, e := range val {
				v.check(fmt.Sprintf("%s[%d]", path, i), items, e)
			}
		}
	}
}

func asList(x any) []any {
	l, _ := x.([]any)
	return l
}
