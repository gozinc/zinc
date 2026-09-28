---
title: Streaming Response
description: Send a response in pieces as you produce them, so the client sees each line without waiting for the end.
---

This program writes five lines, half a second apart, and the client sees each one as soon as it's written. You'd stream a response for long exports, build or deploy logs, or AI-generated text. It uses Go's `http.ResponseController` on `c.Writer()`, which works in Zinc because the writer is a standard `http.ResponseWriter`.

## The program

```go title="main.go"
package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/0mjs/zinc"
)

func main() {
	app := zinc.New()

	app.Get("/stream", func(c *zinc.Context) error {
		c.Type("txt")
		controller := http.NewResponseController(c.Writer())

		for i := 1; i <= 5; i++ {
			// Give each chunk its own write deadline, so the stream can
			// outlast Config.WriteTimeout (10 seconds by default).
			if err := controller.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(c.Writer(), "chunk %d at %s\n", i, time.Now().Format("15:04:05.000")); err != nil {
				return err
			}
			if err := controller.Flush(); err != nil {
				return err
			}

			select {
			case <-time.After(500 * time.Millisecond):
			case <-c.Context().Done():
				return nil
			}
		}

		return nil
	})

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

`curl -N` turns off curl's own buffering. The lines appear one at a time, and the timestamps show they were sent half a second apart:

```bash
curl -N -i http://localhost:8080/stream
# HTTP/1.1 200 OK
# Content-Type: text/plain; charset=utf-8
# Transfer-Encoding: chunked
#
# chunk 1 at 01:20:37.581
# chunk 2 at 01:20:38.082
# chunk 3 at 01:20:38.583
# chunk 4 at 01:20:39.084
# chunk 5 at 01:20:39.585
```

The response has no `Content-Length`, because the server doesn't know the size when it starts. It's sent with `Transfer-Encoding: chunked` instead.

Delete the `SetWriteDeadline` call and change the loop to 25 chunks, and the stream stops after chunk 20. That's the 10-second `WriteTimeout` closing the connection.

## How it works

- `c.Type("txt")` sets `Content-Type: text/plain` before the first byte goes out. Headers can't change after that.
- `http.NewResponseController(c.Writer())` gives you `Flush` and `SetWriteDeadline` on the underlying connection.
- `controller.Flush()` sends everything written so far. Without it, Go buffers small writes and the client may get them all at the end.
- `controller.SetWriteDeadline` moves the write deadline forward before each chunk. Zinc's `WriteTimeout` covers the whole response otherwise.
- `c.Context().Done()` closes when the client goes away, so the handler stops working for nobody.

## Good to know

### Stream from a reader

When you already have an `io.Reader`, such as a file or a pipe from another process, `c.Stream` copies it to the response:

```go
app.Get("/export", func(c *zinc.Context) error {
	f, err := os.Open("exports/big.csv")
	if err != nil {
		return err
	}
	defer f.Close()
	return c.Stream("text/csv", f)
})
```

`c.Stream` doesn't flush between reads, and the `WriteTimeout` still covers the whole copy. Use the program above when you need to choose when each piece goes out, or for streams that run longer than the timeout.

### Proxies can buffer too

A reverse proxy may collect the whole response before passing it on. Behind nginx, send `X-Accel-Buffering: no` with `c.SetHeader` before the first write.

### Events for browsers

For a browser that should react to each piece, [server-sent events](/cookbook/sse/) give you named events, IDs and automatic reconnects.

## See also

- [Responses and Rendering](/guide/responses-and-rendering/): `c.Stream`, `c.SSE` and the other response helpers.
- [Zinc and net/http](/guide/http-interoperability/): using standard `net/http` tools with a Zinc handler.
- [Server-Sent Events](/cookbook/sse/): push named events to a browser.
