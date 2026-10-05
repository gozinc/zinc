---
title: Proxy
description: Forward requests to another server and send its response back to the client.
---

Proxy forwards requests to another HTTP server and relays its response, so one Zinc app can front an older service, a separate API or a pool of backends. It uses the standard library's `httputil.ReverseProxy`, and the request ends there: handlers after it don't run.

## Usage

```go
import "github.com/0mjs/zinc/middleware/proxy"

app.UsePrefix("/api", proxy.New(proxy.Config{Target: "http://localhost:9000"}))
```

With an upstream on port 9000 that echoes what it receives:

```bash
curl -i "http://localhost:8080/api/users/42?fields=name" -H "X-Forwarded-For: 6.6.6.6"
# HTTP/1.1 200 OK
# Content-Type: application/json
# Server: upstream/1.0
#
# {"host":"localhost:8080","path":"/api/users/42","query":"fields=name",
#  "x_forwarded_for":"::1","x_forwarded_host":"localhost:8080","x_forwarded_proto":"http"}

curl -i http://localhost:8080/other
# HTTP/1.1 404 Not Found
```

The upstream gets the full path, `/api` included, and the query string. The client's own `X-Forwarded-For` is dropped and replaced with the address Zinc saw (see [Security](#security)).

Use `app.UsePrefix` for a path subtree, so requests are forwarded even when no Zinc route matches them. `app.Use` forwards everything.

## Defaults

One of `Target`, `Targets` or `Balancer` is **required**.

| Setting | Default |
|---|---|
| Target selection | round-robin across `Targets` |
| Upstream path | the request path, after the target URL's own path |
| Retries | none |
| Unreachable upstream | `502 Bad Gateway`, empty body |
| Transport | `http.DefaultTransport` |

## Configuration

Change the outgoing request and the returned response:

```go
app.Use(proxy.New(proxy.Config{
	Target: "http://localhost:9000",
	Director: func(req *http.Request) {
		req.Header.Set("X-Forwarded-App", "zinc")
	},
	ModifyResponse: func(resp *http.Response) error {
		resp.Header.Del("Server")
		return nil
	},
}))
```

The upstream now sees `X-Forwarded-App: zinc`, and the client no longer gets its `Server: upstream/1.0` header.

| Field | Default | Meaning |
|---|---|---|
| `Target` | none | One absolute upstream URL, such as `https://api.example.com`. A path or query in it is prepended to every request's. |
| `Targets` | none | Several upstreams, used round-robin. Each is a `*proxy.Target` with a `Name` and an absolute `URL`. |
| `Balancer` | round-robin over `Targets` | Picks the upstream for each request. See [Spread requests across servers](#spread-requests-across-servers). |
| `Director` | none | `func(*http.Request)` that edits the outgoing request, after Zinc sets the URL and forwarding headers |
| `ModifyResponse` | none | `func(*http.Response) error` that edits the upstream response before it's sent. An error goes to `ErrorHandler`. |
| `ErrorHandler` | `502 Bad Gateway`, empty body | `func(http.ResponseWriter, *http.Request, error)` that writes the response when the upstream can't be reached or `ModifyResponse` fails |
| `Retries` | `0` | Extra attempts after a connection error. See [Retry failed connections](#retry-failed-connections). |
| `RetryFilter` | retries `GET`, `HEAD`, `OPTIONS`, `TRACE`, `PUT`, `DELETE` | `func(*zinc.Context, error) bool` that decides whether an attempt is retried |
| `Rewrite` | none | Path rules, exact or ending in `*`. See [Rewrite the upstream path](#rewrite-the-upstream-path). |
| `RegexRewrite` | none | Regular-expression path rules, tried when no `Rewrite` rule matches |
| `Transport` | `http.DefaultTransport` | The `http.RoundTripper` that sends the request: set it for timeouts, TLS settings, connection pools or tests |

The upstream receives the client's `Host` header (`localhost:8080` in [Usage](#usage)), not the target's. Upstreams that serve several sites by host name need their own. Clear `Host` in a `Director` and Go sends the target's:

```go
app.Use(proxy.New(proxy.Config{
	Target: "https://api.example.com",
	Director: func(req *http.Request) {
		req.Host = "" // Host: api.example.com
	},
}))
```

## Rewrite the upstream path

Send `/api/...` to the upstream's `/v1/...`:

```go
app.UsePrefix("/api", proxy.New(proxy.Config{
	Target: "http://localhost:9000",
	Rewrite: map[string]string{
		"/api/*": "/v1/*",
	},
}))
// GET /api/users/42 → upstream GET /v1/users/42
```

A rule ending in `*` matches a prefix, and a `*` in the target is replaced by the rest of the path. When wildcard rules aren't enough, use `RegexRewrite`, with `$1`-style references to capture groups:

```go
app.UsePrefix("/api", proxy.New(proxy.Config{
	Target: "http://localhost:9000",
	RegexRewrite: map[*regexp.Regexp]string{
		regexp.MustCompile(`^/api/users/(\d+)$`): "/v1/users/$1",
	},
}))
// GET /api/users/42 → upstream GET /v1/users/42
// GET /api/users/me → upstream GET /api/users/me (no rule matched)
```

Rules see the path after the target URL's own path is added. With `Target: "http://localhost:9000/base?key=1"`, a request for `/users?x=2` reaches the upstream as `/base/users?key=1&x=2`.

The upstream gets the path with the escaping the client sent, so `/objects/a%2Fb` stays one segment instead of becoming `/objects/a/b`; an escaped target path such as `/base%2Froot` is kept the same way. Rules match the decoded path. The fixed part of a rule's target is escaped as usual, and the part a `*` carries over keeps its escaping: with `"/api/*": "/v1/*"`, `/api/a%2Fb` reaches the upstream as `/v1/a%2Fb`. A `RegexRewrite` result keeps the escaping when the same rule, applied to the escaped path, gives the same path; otherwise it's escaped as usual.

:::note[Overlapping rules]
An exact `Rewrite` rule always wins, and among `*` rules the longest prefix wins. `Rewrite` rules are tried before `RegexRewrite`. A map of regular expressions has no order, so Zinc gives it one: the longest pattern is tried first, then patterns in alphabetical order, and the first match wins. Two rules with the same pattern make `proxy.New` panic.
:::

## Spread requests across servers

List several targets and requests go to each in turn:

```go
apiA, _ := url.Parse("http://localhost:9001")
apiB, _ := url.Parse("http://localhost:9002")

app.Use(proxy.New(proxy.Config{
	Targets: []*proxy.Target{
		{Name: "a", URL: apiA},
		{Name: "b", URL: apiB},
	},
}))
// Three requests go to a, b, a.
```

To choose the strategy yourself, pass a `Balancer`. Zinc has round-robin and random ones, both safe for concurrent use:

```go
targets := []*proxy.Target{
	{Name: "a", URL: apiA},
	{Name: "b", URL: apiB},
}

app.Use(proxy.New(proxy.Config{
	Balancer: proxy.NewRandomBalancer(targets), // or proxy.NewRoundRobinBalancer(targets)
}))
```

Any type with a `Next(*zinc.Context) (*proxy.Target, error)` method is a `Balancer`, so you can route by header, user or health. An error from `Next` goes to the app's error handler.

## Retry failed connections

```go
app.Use(proxy.New(proxy.Config{
	Target:  "http://localhost:9000",
	Retries: 2,
}))
```

With `Retries: 2`, a `GET` whose connection fails is tried three times in all before the client gets `502`. Only connection and transport errors are retried. An upstream that answers, even with a `500`, isn't retried.

By default only `GET`, `HEAD`, `OPTIONS`, `TRACE`, `PUT` and `DELETE` are retried. Set `RetryFilter` to decide yourself:

```go
app.Use(proxy.New(proxy.Config{
	Target:  "http://localhost:9000",
	Retries: 2,
	RetryFilter: func(c *zinc.Context, err error) bool {
		return c.Method() == zinc.MethodGet
	},
}))
```

A request with a body is sent once, whatever `RetryFilter` says, because the incoming body can't be read a second time. Retries stop when the client cancels.

:::caution[Retrying POST and PATCH]
If your `RetryFilter` allows `POST` or `PATCH`, the upstream may run the request twice: a connection can fail after the upstream has acted on it.
:::

## Errors

When the upstream can't be reached, the client gets `502 Bad Gateway` with an empty body, and Go's standard logger prints the cause:

```text
http: proxy error: dial tcp 127.0.0.1:9000: connect: connection refused
```

Set `ErrorHandler` to answer differently:

```go
app.Use(proxy.New(proxy.Config{
	Target: "http://localhost:9000",
	ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		io.WriteString(w, `{"error":"upstream unavailable"}`+"\n")
	},
}))
```

```bash
curl -i http://localhost:8080/ping
# HTTP/1.1 503 Service Unavailable
# Content-Type: application/json
#
# {"error":"upstream unavailable"}
```

`ErrorHandler` writes to the response directly: Zinc's error handler isn't involved. Only an error from a `Balancer` goes through Zinc's error handler.

`proxy.New` panics at startup when:

- none of `Target`, `Targets` or `Balancer` is set
- a target isn't an absolute URL with a scheme and host
- `Retries` is negative
- two `RegexRewrite` rules have the same pattern
- it's given more than one `Config`

`proxy.NewRoundRobinBalancer` and `proxy.NewRandomBalancer` panic when given no targets.

## Security

Zinc removes the client's `Forwarded` and `X-Forwarded-*` headers, then sets `X-Forwarded-For`, `X-Forwarded-Host` and `X-Forwarded-Proto` from the connection it received. A client can't pass a forged address through to the upstream. If Zinc itself runs behind a proxy, see [Client IP and Proxies](/guide/ip-address/).

`Director` runs after hop-by-hop headers are removed. A client can't strip a header your `Director` adds by naming it in `Connection`.

`Director`, `ModifyResponse`, `ErrorHandler`, `Balancer` and `Transport` run with the same trust as the rest of your code. Don't build a target from request input without checking it against a list you control.

## Related

- [Reverse proxy recipe](/cookbook/reverse-proxy/): a complete program to run.
- [Rewrite](/middleware/rewrite/): changes the path within your own app.
- [Client IP and Proxies](/guide/ip-address/): reading the client address when Zinc is behind a proxy.
