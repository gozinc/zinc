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
	if err := session.Set("last_path", c.Path()); err != nil {
		return err
	}
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

Use a signing secret containing at least 32 random bytes. `Set` and `Delete` return errors and update cookie headers immediately, so call them before writing or flushing the response. Late changes return `ErrSessionCommitted`; cookies over 4096 bytes return `ErrSessionTooLarge`. Responses are streamed normally without session buffering.

Expiry is authenticated and checked on the server. A positive `MaxAge` sets the lifetime. Browser-session cookies (`MaxAge: 0`) use `Lifetime`, which defaults to 24 hours. Client-side cookie deletion does not revoke an already copied cookie.

For key rotation, configure the new `Secret` and temporarily retain old keys in `PreviousSecrets`. Cookies verified with an old key are signed again using the current key without extending their expiry. Remove previous keys after the rotation window.

The 0.3 cookie format includes a version and expiry. Older cookies are invalidated on upgrade, so users must sign in again.
