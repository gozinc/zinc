// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package openapitest

import (
	"bytes"
	"context"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/openapitest/petclient"
)

var update = flag.Bool("update", false, "rewrite testdata/conformance.json")

const conformanceSpec = "testdata/conformance.json"

// TestConformanceSpecFile keeps the file petclient is generated from in step
// with the app. After a change, run go test -run TestConformanceSpecFile
// -update, then go generate ./petclient.
func TestConformanceSpecFile(t *testing.T) {
	spec, err := conformanceApp().OpenAPISpec(zinc.OpenAPIConfig{Title: "Conformance", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		if err := os.WriteFile(conformanceSpec, spec, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(conformanceSpec)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(spec, want) {
		t.Fatalf("%s is out of date: run with -update, then go generate ./petclient", conformanceSpec)
	}
}

// TestGeneratedClient calls the running app through a client oapi-codegen
// generated from its spec: a tool that reads the spec can use the API.
func TestGeneratedClient(t *testing.T) {
	srv := httptest.NewServer(conformanceApp())
	defer srv.Close()
	c, err := petclient.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	dog := petclient.CreatePetBodyKindDog // oneof=cat dog became an enum type
	created, err := c.CreatePetWithResponse(ctx, "main", petclient.CreatePetJSONRequestBody{Name: "Rex", Kind: &dog})
	if err != nil {
		t.Fatal(err)
	}
	if created.StatusCode() != http.StatusCreated || created.JSON201 == nil || created.JSON201.Name != "Rex" {
		t.Fatalf("create: %d %s", created.StatusCode(), created.Body)
	}

	conflicted, err := c.CreatePetWithResponse(ctx, "main", petclient.CreatePetJSONRequestBody{Name: "Taken"})
	if err != nil {
		t.Fatal(err)
	}
	if conflicted.StatusCode() != http.StatusConflict {
		t.Fatalf("conflict: %d %s", conflicted.StatusCode(), conflicted.Body)
	}

	invalid, err := c.CreatePetWithResponse(ctx, "main", petclient.CreatePetJSONRequestBody{Name: ""})
	if err != nil {
		t.Fatal(err)
	}
	if invalid.StatusCode() != http.StatusUnprocessableEntity || invalid.JSON422 == nil || invalid.JSON422.Error.Status != 422 {
		t.Fatalf("invalid: %d %s", invalid.StatusCode(), invalid.Body)
	}

	got, err := c.GetPetWithResponse(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if got.StatusCode() != http.StatusOK || got.JSON200 == nil || got.JSON200.Owner == nil || string(got.JSON200.Owner.Email) != "ada@example.com" || got.JSON200.Kind != petclient.PetKindDog {
		t.Fatalf("get: %d %s", got.StatusCode(), got.Body)
	}

	kind := "dog"
	list, err := c.ListPetsWithResponse(ctx, &petclient.ListPetsParams{Kind: &kind})
	if err != nil {
		t.Fatal(err)
	}
	if list.StatusCode() != http.StatusOK || list.JSON200 == nil || len(*list.JSON200) != 1 {
		t.Fatalf("list: %d %s", list.StatusCode(), list.Body)
	}

	deleted, err := c.DeletePetWithResponse(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.StatusCode() != http.StatusNoContent {
		t.Fatalf("delete: %d %s", deleted.StatusCode(), deleted.Body)
	}

	replaced, err := c.ReplacePetWithResponse(ctx, "7", petclient.ReplacePetJSONRequestBody{Name: "Max", Kind: &dog})
	if err != nil {
		t.Fatal(err)
	}
	if replaced.StatusCode() != http.StatusOK || replaced.JSON200 == nil || replaced.JSON200.Name != "Max" {
		t.Fatalf("replace: %d %s", replaced.StatusCode(), replaced.Body)
	}
}
