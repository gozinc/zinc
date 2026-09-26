---
title: Session
description: Store signed cookie-backed string session values.
---

`session` keeps small string values, such as a user ID or a flash message, in a signed cookie, so the server stores nothing.

```go
import "github.com/0mjs/zinc/middleware/session"

app.Use(session.New(session.Config{
	Secret: []byte(os.Getenv("SESSION_SECRET")),
}))
```

`Secret` is required and must hold at least 32 random bytes. Inside handlers:

```go
app.Get("/profile", func(c *zinc.Context) error {
	s := session.MustGet(c)
	if err := s.Set("last_path", c.Path()); err != nil {
		return err
	}
	return c.String(s.Get("user_id"))
})
```

## Config

| Field | Default | Meaning |
|---|---|---|
| `Secret` | required | Signs the cookie |
| `PreviousSecrets` | none | Verification-only keys during a rotation |
| `Name` | `zinc_session` | Cookie name |
| `Path` | `/` | Cookie path |
| `Domain` | none | Cookie domain |
| `MaxAge` | `0` | Cookie lifetime in seconds; `0` is a browser-session cookie |
| `Lifetime` | 24 hours | Server-checked lifetime of a browser-session cookie |
| `Secure` | `false` | Sends the cookie over HTTPS only |
| `DisableHTTPOnly` | `false` | Lets scripts read the cookie |
| `SameSite` | `Lax` | Cookie SameSite mode |

:::caution[Signed, not encrypted]
The signature stops clients from changing values, but the cookie is base64-encoded JSON that anyone can decode and read. Never store secrets or personal data in it. For larger or sensitive state, store it server-side and keep only an opaque ID in the cookie.
:::

`Set` and `Delete` return errors and update cookie headers immediately, so call them before writing or flushing the response. Late changes return `session.ErrCommitted`; cookies over 4096 bytes return `session.ErrTooLarge`. Responses are streamed normally without session buffering.

Expiry is authenticated and checked on the server. A positive `MaxAge` sets the lifetime. Client-side cookie deletion does not revoke an already copied cookie.

For key rotation, configure the new `Secret` and temporarily keep old keys in `PreviousSecrets`. Cookies verified with an old key are signed again with the current key without extending their expiry. Remove previous keys after the rotation window.
