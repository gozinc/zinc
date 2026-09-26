---
title: FAQ
description: Answers to common questions about Zinc, including how it compares, stability, performance, and compatibility.
---

## What is Zinc?

A web framework for Go that sits on top of `net/http`. It adds fast routing, request binding, central error handling, response helpers, and 27 first-party middleware packages. It does not replace the standard HTTP server, request, or response writer.

## How does it compare with Gin, Echo, and Chi?

| | Zinc | Gin, Echo | Chi |
|---|---|---|---|
| Handler signature | `func(*zinc.Context) error` | Custom context | Standard `http.HandlerFunc` |
| Route syntax | Go 1.22 braces: `/users/{id}` | Colons: `/users/:id` | Braces |
| An `http.Handler`? | Yes | Yes | Yes |
| Standard handlers on routes | Yes, with `r.PathValue` | Through adapters | Yes |
| Binding, responses, middleware | Built in | Built in | Bring your own |

Zinc sits between the two styles: the ergonomics of Gin and Echo, with Chi's commitment to the standard library. If you like `net/http` but are tired of writing the same decoding, error, and response code in every handler, Zinc is aimed at you.

## Is Zinc production-ready?

Zinc is **pre-1.0**. Its behavior is covered by tests, and the security-relevant middleware is documented with its limits. The public API can still change between minor versions. Pin a version, and read the [release notes](https://github.com/0mjs/zinc/releases) before upgrading.

## Why does a handler return an error?

Returning an error lets one [error handler](/guide/errors/) decide how every failure looks to clients, instead of each handler writing its own error response. Handlers stay short, and error responses stay consistent.

## Can I use my existing net/http code?

Yes. Standard handlers can serve routes through `HandleHTTP` or own subtrees through `Mount`. Standard middleware wraps the app through `UseHTTP`. The app itself is an `http.Handler`. See [Zinc and net/http](/guide/http-interoperability/) and [Adopt Zinc in net/http](/cookbook/existing-net-http-service/).

## How fast is it?

In the 0.4.0 release run, Zinc had the lowest median latency in 58 of 77 scenarios against Gin, Echo, and Chi, all measured together on 26 September 2026. Common string-response routes use 16 B and one allocation per request. See [Benchmarks](/extra/benchmarks/) for the full measurements, losses, and reproduction steps.

## Does Zinc have dependencies?

No. Zinc's `go.mod` requires no module, and its middleware packages use only Zinc and the standard library. Middleware that wraps a third-party library, such as JWT, lives in [`github.com/0mjs/contrib`](/middleware/overview/#contrib), one module per package, so you download only what you import. Body formats beyond JSON and XML, such as YAML or TOML, plug in with the library you choose through [decoders and encoders](/guide/customization/#body-formats).

## Does Zinc validate input?

Zinc calls any validator you configure after every bind, but does not ship one. A three-line adapter plugs in [go-playground/validator](/guide/binding/#validation) or any other library.

## Does it support WebSockets, SSE, and HTTP/2?

Yes. WebSocket libraries work unchanged because handlers get the real response writer ([recipe](/cookbook/websocket/)). `c.SSE` writes server-sent events ([recipe](/cookbook/sse/)). HTTP/2 comes from Go's server over TLS ([recipe](/cookbook/http2/)).

## Why do invalid routes panic?

Route patterns are written in source code, so a typo is a programming error. Panicking at startup surfaces it immediately instead of at the first request. When patterns come from configuration, use `TryHandle`, which returns an error.

## Where do I report bugs or ask questions?

Open an issue on [GitHub](https://github.com/0mjs/zinc/issues). Read [CONTRIBUTING.md](https://github.com/0mjs/zinc/blob/dev/CONTRIBUTING.md) before opening a pull request.
