---
title: Session
description: Remember small values, such as a signed-in user's ID, between requests in a signed cookie.
---

Session lets you remember a few small values between requests, such as the signed-in user's ID or a flash message. The values live in a signed cookie in the browser, so you need no session store on the server. For anything large or private, keep it in your database and put only its ID in the session.

## Usage

```go
import "github.com/0mjs/zinc/middleware/session"

app.Use(session.New(session.Config{
	Secret: []byte(os.Getenv("SESSION_SECRET")), // at least 32 random bytes
}))

app.Post("/login", func(c *zinc.Context) error {
	if err := session.MustGet(c).Set("user_id", "42"); err != nil {
		return err
	}
	return c.String("signed in")
})

app.Get("/me", func(c *zinc.Context) error {
	return c.JSON(zinc.Map{"user_id": session.MustGet(c).Get("user_id")})
})
```

```bash
curl -i -c jar -X POST http://localhost:8080/login
# HTTP/1.1 200 OK
# Set-Cookie: zinc_session=eyJ2IjoxLCJleHAiOjE3OTA2NDEwMjUsInZhbHVlcyI6eyJ1c2VyX2lkIjoiNDIifX0.IaMoQ3vdDzFWpi5mJ-ijnWVg6wLv3qxx8GwQwWhVEN8; Path=/; HttpOnly; SameSite=Lax
# signed in

curl -b jar http://localhost:8080/me
# {"user_id":"42"}

curl http://localhost:8080/me
# {"user_id":""}
```

A request without a session cookie gets an empty session, and `Get` returns `""` for keys that aren't set.

Generate a secret with `openssl rand -base64 32` and keep it out of your code.

:::caution[Signed, not encrypted]
The signature stops anyone from changing the values, but anyone holding the cookie can read them. The part before the `.` is base64-encoded JSON:

```bash
echo eyJ2IjoxLCJleHAiOjE3OTA2NDEwMjUsInZhbHVlcyI6eyJ1c2VyX2lkIjoiNDIifX0 | base64 -d
# {"v":1,"exp":1790641025,"values":{"user_id":"42"}}
```

Never store passwords, tokens or personal data in a session.
:::

## Defaults

| Setting | Default |
|---|---|
| `Secret` | **required**: at least 32 bytes, or `New` panics |
| Cookie | `zinc_session`, `Path=/`, `HttpOnly`, `SameSite=Lax`, not `Secure` |
| Browser lifetime | until the browser closes (no `Max-Age`) |
| Server-checked lifetime | 24 hours from the last write |

## Configuration

A cookie that lasts an hour, only over HTTPS, while you rotate to a new secret:

```go
app.Use(session.New(session.Config{
	Secret:          []byte(os.Getenv("SESSION_SECRET")),
	PreviousSecrets: [][]byte{[]byte(os.Getenv("SESSION_SECRET_OLD"))},
	Name:            "sid",
	MaxAge:          3600,
	Secure:          true,
}))
```

```text
Set-Cookie: sid=eyJ2IjoxLCJleHAiOjE3OTA1NTg0MjIs...; Path=/; Expires=Mon, 28 Sep 2026 01:20:22 GMT; Max-Age=3600; HttpOnly; Secure; SameSite=Lax
```

| Field | Default | Meaning |
|---|---|---|
| `Secret` | required | Key that signs the cookie. At least 32 random bytes. |
| `PreviousSecrets` | none | Old keys still accepted while you rotate. Each at least 32 bytes. |
| `Name` | `zinc_session` (`session.DefaultName`) | Cookie name. |
| `Path` | `/` | Cookie path. |
| `Domain` | none | Cookie domain. |
| `MaxAge` | `0` | Cookie lifetime in seconds. `0` makes a browser-session cookie. A positive value also replaces `Lifetime`. |
| `Lifetime` | 24 hours | How long a session stays valid after its last write, checked on the server. At least one second; negative panics. |
| `Secure` | `false` | Send the cookie only over HTTPS. Turn on in production. |
| `DisableHTTPOnly` | `false` | Let JavaScript read the cookie. |
| `SameSite` | `Lax` | Cookie `SameSite` mode. |
| `Now` | `time.Now` | Clock used for expiry. For tests. |

An invalid cookie name, path or domain panics when `New` runs.

## Write values

```go
s := session.MustGet(c)

s.Get("user_id")          // "42", or "" if not set
s.Values()                // a copy of every value, as map[string]string
err := s.Set("flash", "Saved")
err = s.Delete("flash")
```

Values are strings. `Set` and `Delete` write the new `Set-Cookie` header straight away, so:

- **Call them before writing the response.** After `c.JSON(...)` or any other write, the headers have been sent and they return `session.ErrCommitted`.
- **Keep the session small.** If the cookie would exceed 4096 bytes, they return `session.ErrTooLarge`.

In both cases the session keeps its previous values. If you don't return the error, the middleware returns it after your handler, so it still reaches your [error handler](/guide/errors/) when the response hasn't been sent yet.

Responses aren't buffered: a handler can stream as usual once it has finished changing the session.

## Expiry

Each `Set` or `Delete` signs a new expiry time into the cookie: now plus `Lifetime`, or plus `MaxAge` when it's set. Reading doesn't extend it. The server checks this time on every request, so a copied cookie stops working when it runs out, even if the browser still has it.

Deleting the cookie in the browser doesn't revoke a copy someone else holds. To end sessions early, keep a session ID in the cookie and check it against your database.

## Rotate the secret

1. Set the new key as `Secret` and move the old one into `PreviousSecrets`.
2. Deploy. Cookies signed with the old key still work, and each one is signed again with the new key the next time it's used, keeping its original expiry.
3. After one `Lifetime` has passed, remove the old key.

## Errors

| Error | When | Client gets |
|---|---|---|
| `session.ErrInvalid` | The cookie has a bad signature, can't be decoded, or has expired | `400 Bad Request`, and the cookie is cleared |
| `session.ErrCommitted` | `Set` or `Delete` after the response was written | whatever was already sent |
| `session.ErrTooLarge` | The cookie would exceed 4096 bytes | `500`, unless you handle it |

```bash
curl -i -b 'zinc_session=tampered.value' http://localhost:8080/me
# HTTP/1.1 400 Bad Request
# Set-Cookie: zinc_session=; Path=/; Expires=Thu, 01 Jan 1970 00:00:01 GMT; Max-Age=0; HttpOnly; SameSite=Lax
# {"error":{"status":400,"message":"Bad Request"}}
```

Clearing the cookie means the browser's next request starts with an empty session, so an expired session doesn't block a fresh sign-in.

## Related

- [CSRF](/middleware/csrf/): protect cookie-authenticated forms and requests.
- [Cookies](/guide/cookies/): read and set other cookies.
- [Basic Auth](/middleware/basicauth/) and [JWT](/middleware/jwtauth/): authentication without cookies.
