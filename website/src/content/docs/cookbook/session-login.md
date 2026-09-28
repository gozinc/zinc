---
title: Session Login
description: Log users in with a form, keep them logged in with a signed session cookie, protect a page, and log them out.
---

This program is a login flow for a server-rendered site: a login form, a page only logged-in users can see, and a logout button. After a successful login the user's name lives in a signed cookie, so the server keeps no session store. It uses the [session middleware](/middleware/session/) and a small middleware of your own that guards the protected routes.

## Run it

```bash
mkdir zinc-login && cd zinc-login
go mod init example.com/zinc-login
go get github.com/0mjs/zinc
```

Save the program as `main.go` and run `go run .`. It listens on port 8080. Without `SESSION_SECRET` set, it signs cookies with a local test secret. Open <http://localhost:8080/account> in a browser and log in as `ada` with the password `correct-horse-battery`.

## The program

```go title="main.go"
package main

import (
	"crypto/subtle"
	"html"
	"log"
	"os"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/session"
)

// users stands in for your user store. Store password hashes
// (bcrypt or argon2), never plain passwords, in yours.
var users = map[string]string{
	"ada": "correct-horse-battery",
}

const loginPage = `<!doctype html>
<title>Log in</title>
<form method="post" action="/login">
  <input name="username" placeholder="Username">
  <input name="password" type="password" placeholder="Password">
  <button>Log in</button>
</form>`

// requireLogin sends visitors without a user in their session to /login.
func requireLogin(c *zinc.Context) error {
	if session.MustGet(c).Get("user") == "" {
		return c.Redirect("/login")
	}
	return c.Next()
}

func main() {
	secret := []byte(os.Getenv("SESSION_SECRET"))
	if len(secret) == 0 {
		secret = []byte("local-test-secret-at-least-32-bytes")
		log.Print("SESSION_SECRET not set; using the local test secret")
	}

	app := zinc.New()
	app.Use(session.New(session.Config{Secret: secret}))

	app.Get("/login", func(c *zinc.Context) error {
		return c.HTML(loginPage)
	})

	app.Post("/login", func(c *zinc.Context) error {
		name := c.FormValue("username")
		password, ok := users[name]
		if !ok || subtle.ConstantTimeCompare([]byte(password), []byte(c.FormValue("password"))) != 1 {
			return zinc.Unauthorized("wrong username or password")
		}
		if err := session.MustGet(c).Set("user", name); err != nil {
			return err
		}
		return c.Status(zinc.StatusSeeOther).Redirect("/account")
	})

	app.Post("/logout", func(c *zinc.Context) error {
		if err := session.MustGet(c).Delete("user"); err != nil {
			return err
		}
		return c.Status(zinc.StatusSeeOther).Redirect("/login")
	})

	account := app.Group("/account", requireLogin)
	account.Get("/", func(c *zinc.Context) error {
		user := session.MustGet(c).Get("user")
		return c.HTML(`<p>Hello, ` + html.EscapeString(user) + `.</p>
<form method="post" action="/logout"><button>Log out</button></form>`)
	})

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

The protected page sends you to the login form:

```bash
curl -i localhost:8080/account
# HTTP/1.1 302 Found
# Location: /login
```

Log in with curl, saving the cookie to `cookies.txt`:

```bash
curl -i localhost:8080/login -c cookies.txt -b cookies.txt \
  -d username=ada -d password=correct-horse-battery
# HTTP/1.1 303 See Other
# Location: /account
# Set-Cookie: zinc_session=eyJ2IjoxLCJleHAiOjE3OTA2NDExMzQsInZhbHVlcyI6eyJ1c2VyIjoiYWRhIn19.aWZjCCEUSO7r9Pq-8kS_HSmDmjrx9LvyrkNiaiAbE-M; Path=/; HttpOnly; SameSite=Lax

curl localhost:8080/account -b cookies.txt
# <p>Hello, ada.</p>
# <form method="post" action="/logout"><button>Log out</button></form>
```

Log out, and the page sends you back to the form:

```bash
curl -i -X POST localhost:8080/logout -c cookies.txt -b cookies.txt
# HTTP/1.1 303 See Other
# Location: /login
# Set-Cookie: zinc_session=eyJ2IjoxLCJleHAiOjE3OTA2NDExMzQsInZhbHVlcyI6e319.YUAPfUJwaYVHzCntqj0oxivR51PHFAChWmJ8zeAHDto; Path=/; HttpOnly; SameSite=Lax

curl -i localhost:8080/account -b cookies.txt
# HTTP/1.1 302 Found
# Location: /login
```

A wrong password gets `401`, and a cookie that wasn't signed with your secret gets `400` and is cleared:

```bash
curl -i localhost:8080/login -d username=ada -d password=nope
# HTTP/1.1 401 Unauthorized
#
# {"error":{"status":401,"message":"wrong username or password"}}

curl -i localhost:8080/account -b 'zinc_session=eyJ2IjoxfQ.forged'
# HTTP/1.1 400 Bad Request
# Set-Cookie: zinc_session=; Path=/; Expires=Thu, 01 Jan 1970 00:00:01 GMT; Max-Age=0; HttpOnly; SameSite=Lax
#
# {"error":{"status":400,"message":"Bad Request"}}
```

## How it works

- `app.Use(session.New(...))` loads the `zinc_session` cookie on every request and checks its signature. `session.MustGet(c)` returns that request's session.
- `Set("user", name)` writes the new signed cookie into the response headers immediately, so call it before `c.Redirect` or any other output.
- `requireLogin` runs before every route in the `/account` group. With no `user` in the session it redirects to `/login`; otherwise `c.Next()` runs the page.
- `c.Status(zinc.StatusSeeOther).Redirect(...)` answers a form `POST` with `303`, so the browser follows it with a `GET` and a reload doesn't resubmit the form.
- `Delete("user")` on logout writes a cookie with no values, so the next request looks logged out.

:::caution[Signed, not encrypted]
Anyone holding the cookie can read it. The first part is base64 JSON: `echo eyJ2IjoxLCJleHAiOjE3OTA2NDExMzQsInZhbHVlcyI6eyJ1c2VyIjoiYWRhIn19 | base64 -d` prints `{"v":1,"exp":1790641134,"values":{"user":"ada"}}`. Store an ID in the session, never a password or personal data.
:::

## Before production

- Set `SESSION_SECRET` to at least 32 random bytes, remove the local fallback, and set `Secure: true` so the cookie is sent over HTTPS only.
- Store password hashes with `golang.org/x/crypto/bcrypt` or argon2, not the plain passwords in `users`.
- Logging out clears the browser's cookie, but a copy taken before logout stays valid until it expires (24 hours by default). To end sessions on the server, keep a session ID in the cookie and the session itself in your database.
- Add the [CSRF middleware](/middleware/csrf/) so another site can't submit your forms, and a [rate limiter](/middleware/limiter/) on `POST /login` to slow down password guessing.

## See also

- [Session middleware](/middleware/session/): every config field, key rotation and cookie size limits.
- [Cookies](/guide/cookies/): read and set cookies without the session middleware.
- [JWT](/cookbook/jwt/): authenticate API clients with bearer tokens instead.
