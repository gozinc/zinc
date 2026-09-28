---
title: Testing
description: Test routes, middleware, and error handling with the standard net/http/httptest package.
---

You can test a Zinc app with Go's own `net/http/httptest` package, with no extra tooling. Send the app a request, then check the status, headers and body it sends back.

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

A Zinc app is an `http.Handler`, so `app.ServeHTTP` runs the full request: routing, middleware, binding and the error handler. `rec` records what was sent. Note the trailing `"\n"`: `c.JSON` ends every body with a newline.

:::note[No test helpers needed]
Zinc has no separate test package. A `*zinc.Context` only exists during a request, so there's nothing to build by hand: send a request and the app creates one.
:::

## Share the app between main and tests

Build your routes in one function that both `main` and your tests call. Your tests then run the same routing, middleware and error handling as production.

```go
// app.go
type User struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Store interface { // your data layer
	FindUser(ctx context.Context, id int64) (User, bool)
}

func showUser(store Store) zinc.HandlerFunc {
	return func(c *zinc.Context) error {
		id, err := zinc.Param[int64](c, "id")
		if err != nil {
			return err
		}
		user, ok := store.FindUser(c.Context(), id)
		if !ok {
			return zinc.NotFound("user not found")
		}
		return c.JSON(user)
	}
}

func newApp(store Store) *zinc.App {
	app := zinc.New()
	app.Use(recover.New())
	app.Get("/health", func(c *zinc.Context) error { return c.String("ok") })
	app.Get("/users/{id}", showUser(store))
	return app
}

// main.go
func main() {
	log.Fatal(newApp(postgresStore()).Listen(":8080")) // postgresStore is your production Store
}
```

In tests, pass a fake store instead:

```go
// app_test.go
type fakeStore map[int64]User

func (s fakeStore) FindUser(_ context.Context, id int64) (User, bool) {
	user, ok := s[id]
	return user, ok
}
```

## Check many cases with one table

A table-driven test covers each case in a line:

```go
func TestUsers(t *testing.T) {
	app := newApp(fakeStore{42: {ID: 42, Name: "Ada"}})

	tests := []struct {
		name, path string
		want       int
	}{
		{"found", "/users/42", http.StatusOK},
		{"missing", "/users/7", http.StatusNotFound},
		{"not a number", "/users/abc", http.StatusBadRequest},
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

```text
--- PASS: TestUsers (0.00s)
    --- PASS: TestUsers/found (0.00s)
    --- PASS: TestUsers/missing (0.00s)
    --- PASS: TestUsers/not_a_number (0.00s)
    --- PASS: TestUsers/no_such_route (0.00s)
```

## Test one handler

To test a handler on its own, register it on a one-route app. Route parameters, binding and the error handler all behave as they do in production:

```go
func TestShowUserBadID(t *testing.T) {
	app := zinc.New()
	app.Get("/users/{id}", showUser(fakeStore{}))

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/abc", nil))

	want := `{"error":{"status":400,"message":"invalid path parameter","fields":{"id":"must be an integer"}}}` + "\n"
	if rec.Code != http.StatusBadRequest || rec.Body.String() != want {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}
```

The 400 comes from `zinc.Param[int64]` in `showUser`: `abc` isn't an integer.

## Send a JSON body

Pass the body to `httptest.NewRequest` and set `Content-Type`, as a client would:

```go
func TestCreateUser(t *testing.T) {
	app := zinc.New()
	app.Post("/users", func(c *zinc.Context) error {
		var in struct {
			Name string `json:"name"`
		}
		if err := c.Bind().JSON(&in); err != nil {
			return err
		}
		return c.Status(zinc.StatusCreated).JSON(zinc.Map{"name": in.Name})
	})

	body := strings.NewReader(`{"name":"Ada"}`)
	req := httptest.NewRequest(http.MethodPost, "/users", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if got, want := rec.Body.String(), `{"name":"Ada"}`+"\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}
```

Binding picks the decoder from `Content-Type`. A request without one is read as JSON, but forms, XML and custom formats need the header.

## Test with an auth header or cookies

Set headers and add cookies on the request. Read cookies the app sets from `rec.Result().Cookies()`:

```go
func TestMe(t *testing.T) {
	app := zinc.New()
	app.Get("/me", func(c *zinc.Context) error {
		if c.Header("Authorization") != "Bearer test-token" {
			return zinc.ErrUnauthorized
		}
		cookie, err := c.Cookie("session")
		if err != nil {
			return zinc.ErrUnauthorized
		}
		c.SetCookie(&http.Cookie{Name: "seen", Value: "1"})
		return c.JSON(zinc.Map{"session": cookie.Value})
	})

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	req.AddCookie(&http.Cookie{Name: "session", Value: "abc123"})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "seen" {
		t.Fatalf("cookies = %v", cookies)
	}
}
```

Here the body is `{"session":"abc123"}` and the response carries `Set-Cookie: seen=1`.

## Test middleware

Register the middleware on a small app with one route, then check what it changes: headers, the status, or values it passes on.

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

The header holds a random ID such as `81dce52d9555d21b71d44050fc4a5e2e`, so the test checks it's there rather than its value.

## Test over a network connection

Use `httptest.NewServer` when behavior depends on an HTTP client and a live connection: redirects, cookies across requests, streaming, or HTTP/2.

```go
func TestHealthOverNetwork(t *testing.T) {
	server := httptest.NewServer(newApp(fakeStore{}))
	defer server.Close()

	res, err := server.Client().Get(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
}
```

## Next steps

- [Errors](/guide/errors/): the status codes and bodies your tests will see.
- [Binding](/guide/binding/): what a request needs for binding to succeed.
- [Zinc and net/http](/guide/http-interoperability/): everything else that works because the app is an `http.Handler`.
- [Typed Handlers](/guide/typed-handlers/): the next step once the essentials feel familiar.
