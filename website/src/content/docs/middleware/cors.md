---
title: CORS
description: Let browser code on other origins call your API, with preflight answered for you.
---

CORS lets a web page on one origin, such as `https://app.example.com`, read responses from your API on another. Add it when a browser front end calls your API from a different host or port. Server-to-server clients and same-origin pages don't need it.

## Usage

```go
import "github.com/0mjs/zinc/middleware/cors"

app.Use(cors.New(cors.Config{
	AllowOrigins: []string{"https://app.example.com", "https://admin.example.com"},
}))
```

A request from a listed origin gets `Access-Control-Allow-Origin` back:

```bash
curl -i http://localhost:8080/users -H "Origin: https://app.example.com"
# HTTP/1.1 200 OK
# Access-Control-Allow-Origin: https://app.example.com
# Content-Type: application/json; charset=utf-8
# Vary: Origin
# Vary: Access-Control-Request-Method
# Vary: Access-Control-Request-Headers
#
# {"users":[]}
```

A browser sends a preflight `OPTIONS` request before a `PATCH` or a request with a JSON body. `cors` answers it with `204 No Content`, so you don't register `OPTIONS` routes:

```bash
curl -i -X OPTIONS http://localhost:8080/users/42 \
  -H "Origin: https://app.example.com" \
  -H "Access-Control-Request-Method: PATCH" \
  -H "Access-Control-Request-Headers: content-type"
# HTTP/1.1 204 No Content
# Access-Control-Allow-Headers: Origin,Content-Type,Accept,Authorization
# Access-Control-Allow-Methods: GET,POST,HEAD,PUT,DELETE,PATCH
# Access-Control-Allow-Origin: https://app.example.com
# Vary: Origin
# Vary: Access-Control-Request-Method
# Vary: Access-Control-Request-Headers
```

Register it with `app.Use`, so preflight requests are answered even for paths that only have a `PATCH` or `DELETE` route.

## Defaults

With no config, `cors.New()` allows any origin without credentials.

| Setting | Default |
|---|---|
| Origins | `*` (none when credentials are allowed) |
| Methods | `GET`, `POST`, `HEAD`, `PUT`, `DELETE`, `PATCH` |
| Request headers | `Origin`, `Content-Type`, `Accept`, `Authorization` |
| Exposed headers | none |
| Credentials | not allowed |
| Preflight cache | not set, so the browser uses its own default |

## Configuration

A front end that signs in with cookies and reads a request ID from responses:

```go
app.Use(cors.New(cors.Config{
	AllowOrigins:     []string{"https://app.example.com"},
	AllowMethods:     []string{"GET", "POST", "PATCH", "DELETE"},
	AllowHeaders:     []string{"Authorization", "Content-Type", "X-Request-ID"},
	ExposeHeaders:    []string{"X-Request-ID"},
	AllowCredentials: true,
	MaxAge:           600, // seconds
}))
```

```bash
curl -i -X OPTIONS http://localhost:8080/me \
  -H "Origin: https://app.example.com" \
  -H "Access-Control-Request-Method: PATCH" \
  -H "Access-Control-Request-Headers: authorization,content-type"
# HTTP/1.1 204 No Content
# Access-Control-Allow-Credentials: true
# Access-Control-Allow-Headers: Authorization,Content-Type,X-Request-ID
# Access-Control-Allow-Methods: GET,POST,PATCH,DELETE
# Access-Control-Allow-Origin: https://app.example.com
# Access-Control-Max-Age: 600
# ...

curl -i http://localhost:8080/me -H "Origin: https://app.example.com"
# HTTP/1.1 200 OK
# Access-Control-Allow-Credentials: true
# Access-Control-Allow-Origin: https://app.example.com
# Access-Control-Expose-Headers: X-Request-ID
# X-Request-Id: abc123
# ...
```

Fields you leave empty keep their defaults.

| Field | Default | Meaning |
|---|---|---|
| `AllowOrigins` | `["*"]` | Exact origins allowed, such as `https://app.example.com`, or `*` for any. Empty with `AllowCredentials` means no origin is allowed. |
| `AllowMethods` | `GET`, `POST`, `HEAD`, `PUT`, `DELETE`, `PATCH` | Methods listed in `Access-Control-Allow-Methods` on preflight responses |
| `AllowHeaders` | `Origin`, `Content-Type`, `Accept`, `Authorization` | Request headers listed in `Access-Control-Allow-Headers` on preflight responses |
| `ExposeHeaders` | none | Response headers browser code may read, sent as `Access-Control-Expose-Headers` |
| `AllowCredentials` | `false` | Sends `Access-Control-Allow-Credentials: true`, so the browser includes cookies and reads the response |
| `MaxAge` | `0` (not sent) | Seconds a browser may cache a preflight answer, sent as `Access-Control-Max-Age` |

## Errors

`cors` never rejects a request itself. A request from an origin that isn't listed continues without CORS headers, and the browser then blocks the page from reading the response:

```bash
curl -i http://localhost:8080/users -H "Origin: https://evil.example"
# HTTP/1.1 200 OK
# Content-Type: application/json; charset=utf-8
# Vary: Origin
# ...
```

Requests without an `Origin` header, such as `curl` or server-to-server calls, pass through the same way.

`cors.New` panics at startup when:

- `AllowOrigins` contains `*` and `AllowCredentials` is `true`
- `MaxAge` is negative
- it's given more than one `Config`

## Security

Cookies and `Authorization` headers only travel cross-origin when you allow credentials. List every trusted origin when you do.

:::danger[Never combine * with credentials]
Allowing any origin together with credentials would let every website make requests with your users' cookies and read the responses. Zinc refuses that configuration: `AllowOrigins: []string{"*"}` with `AllowCredentials: true` panics when the middleware is created.
:::

`cors` always adds the three `Vary` headers, so a shared cache doesn't serve one origin's answer to another.

## Related

- [CSRF](/middleware/csrf/): protects cookie-authenticated endpoints from forged requests.
- [Secure Headers](/middleware/secure/): sets the other browser security headers.
- [Groups and Middleware](/guide/groups-and-middleware/): where app, prefix and group middleware run.
