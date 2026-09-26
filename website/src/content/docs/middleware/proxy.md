---
title: Proxy
description: Reverse proxy requests with net/http/httputil.
---

`proxy` forwards requests through `httputil.ReverseProxy` and ends the chain there.

```go
import "github.com/0mjs/zinc/middleware/proxy"

app.UsePrefix("/upstream", proxy.New(proxy.Config{Target: "https://api.example.com"}))
```

One of `Target`, `Targets`, or `Balancer` is required. Zinc rewrites the request scheme, host, and path through the standard library reverse proxy.

Use `Director` and `ModifyResponse` when the request or response needs adjustment.

```go
app.Use(proxy.New(proxy.Config{
	Target: "https://api.example.com",
	Director: func(req *http.Request) {
		req.Header.Set("X-Forwarded-App", "zinc")
	},
	ModifyResponse: func(resp *http.Response) error {
		resp.Header.Del("Server")
		return nil
	},
}))
```

## Multiple targets

`proxy` can balance across multiple targets. `Targets` uses round-robin by default.

```go
apiA, _ := url.Parse("https://api-a.example.com")
apiB, _ := url.Parse("https://api-b.example.com")

app.Use(proxy.New(proxy.Config{
	Targets: []*proxy.Target{
		{Name: "a", URL: apiA},
		{Name: "b", URL: apiB},
	},
}))
```

Use an explicit balancer when the choice should be obvious from the config.

```go
targets := []*proxy.Target{
	{Name: "a", URL: apiA},
	{Name: "b", URL: apiB},
}

app.Use(proxy.New(proxy.Config{
	Balancer: proxy.NewRoundRobinBalancer(targets),
}))
```

For random target selection:

```go
app.Use(proxy.New(proxy.Config{
	Balancer: proxy.NewRandomBalancer(targets),
}))
```

## Rewrites

Use `Rewrite` for exact paths or `*` suffix rules.

```go
app.UsePrefix("/api", proxy.New(proxy.Config{
	Target: "https://api.example.com",
	Rewrite: map[string]string{
		"/api/*": "/v1/*",
	},
}))
```

Use `RegexRewrite` when wildcard rules are not expressive enough.

```go
app.UsePrefix("/api", proxy.New(proxy.Config{
	Target: "https://api.example.com",
	RegexRewrite: map[*regexp.Regexp]string{
		regexp.MustCompile(`^/api/users/(\d+)$`): "/v1/users/$1",
	},
}))
```

## Retries

Retries are opt-in.

```go
app.Use(proxy.New(proxy.Config{
	Target:  "https://api.example.com",
	Retries: 2,
	RetryFilter: func(c *zinc.Context, err error) bool {
		return c.Method() == zinc.MethodGet
	},
}))
```

Zinc only retries when the request body can be replayed safely: no body, `http.NoBody`, or a request with `GetBody` configured.

## Transport

Pass a custom transport for timeouts, tracing, connection pools, or tests.

```go
app.Use(proxy.New(proxy.Config{
	Target:    "https://api.example.com",
	Transport: customTransport,
}))
```

The target must be a valid absolute URL.

Inbound `Forwarded` and `X-Forwarded-*` headers are removed before Zinc sets fresh forwarding headers from the direct request. `Director` executes through the standard library's `Rewrite` phase, after hop-by-hop headers are removed, so a client cannot remove a director-added identity header through `Connection`.

Retries require a replayable body. By default only GET, HEAD, OPTIONS, TRACE, PUT, and DELETE can retry after transport errors. A custom `RetryFilter` explicitly controls method policy, including any opt-in for POST/PATCH; callers must account for possible duplicate upstream execution. Cancellation stops retries.
