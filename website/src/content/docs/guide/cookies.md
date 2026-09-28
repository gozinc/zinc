---
title: Cookies
description: Set, read, and clear cookies, and give every cookie a safe SameSite default.
---

Cookies let the browser remember something between requests, such as a session token or a theme. Use them when the browser should send a value back on every request without your front end doing anything.

```go
c.SetCookie(&http.Cookie{Name: "theme", Value: "dark", Path: "/"})
```

```bash
curl -i http://localhost:8080/theme
# HTTP/1.1 204 No Content
# Set-Cookie: theme=dark; Path=/
```

Cookies are the standard library's `http.Cookie`, so every attribute works as it does in `net/http`.

## Set a cookie

A sign-in handler that stores a session token:

```go
app.Post("/login", func(c *zinc.Context) error {
	// auth is your own sign-in code
	token, err := auth.SignIn(c.Context(), c.FormValue("email"), c.FormValue("password"))
	if err != nil {
		return zinc.ErrUnauthorized
	}

	c.SetCookie(&http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   int((24 * time.Hour).Seconds()),
		HttpOnly: true,                 // not readable from JavaScript
		Secure:   true,                 // HTTPS only
		SameSite: http.SameSiteLaxMode, // not sent on most cross-site requests
	})
	return c.NoContent()
})
```

```bash
curl -i -X POST http://localhost:8080/login -d email=ada@example.com -d password=secret
# HTTP/1.1 204 No Content
# Set-Cookie: session=abc123; Path=/; Max-Age=86400; HttpOnly; Secure; SameSite=Lax
```

For anything that identifies a user, set `HttpOnly`, `Secure` and a `SameSite` mode.

:::caution[SameSite=None needs Secure]
Browsers reject a cookie with `SameSite=None` unless `Secure` is also set.
:::

## Read a cookie

`c.Cookie` returns the named cookie, or `http.ErrNoCookie` when the request doesn't have it:

```go
app.Get("/me", func(c *zinc.Context) error {
	cookie, err := c.Cookie("session")
	if errors.Is(err, http.ErrNoCookie) {
		return zinc.Unauthorized("sign in required")
	}
	if err != nil {
		return err
	}
	return c.JSON(zinc.Map{"session": cookie.Value})
})
```

```bash
curl http://localhost:8080/me
# {"error":{"status":401,"message":"sign in required"}}

curl -b session=abc123 http://localhost:8080/me
# {"session":"abc123"}
```

`c.Cookies()` returns every cookie on the request, in the order the browser sent them.

## Clear a cookie

`ClearCookie` sends an expired, empty copy of the cookie, so the browser deletes it:

```go
app.Post("/logout", func(c *zinc.Context) error {
	c.ClearCookie(&http.Cookie{Name: "session"})
	c.ClearCookie(&http.Cookie{Name: "prefs", Path: "/app", Domain: "example.com"})
	return c.NoContent()
})
```

```bash
curl -i -X POST http://localhost:8080/logout
# HTTP/1.1 204 No Content
# Set-Cookie: session=; Path=/; Expires=Thu, 01 Jan 1970 00:00:01 GMT; Max-Age=0
# Set-Cookie: prefs=; Path=/app; Domain=example.com; Expires=Thu, 01 Jan 1970 00:00:01 GMT; Max-Age=0
```

:::caution[Match the name, path and domain]
The browser deletes a cookie only when the name, path and domain all match the cookie you set. If you leave `Path` empty, `ClearCookie` uses `/`.
:::

## Set a default SameSite

To give every cookie a `SameSite` mode without repeating it, set `CookieSameSite` in [`zinc.Config`](/guide/configuration/):

```go
app := zinc.New(zinc.Config{
	CookieSameSite: http.SameSiteLaxMode,
})

app.Get("/theme", func(c *zinc.Context) error {
	c.SetCookie(&http.Cookie{Name: "theme", Value: "dark", Path: "/"})
	c.SetCookie(&http.Cookie{Name: "embed", Value: "1", Path: "/", Secure: true, SameSite: http.SameSiteNoneMode})
	return c.NoContent()
})
```

```bash
curl -i http://localhost:8080/theme
# HTTP/1.1 204 No Content
# Set-Cookie: theme=dark; Path=/; SameSite=Lax
# Set-Cookie: embed=1; Path=/; Secure; SameSite=None
```

The default applies to cookies written with `c.SetCookie` and `c.ClearCookie` that don't set `SameSite` themselves. A cookie that sets its own mode keeps it. Without `CookieSameSite`, cookies go out with no `SameSite` attribute.

## Next steps

- [Session](/middleware/session/): store small signed values in a cookie.
- [CSRF](/middleware/csrf/): protect cookie-authenticated forms and requests.
- [JWT](/middleware/jwtauth/) and [Key Auth](/middleware/keyauth/): read tokens from cookies.
