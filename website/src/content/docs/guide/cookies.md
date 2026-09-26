---
title: Cookies
description: Set, read, and clear cookies with safe defaults, using the standard http.Cookie type.
---

Zinc uses the standard library's `http.Cookie` type, so every attribute works exactly as it does in `net/http`.

## Set a cookie

```go
app.Post("/login", func(c *zinc.Context) error {
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

For anything that identifies a user, set `HttpOnly`, `Secure`, and a `SameSite` mode. Browsers reject `SameSite=None` unless `Secure` is also set.

## Read cookies

```go
cookie, err := c.Cookie("session")
if errors.Is(err, http.ErrNoCookie) {
	return zinc.Unauthorized("sign in required")
}
if err != nil {
	return err
}
user, err := auth.UserFromToken(c.Context(), cookie.Value)
```

`c.Cookies()` returns every cookie on the request.

## Clear cookies

```go
c.ClearCookie(&http.Cookie{Name: "session"})
c.ClearCookie(&http.Cookie{Name: "prefs", Path: "/app", Domain: "example.com"})
return c.NoContent()
```

`ClearCookie` sends an expired copy of the cookie, so the browser deletes it. The name, path, and domain must match the cookie you set; the path defaults to `/`.

To give every cookie a default `SameSite` mode, set `Config.CookieSameSite`. It applies to cookies that don't set `SameSite` themselves.

## Related middleware

- [Session](/middleware/session/) stores small signed values in a cookie.
- [CSRF](/middleware/csrf/) protects cookie-authenticated forms and requests.
- [JWT](/middleware/jwtauth/) and [Key Auth](/middleware/keyauth/) can read tokens from cookies.
