// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/0mjs/zinc"
)

// do sends a request to app and prints the status and body, as a client
// would see them.
func do(app *zinc.App, method, target, body string) {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	fmt.Println(w.Code, strings.TrimSpace(w.Body.String()))
}

// A route returns an error or writes a response. Zinc is an http.Handler, so
// app.Listen(":8080") serves it, or any net/http server can.
func Example() {
	app := zinc.New()
	app.Get("/hello/{name}", func(c *zinc.Context) error {
		return c.JSON(zinc.Map{"message": "Hello, " + c.Param("name") + "!"})
	})

	do(app, "GET", "/hello/ada", "")
	// Output: 200 {"message":"Hello, ada!"}
}

type CreateUser struct {
	Org   string `path:"org"`
	Email string `json:"email" validate:"required,email"`
}

type User struct {
	ID    int    `json:"id"`
	Org   string `json:"org"`
	Email string `json:"email"`
}

// A typed handler's input binds from the request and is validated before the
// function runs; its output is the response. Both describe the route in the
// OpenAPI spec.
func ExampleTyped() {
	app := zinc.New()
	app.Post("/orgs/{org}/users", zinc.Typed(func(c *zinc.Context, in CreateUser) (User, error) {
		return User{ID: 1, Org: in.Org, Email: in.Email}, nil
	})).Status(http.StatusCreated)

	do(app, "POST", "/orgs/acme/users", `{"email":"ada@example.com"}`)
	do(app, "POST", "/orgs/acme/users", `{"email":"nope"}`)
	// Output:
	// 201 {"id":1,"org":"acme","email":"ada@example.com"}
	// 422 {"error":{"status":422,"message":"validation failed","fields":{"email":"must be an email address"}}}
}

// A handler returns an error, and one error handler writes every failure.
func ExampleNotFound() {
	app := zinc.New()
	app.Get("/users/{id}", func(c *zinc.Context) error {
		return zinc.NotFound("user not found")
	})

	do(app, "GET", "/users/42", "")
	// Output: 404 {"error":{"status":404,"message":"user not found"}}
}

// A group shares a prefix and middleware with every route registered in it.
func ExampleApp_Group() {
	app := zinc.New()
	requireKey := func(c *zinc.Context) error {
		if c.Header("X-API-Key") != "secret" {
			return zinc.Unauthorized("missing or wrong API key")
		}
		return c.Next()
	}
	api := app.Group("/api", requireKey)
	api.Get("/status", func(c *zinc.Context) error { return c.JSON(zinc.Map{"ok": true}) })

	do(app, "GET", "/api/status", "")
	// Output: 401 {"error":{"status":401,"message":"missing or wrong API key"}}
}

// Every app serves its OpenAPI 3.1 spec at /openapi.json. OpenAPISpec builds
// it in code, for tests or to write it to a file.
func ExampleApp_OpenAPISpec() {
	app := zinc.New()
	app.Post("/orgs/{org}/users", zinc.Typed(func(c *zinc.Context, in CreateUser) (User, error) {
		return User{}, nil
	})).Status(http.StatusCreated)

	spec, err := app.OpenAPISpec(zinc.OpenAPIConfig{Title: "Users", Version: "1.0.0"})
	if err != nil {
		panic(err)
	}
	var doc struct {
		OpenAPI string                                `json:"openapi"`
		Paths   map[string]map[string]json.RawMessage `json:"paths"`
	}
	_ = json.Unmarshal(spec, &doc)
	fmt.Println(doc.OpenAPI)
	for path, ops := range doc.Paths {
		for method := range ops {
			fmt.Println(method, path)
		}
	}
	// Output:
	// 3.1.0
	// post /orgs/{org}/users
}
