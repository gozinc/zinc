// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package openapitest validates the OpenAPI spec Zinc generates. It's a
// separate module so the validator it uses never becomes a dependency of
// Zinc itself.
package openapitest

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// goldenPath is the spec the root package's TestOpenAPIGolden checks in.
const goldenPath = "../testdata/openapi/golden.json"

const (
	oasSchemaURL = "https://spec.openapis.org/oas/3.1/schema/2022-10-07"
	goldenURL    = "https://zinc.test/golden.json"
)

func load(t *testing.T, path string) any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	doc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return doc
}

// TestGoldenSpecIsOpenAPI31 checks the document's structure against the
// official OpenAPI 3.1 schema, which doesn't look inside schema objects.
func TestGoldenSpecIsOpenAPI31(t *testing.T) {
	c := jsonschema.NewCompiler()
	if err := c.AddResource(oasSchemaURL, load(t, "testdata/oas-3.1-schema.json")); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile(oasSchemaURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := sch.Validate(load(t, goldenPath)); err != nil {
		t.Fatalf("golden spec isn't valid OpenAPI 3.1:\n%v", err)
	}
}

// TestGoldenSchemasAreJSONSchema compiles every schema object in the spec,
// in components and inline, as JSON Schema 2020-12, the dialect OpenAPI 3.1
// uses. Compiling checks each against the 2020-12 meta-schema and resolves
// every $ref.
func TestGoldenSchemasAreJSONSchema(t *testing.T) {
	doc := load(t, goldenPath)
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	if err := c.AddResource(goldenURL, doc); err != nil {
		t.Fatal(err)
	}
	pointers := schemaPointers(doc, "")
	if len(pointers) < 10 {
		t.Fatalf("found only %d schemas; the walk is broken", len(pointers))
	}
	for _, ptr := range pointers {
		if _, err := c.Compile(goldenURL + "#" + ptr); err != nil {
			t.Errorf("schema at %s: %v", ptr, err)
		}
	}
}

// schemaPointers finds the JSON pointer of every schema object: each
// "schema" value under paths, and each entry of components.schemas.
func schemaPointers(node any, ptr string) []string {
	var out []string
	switch v := node.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			child := ptr + "/" + escapePointer(k)
			switch {
			case k == "schema" && strings.HasPrefix(ptr, "/paths/"):
				out = append(out, child)
			case ptr == "/components/schemas":
				out = append(out, child)
			default:
				out = append(out, schemaPointers(v[k], child)...)
			}
		}
	case []any:
		for i, item := range v {
			out = append(out, schemaPointers(item, ptr+"/"+strconv.Itoa(i))...)
		}
	}
	return out
}

func escapePointer(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

// TestValidationCatchesMistakes proves the two checks above can fail: each
// case breaks a copy of the golden spec.
func TestValidationCatchesMistakes(t *testing.T) {
	c := jsonschema.NewCompiler()
	if err := c.AddResource(oasSchemaURL, load(t, "testdata/oas-3.1-schema.json")); err != nil {
		t.Fatal(err)
	}
	oas, err := c.Compile(oasSchemaURL)
	if err != nil {
		t.Fatal(err)
	}
	get := func(doc any, path ...string) map[string]any {
		node := doc
		for _, p := range path {
			node = node.(map[string]any)[p]
		}
		return node.(map[string]any)
	}

	doc := load(t, goldenPath)
	doc.(map[string]any)["openapi"] = "3.0.3"
	if oas.Validate(doc) == nil {
		t.Error("an OpenAPI 3.0 version string passed")
	}

	// OpenAPI 3.1 requires required: true on path parameters. (It made
	// responses optional, unlike 3.0, so leaving those out isn't an error.)
	doc = load(t, goldenPath)
	param := get(doc, "paths", "/pets/{id}", "get")["parameters"].([]any)[0].(map[string]any)
	delete(param, "required")
	if oas.Validate(doc) == nil {
		t.Error("a path parameter without required: true passed")
	}

	for name, mutate := range map[string]func(schemas map[string]any){
		"dangling $ref": func(s map[string]any) {
			get(s, "oaPet", "properties", "owner")["anyOf"].([]any)[0].(map[string]any)["$ref"] = "#/components/schemas/Nope"
		},
		"bad keyword value": func(s map[string]any) { get(s, "oaPet", "properties", "name")["minLength"] = -1 },
	} {
		doc := load(t, goldenPath)
		mutate(get(doc, "components", "schemas"))
		c := jsonschema.NewCompiler()
		c.DefaultDraft(jsonschema.Draft2020)
		if err := c.AddResource(goldenURL, doc); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Compile(goldenURL + "#/components/schemas/oaPet"); err == nil {
			t.Errorf("%s: compiled without error", name)
		}
	}
}
