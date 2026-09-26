---
title: Body Dump
description: Capture request and response bodies for logging, auditing, or test instrumentation.
---

`bodydump` captures request and response bodies and passes them to an observer, without changing the bytes handlers read or clients receive.

```go
import "github.com/0mjs/zinc/middleware/bodydump"

app.Use(bodydump.New(bodydump.Config{
	Observe: func(c *zinc.Context, s bodydump.Snapshot) {
		slog.Info("body_dump",
			"route", s.RoutePath,
			"status", s.Status,
			"request_bytes", s.RequestBytes,
			"response_bytes", s.ResponseBytes,
		)
	},
}))
```

## Config

| Field | Default | Meaning |
|---|---|---|
| `Observe` | required | Receives each snapshot |
| `Redact` | none | Changes the snapshot before `Observe` sees it |
| `MaxRequestBytes` | 1 MiB | Most request bytes captured |
| `MaxResponseBytes` | 1 MiB | Most response bytes captured |

A `bodydump.Snapshot` has `Method`, `Path`, `RoutePath`, `Status`, `RequestBody`, `ResponseBody`, `RequestBytes`, `ResponseBytes`, `RequestTruncated`, `ResponseTruncated`, and `Error`.

## Redaction

```go
app.Use(bodydump.New(bodydump.Config{
	Observe: sendToAuditLog,
	Redact: func(_ *zinc.Context, s *bodydump.Snapshot) {
		s.RequestBody = []byte("[redacted]")
	},
	MaxRequestBytes:  64 << 10,
	MaxResponseBytes: 64 << 10,
}))
```

Use this for debugging, auditing, and integration tests. Avoid it on very high-volume routes unless you are deliberate about truncation and redaction.
