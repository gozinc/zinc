---
title: Session
description: Store signed cookie-backed string session values.
---

`SessionCookie` keeps small string values, such as a user ID or a flash message, in a signed cookie, so the server stores nothing.

```go
app.Use(middleware.SessionCookie("sid", os.Getenv("SESSION_SECRET")))
```

Inside handlers:

```go
app.Get("/profile", func(c *zinc.Context) error {
	session := middleware.MustSession(c)
	session.Set("last_path", c.Path())
	return c.String(session.Get("user_id"))
})
```

Use `SessionWithConfig` to customize cookie settings.

```go
app.Use(middleware.SessionWithConfig(middleware.SessionConfig{
	Name:     "sid",
	Secret:   []byte(os.Getenv("SESSION_SECRET")),
	MaxAge:   86400,
	Secure:   true,
	SameSite: http.SameSiteLaxMode,
}))
```

:::caution[Signed, not encrypted]
The signature stops clients from changing values, but the cookie is base64-encoded JSON that anyone can decode and read. Never store secrets or personal data in it. For larger or sensitive state, store it server-side and keep only an opaque ID in the cookie.
:::

Keep the secret long and random, and rotate it by deploying a new one. Existing sessions end when it changes.
