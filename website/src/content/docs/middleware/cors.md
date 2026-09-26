---
title: CORS
description: Let browsers on other origins call your API, with correct preflight handling and safe credential rules.
---

Browsers block a page on one origin from reading responses from another unless the server allows it with CORS headers. `cors` sends those headers and answers preflight requests.

```go
import "github.com/0mjs/zinc/middleware/cors"

app.Use(cors.New(cors.Config{
	AllowOrigins: []string{"https://app.example.com", "https://admin.example.com"},
}))
```

Requests from the listed origins get `Access-Control-Allow-Origin`. Requests without an `Origin` header, such as server-to-server calls, pass through untouched. Preflight `OPTIONS` requests get `204 No Content`.

## Defaults

| Setting | Default |
|---|---|
| Origins | `*` |
| Methods | `GET`, `POST`, `HEAD`, `PUT`, `DELETE`, `PATCH` |
| Request headers | `Origin`, `Content-Type`, `Accept`, `Authorization` |
| Exposed headers | none |
| Credentials | not allowed |
| Preflight cache | not set |

## Config

Fields you leave empty keep the defaults above.

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

| Field | Meaning |
|---|---|
| `AllowOrigins` | Exact origins allowed, or `*` |
| `AllowMethods` | Methods allowed in preflight responses |
| `AllowHeaders` | Request headers allowed in preflight responses |
| `ExposeHeaders` | Response headers that browser code may read |
| `AllowCredentials` | Sends `Access-Control-Allow-Credentials: true` |
| `MaxAge` | Seconds a browser may cache a preflight result |

## Credentials

Cookies and `Authorization` headers are only sent cross-origin when you allow credentials. List every trusted origin explicitly when you do.

:::danger[Never combine * with credentials]
Reflecting any origin together with `Access-Control-Allow-Credentials: true` would let every website make authenticated requests with your users' cookies and read the responses. Zinc refuses that configuration: `AllowOrigins: ["*"]` with `AllowCredentials: true` panics when the middleware is created. List exact origins when credentials are allowed.
:::

With credentials enabled, an empty origin list denies all cross-origin access. Without credentials, it allows `*`. Partial configurations fill in default methods and headers, and a negative `MaxAge` panics.

## Next steps

- [CSRF](/middleware/csrf/) protects cookie-authenticated endpoints from forged requests.
- [Secure Headers](/middleware/secure/) sets the other browser security headers.
