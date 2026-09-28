---
title: Decompress
description: Accept gzip-compressed request bodies and read them as plain ones.
---

Decompress lets clients send request bodies gzipped with `Content-Encoding: gzip`, and hands your handlers the plain body. Add it when clients upload large JSON, logs or events and want to save bandwidth. Handlers and binding read the body as if it had never been compressed.

## Usage

```go
import "github.com/0mjs/zinc/middleware/decompress"

app.Use(decompress.New())

type Event struct {
	Name string `json:"name"`
}

app.Post("/ingest", func(c *zinc.Context) error {
	var input Event
	if err := c.Bind().JSON(&input); err != nil {
		return err
	}
	return c.JSON(zinc.Map{"received": input.Name})
})
```

```bash
echo '{"name":"signup"}' | gzip > event.json.gz

curl http://localhost:8080/ingest \
  -H "Content-Type: application/json" -H "Content-Encoding: gzip" \
  --data-binary @event.json.gz
# {"received":"signup"}

curl http://localhost:8080/ingest \
  -H "Content-Type: application/json" -d '{"name":"signup"}'
# {"received":"signup"}
```

Plain bodies, with no `Content-Encoding` or `identity`, pass through unchanged. For a decompressed body, the middleware removes the `Content-Encoding` and `Content-Length` request headers, since they described the compressed bytes.

## Defaults

| Setting | Default |
|---|---|
| Encodings accepted | `gzip` (plus plain bodies) |
| Largest decompressed body | the app's `Config.BodyLimit` (4 MiB unless you change it) |

## Configuration

Allow up to 16 MiB once decompressed:

```go
app := zinc.New(zinc.Config{BodyLimit: 16 << 20})
app.Use(decompress.New(decompress.Config{MaxDecompressedSize: 16 << 20}))
```

| Field | Default | Meaning |
|---|---|---|
| `MaxDecompressedSize` | `0`, which means the app's `Config.BodyLimit`, or 4 MiB when that's `-1` (off) | Largest body, in bytes, a request may expand to. Reading past it fails with `413`. |

:::caution[Raise BodyLimit too]
`c.Bind()`, `c.BodyBytes()` and form parsing still apply the app's `Config.BodyLimit` to the decompressed body. A `MaxDecompressedSize` above `BodyLimit` only helps handlers that read `c.Request().Body` themselves, so raise both together as above.
:::

## Errors

| Request | Status | `errors.Is` target |
|---|---|---|
| `Content-Encoding` other than `gzip` or `identity`, such as `br` | `415 Unsupported Media Type` | `decompress.ErrUnsupportedEncoding`, `zinc.ErrUnsupportedMediaType` |
| Body isn't valid gzip | `400 Bad Request` | `decompress.ErrInvalidBody`, `zinc.ErrBadRequest` |
| Body expands past the limit | `413 Request Entity Too Large` | `zinc.ErrRequestEntityTooLarge` |

```bash
curl -i http://localhost:8080/ingest \
  -H "Content-Type: application/json" -H "Content-Encoding: br" -d '{"name":"signup"}'
# HTTP/1.1 415 Unsupported Media Type
#
# {"error":{"status":415,"message":"Unsupported Media Type"}}
```

The limit exists because a small compressed body can expand enormously: a few kilobytes of gzip can hold gigabytes of zeros. The `413` comes back when the handler reads past the limit, not before.

`decompress.New` panics at startup when `MaxDecompressedSize` is negative, or when it's given more than one `Config`.

## Related

- [Body Limit](/middleware/bodylimit/): caps the size of request bodies on particular routes.
- [Compress](/middleware/compress/): gzips responses.
- [Content Type](/middleware/contenttype/): rejects bodies in media types or encodings you don't accept.
- [Configuration](/guide/configuration/): the app-wide `BodyLimit`.
