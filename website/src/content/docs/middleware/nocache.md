---
title: No Cache
description: Stop browsers and proxies from storing or reusing responses.
---

No Cache tells browsers and proxies not to store a response or reuse it later. Add it to account pages, dashboards and anything that shows personal data, so the back button or a shared proxy never serves a stale or private copy.

## Usage

```go
import "github.com/0mjs/zinc/middleware/nocache"

account := app.Group("/account", nocache.New())
account.Get("/settings", showSettings) // showSettings: your handler
```

```bash
curl -i http://localhost:8080/account/settings
# HTTP/1.1 200 OK
# Cache-Control: no-cache, no-store, max-age=0, must-revalidate
# Content-Type: application/json; charset=utf-8
# Expires: 0
# Pragma: no-cache
#
# {"email":"ada@example.com"}
```

`Pragma` and `Expires` cover old HTTP/1.0 caches that ignore `Cache-Control`.

## Defaults

It always sets the three headers shown above.

| Header | Value |
|---|---|
| `Cache-Control` | `no-cache, no-store, max-age=0, must-revalidate` |
| `Pragma` | `no-cache` |
| `Expires` | `0` |

## Configuration

There's nothing to configure: `nocache.New()` takes no arguments. The headers are set before your handler runs, so a handler can still replace one with `c.SetHeader`.

## Errors

`nocache` never fails a request.

## Related

- [Headers](/middleware/headers/): sets other fixed response headers.
- [Secure Headers](/middleware/secure/): sets browser security headers.
- [Groups and Middleware](/guide/groups-and-middleware/): attaching middleware to a group.
