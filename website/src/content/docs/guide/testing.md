---
title: Testing
description: Test routes, middleware, and error handling with the standard net/http/httptest package.
---

A Zinc app is an `http.Handler`, so you test it with `net/http/httptest` and no extra tooling. Build the app the same way production does, send it a request, and assert on the response.

```go
func TestGreeting(t *testing.T) {
	app := zinc.New()
	app.Get("/hello/{name}", func(c *zinc.Context) error {
		return c.JSON(zinc.Map{"hello": c.Param("name")})
	})

	req := httptest.NewRequest(http.MethodGet, "/hello/gopher", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if got, want := rec.Body.String(), `{"hello":"gopher"}`+"\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}
```

## Share the app between main and tests

Build routes in a function that both `main` and your tests call. Tests then exercise the real routing, middleware, and error handling.

```go
// app.go
func newApp(store Store) *zinc.App {
	app := zinc.New()
	app.Use(recover.New())
	app.Get("/users/{id}", showUser(store))
	return app
}

// main.go
func main() {
	log.Fatal(newApp(postgresStore()).Listen(":8080"))
}
```

## Table-driven tests

```go
func TestUsers(t *testing.T) {
	app := newApp(fakeStore{"42": {ID: "42", Name: "Ada"}})

	tests := []struct {
		name, path string
		want       int
	}{
		{"found", "/users/42", http.StatusOK},
		{"missing", "/users/7", http.StatusNotFound},
		{"no such route", "/accounts/42", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}
```

## Send a JSON body

```go
body := strings.NewReader(`{"name":"Ada"}`)
req := httptest.NewRequest(http.MethodPost, "/users", body)
req.Header.Set("Content-Type", "application/json")
```

Binding picks the decoder from `Content-Type`, so set it in tests just as a client would.

## Test middleware

Mount the middleware on a tiny app and assert what it changes: headers, status, or values it passes on.

```go
func TestRequestID(t *testing.T) {
	app := zinc.New()
	app.Use(requestid.New())
	app.Get("/", func(c *zinc.Context) error { return c.NoContent() })

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing X-Request-ID")
	}
}
```

## Test one handler

To test a handler on its own, register it on a one-route app. Route parameters, binding, and the error handler all behave exactly as in production:

```go
func TestShowUser(t *testing.T) {
	app := zinc.New()
	app.Get("/users/{id}", showUser)

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/abc", nil))

	want := `{"error":{"status":400,"message":"invalid path parameter","fields":{"id":"must be an integer"}}}` + "\n"
	if rec.Code != http.StatusBadRequest || rec.Body.String() != want {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}
```

Zinc has no separate test package. A `*zinc.Context` only exists during a request, so there is nothing to construct by hand.

## Test over a real connection

Use `httptest.NewServer` when behavior depends on a real client: redirects, cookies, streaming, or HTTP/2.

```go
server := httptest.NewServer(app)
defer server.Close()

res, err := server.Client().Get(server.URL + "/health")
```

## Next steps

- [Errors](/guide/errors/) explains the status codes your tests will see.
- [Zinc and net/http](/guide/http-interoperability/) covers everything else that works because the app is a handler.
