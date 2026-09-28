---
title: Basic Auth
description: Protect routes with a username and password, using the browser's built-in login prompt.
---

Basic Auth puts a username and password in front of a set of routes, using the login prompt every browser already has. Add it to admin panels, internal tools and operational endpoints; for API clients, [Key Auth](/middleware/keyauth/) or [JWT](/middleware/jwtauth/) fit better.

## Usage

```go
import "github.com/0mjs/zinc/middleware/basicauth"

admin := app.Group("/admin", basicauth.New(basicauth.Config{
	Validator: basicauth.Static("admin", os.Getenv("ADMIN_PASSWORD")),
}))

admin.Get("/stats", func(c *zinc.Context) error {
	return c.JSON(zinc.Map{"user": basicauth.MustGet(c).Username})
})
```

```bash
curl -i http://localhost:8080/admin/stats
# HTTP/1.1 401 Unauthorized
# Www-Authenticate: Basic realm="Restricted"
# {"error":{"status":401,"message":"Unauthorized"}}

curl -u admin:s3cret http://localhost:8080/admin/stats
# {"user":"admin"}
```

The `WWW-Authenticate` header is what makes a browser show its login prompt.

:::caution[Use HTTPS]
Basic credentials are only base64-encoded, not encrypted. Anyone who can read the traffic can read the password, so serve these routes over HTTPS.
:::

## Defaults

| Setting | Default |
|---|---|
| `Validator` | **required**: `New` panics without it |
| Where credentials come from | `Authorization: Basic <base64>` header |
| Realm | `Restricted` |
| On success | calls `c.Next()` |
| On failure | `401` with a `WWW-Authenticate` challenge |

## Configuration

Accept several users, name the realm, and read credentials from a second header when a proxy has taken `Authorization`:

```go
app.Use(basicauth.New(basicauth.Config{
	Realm: "Ops",
	Extractor: basicauth.FromFirst(
		basicauth.FromAuthorizationHeader(),
		basicauth.FromHeaderPrefix("X-Ops-Auth", "Basic "),
	),
	Validator: basicauth.StaticPairs(
		basicauth.Pair{Username: "alice", Password: os.Getenv("ALICE_PASSWORD")},
		basicauth.Pair{Username: "bob", Password: os.Getenv("BOB_PASSWORD")},
	),
}))
```

```bash
curl -i http://localhost:8080/
# HTTP/1.1 401 Unauthorized
# Www-Authenticate: Basic realm="Ops"

curl -s -o /dev/null -w '%{http_code}\n' \
  -H 'X-Ops-Auth: Basic Ym9iOmItcGFzcw==' http://localhost:8080/   # bob:b-pass
# 200
```

| Field | Default | Meaning |
|---|---|---|
| `Validator` | required | Checks the username and password. Returns `(true, nil)` to let the request through. |
| `Extractor` | `FromAuthorizationHeader()` | Reads the credentials from the request. |
| `Realm` | `"Restricted"` | The realm named in the `WWW-Authenticate` challenge. |
| `SuccessHandler` | calls `c.Next()` | Runs after the credentials pass. Call `c.Next()` in it to continue the chain. |
| `ErrorHandler` | `401` with challenge | Runs when extraction or validation fails, with the error. Its return value is what the client gets. |

## Check credentials

`basicauth.Static(username, password)` accepts one pair, and `basicauth.StaticPairs(pairs...)` accepts any pair in a list. Both compare in constant time, so response timing doesn't reveal how much of a guess was right.

To check against your own store, write a `Validator`:

```go
Validator: func(c *zinc.Context, cred basicauth.Credentials) (bool, error) {
	return users.CheckPassword(c.Context(), cred.Username, cred.Password) // your data layer
},
```

Return `(false, nil)` for wrong credentials; the client gets a `401`. Return an error only when the check itself failed, such as a database outage. That error goes to the app's error handler, so the client gets a `500`.

## Read credentials from another header

The default reads the `Authorization` header. Other extractors:

| Extractor | Reads |
|---|---|
| `basicauth.FromAuthorizationHeader()` | `Authorization: Basic <base64>` |
| `basicauth.FromHeader(name)` | The whole header value as `<base64>` |
| `basicauth.FromHeaderPrefix(name, prefix)` | The value after `prefix`, matched without regard to case |
| `basicauth.FromFirst(extractors...)` | The first extractor that finds credentials |

`FromFirst` moves to the next extractor only when credentials are missing. Malformed credentials stop the search, so a bad value in one header can't be bypassed by a good one in another.

## Reading state

After the credentials pass, handlers can read who signed in:

```go
identity, ok := basicauth.Get(c) // ok is false without the middleware
identity := basicauth.MustGet(c) // panics without the middleware

identity.Username // "admin"
identity.Source   // basicauth.SourceAuthorizationHeader or basicauth.SourceHeader
```

The password isn't stored.

## Errors

The default `ErrorHandler` answers `401` with `WWW-Authenticate: Basic realm="..."` for these errors:

| Error | When |
|---|---|
| `basicauth.ErrCredentialsMissing` | No credentials in the request |
| `basicauth.ErrCredentialsMalformed` | Not valid base64, or no `:` between username and password |
| `basicauth.ErrCredentialsInvalid` | The validator returned `false` |

Any other error from your validator is returned as is. A custom `ErrorHandler` can tell the cases apart with `errors.Is`:

```go
ErrorHandler: func(c *zinc.Context, err error) error {
	if errors.Is(err, basicauth.ErrCredentialsMissing) {
		return c.Redirect("/login")
	}
	return zinc.ErrUnauthorized
},
```

```bash
curl -i http://localhost:8080/
# HTTP/1.1 302 Found
# Location: /login
```

A custom handler replaces the challenge too: set `WWW-Authenticate` yourself if you still want the browser prompt.

## Related

- [Key Auth](/middleware/keyauth/): opaque API keys for machine clients.
- [JWT](/middleware/jwtauth/): signed bearer tokens that carry claims.
- [Casbin Auth](/middleware/casbin/): decide what a signed-in user may do, using `casbin.SubjectFromBasicAuth()`.
- [Groups and Middleware](/guide/groups-and-middleware/): attach auth to one group of routes.
