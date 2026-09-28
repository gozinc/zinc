---
title: Body Dump
description: Hand a copy of each request and response body to your own function, for debugging, auditing or tests.
---

Body Dump gives your function a copy of each request body and response body, after the handler has run. Use it to debug what a client really sent, keep an audit trail, or check payloads in integration tests. Handlers still read the full body, and clients still get the full response.

## Usage

```go
import "github.com/0mjs/zinc/middleware/bodydump"

app.Use(bodydump.New(bodydump.Config{
	Observe: func(c *zinc.Context, s bodydump.Snapshot) {
		slog.Info("body_dump",
			"route", s.RoutePath,
			"status", s.Status,
			"request", string(s.RequestBody),
			"response", string(s.ResponseBody),
		)
	},
}))
```

```bash
curl -X POST -H 'Content-Type: application/json' -d '{"sku":"A-7","qty":2}' http://localhost:8080/orders
# {"id":1,"qty":2,"sku":"A-7"}
```

```text
level=INFO msg=body_dump route=/orders status=201 request="{\"sku\":\"A-7\",\"qty\":2}" response="{\"id\":1,\"qty\":2,\"sku\":\"A-7\"}\n"
```

:::caution[Bodies often hold secrets]
Passwords, tokens and personal data pass through request and response bodies. Set `Redact` before you send snapshots anywhere they're stored, and keep the capture limits small on busy routes.
:::

## Defaults

| Setting | Default |
|---|---|
| Observe | none: **required** |
| Redact | none |
| Request bytes captured | 1 MiB |
| Response bytes captured | 1 MiB |

## Configuration

This hides login bodies and keeps at most 64 KiB of each body:

```go
app.Use(bodydump.New(bodydump.Config{
	Observe: sendToAuditLog, // your audit sink
	Redact: func(c *zinc.Context, s *bodydump.Snapshot) {
		if c.Path() == "/login" {
			s.RequestBody = []byte("[redacted]")
		}
	},
	MaxRequestBytes:  64 << 10,
	MaxResponseBytes: 64 << 10,
}))
```

| Field | Default | Meaning |
|---|---|---|
| `Observe` | **required** | `func(*zinc.Context, bodydump.Snapshot)`. Receives each request's snapshot. Nil panics when the middleware is created. |
| `Redact` | none | `func(*zinc.Context, *bodydump.Snapshot)`. Changes the snapshot before `Observe` sees it. |
| `MaxRequestBytes` | 1 MiB | Most request bytes copied into the snapshot. `0` keeps the default; a negative value copies everything. |
| `MaxResponseBytes` | 1 MiB | Most response bytes copied into the snapshot. `0` keeps the default; a negative value copies everything. |

The defaults are exported as `bodydump.DefaultMaxRequestBytes` and `bodydump.DefaultMaxResponseBytes`.

## What a snapshot holds

| Field | Meaning |
|---|---|
| `Method`, `Path`, `RoutePath` | The request method, path, and matched route pattern |
| `Status` | Status sent to the client |
| `RequestBody`, `ResponseBody` | The copied bytes, up to the limits |
| `RequestBytes`, `ResponseBytes` | Full sizes, even when the copy was cut short |
| `RequestTruncated`, `ResponseTruncated` | `true` when the copy stopped at the limit |
| `Error` | The error the chain returned, if any |

With both limits set to 16 bytes, and every field logged, a 26-byte echo looks like this:

```text
level=INFO msg=body_dump path=/echo request=0123456789abcdef request_bytes=26 request_truncated=true response=0123456789abcdef response_bytes=26 response_truncated=true err=<nil>
```

The limits only cut the copy. The handler read all 26 bytes and the client received all 26.

## Errors

The middleware reads the whole request body before your handler runs, using `c.BodyBytes`. If that fails, for example because the body is over [`Config.BodyLimit`](/guide/configuration/), `Observe` still receives a snapshot with `Error` set and no bodies, and the request fails with that error:

```bash
curl -i -X POST --data-binary @big.json http://localhost:8080/echo   # over Config.BodyLimit
# HTTP/1.1 413 Request Entity Too Large
```

Errors from the rest of the chain are passed through unchanged, and also appear in `Snapshot.Error`.

:::note[Memory use]
Because the whole request body is read up front, each request holds it in memory, up to `Config.BodyLimit`, whatever `MaxRequestBytes` is set to. `MaxRequestBytes` only limits the copy the snapshot keeps.
:::

## Related

- [Request Logger](/middleware/logger/) logs one line per request without the bodies.
- [Body Limit](/middleware/bodylimit/) caps request body size.
- [Testing](/guide/testing/) covers checking responses in tests.
