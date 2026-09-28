---
title: Compress
description: Gzip responses for clients that accept it.
---

Compress gzips responses for clients that send `Accept-Encoding: gzip`, which makes JSON and HTML much smaller on the wire. Skip it when a CDN or reverse proxy in front of the app already compresses responses.

## Usage

```go
import "github.com/0mjs/zinc/middleware/compress"

app.Use(compress.New())
```

A 2,012-byte JSON response, fetched with and without gzip:

```bash
curl -s -D - -o /dev/null -w "%{size_download} bytes\n" \
  -H "Accept-Encoding: gzip" http://localhost:8080/report
# HTTP/1.1 200 OK
# Content-Encoding: gzip
# Content-Type: application/json; charset=utf-8
# Vary: Accept-Encoding
#
# 53 bytes

curl -s -D - -o /dev/null -w "%{size_download} bytes\n" http://localhost:8080/report
# HTTP/1.1 200 OK
# Content-Type: application/json; charset=utf-8
# Vary: Accept-Encoding
#
# 2012 bytes
```

Error responses from your handlers, and 404s, are compressed too.

## Defaults

| Setting | Default |
|---|---|
| Level | `gzip.DefaultCompression` |
| Smallest body compressed | any size |

## Configuration

Trade a little size for speed, and leave small responses alone:

```go
import (
	"compress/gzip"

	"github.com/0mjs/zinc/middleware/compress"
)

app.Use(compress.New(compress.Config{Level: gzip.BestSpeed, MinLength: 1024}))
```

A 12-byte `{"ok":true}` now goes out uncompressed, and the 2,012-byte report is still gzipped. Setting `MinLength` is worth it: gzip adds about 20 bytes of framing, so a short error body comes out larger than it went in.

| Field | Default | Meaning |
|---|---|---|
| `Level` | `0`, which means `gzip.DefaultCompression` | A `compress/gzip` level, from `gzip.HuffmanOnly` (-2) to `gzip.BestCompression` (9) |
| `MinLength` | `0` | Responses shorter than this many bytes are sent uncompressed |

## When a response isn't compressed

The response goes out as written when:

- the client doesn't accept gzip, or sends `gzip;q=0`
- the request method is `HEAD`
- the status can't have a body (`1xx`, `204`, `304`)
- the handler already set `Content-Encoding`
- the body is shorter than `MinLength`

Every response gets `Vary: Accept-Encoding`, compressed or not, so a shared cache keeps the two versions apart.

## Streaming and WebSockets

The compressing writer keeps `Flush`, `Hijack` and `Push` working, so streaming responses and WebSocket upgrades still work behind it. With `MinLength` set, a `Flush` before the body reaches that size sends the response uncompressed.

## Errors

`compress` doesn't reject requests. An error from writing the gzip stream is returned from the middleware.

`compress.New` panics at startup when `Level` is outside -2 to 9, when `MinLength` is negative, or when it's given more than one `Config`.

## Related

- [Decompress](/middleware/decompress/): accepts gzip request bodies.
- [Streaming Response](/cookbook/streaming-response/): flushing a response as you write it.
