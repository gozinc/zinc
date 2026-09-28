---
title: Server-Sent Events
description: Push a stream of events from the server to the browser over one long-lived HTTP response.
---

This program sends the time to the browser once a second until the tab closes. You'd use server-sent events (SSE) for live feeds, progress bars and notifications: anything where data flows from server to browser only. They're plain HTTP, so they need no extra library. For two-way messages, use a [WebSocket](/cookbook/websocket/).

## The program

```go title="main.go"
package main

import (
	"log"
	"strconv"
	"time"

	"github.com/0mjs/zinc"
)

const page = `<!doctype html>
<title>Clock</title>
<pre id="log"></pre>
<script>
  const events = new EventSource("/events");
  events.addEventListener("clock", (event) => {
    const { time } = JSON.parse(event.data);
    document.querySelector("#log").textContent += time + "\n";
  });
</script>
`

func main() {
	app := zinc.New()

	app.Get("/", func(c *zinc.Context) error {
		return c.HTML(page)
	})

	app.Get("/events", func(c *zinc.Context) error {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case now := <-ticker.C:
				if err := c.SSE(zinc.Event{
					Event: "clock",
					ID:    strconv.FormatInt(now.Unix(), 10),
					Data:  zinc.Map{"time": now.UTC().Format(time.RFC3339)},
				}); err != nil {
					return err
				}
			case <-c.Context().Done():
				log.Println("client went away")
				return nil
			}
		}
	})

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

Open `http://localhost:8080` in a browser and watch a line appear every second.

From a terminal, `curl -N` turns off curl's buffering so you see each event as it arrives. Press Ctrl-C to stop:

```bash
curl -N -i http://localhost:8080/events
# HTTP/1.1 200 OK
# Content-Type: text/event-stream
# Transfer-Encoding: chunked
#
# event: clock
# id: 1790554756
# data: {"time":"2026-09-28T00:19:16Z"}
#
# event: clock
# id: 1790554757
# data: {"time":"2026-09-28T00:19:17Z"}
```

When curl disconnects, the server logs:

```text
2026/09/28 01:19:18 client went away
```

## How it works

- `c.SSE(zinc.Event{...})` sets `Content-Type: text/event-stream` on the first call, writes one event and flushes it, so it reaches the client straight away.
- `Event` names the event. The browser's `addEventListener("clock", ...)` receives only events with that name. Leave it empty to use the `message` event instead.
- `Data` is encoded as JSON unless it's a `string` or `[]byte`, which are sent as they are.
- `c.Context().Done()` closes when the client disconnects. Returning then ends the handler and frees the goroutine.
- `new EventSource("/events")` in the page opens the stream and reconnects on its own if the connection drops.

## Before production

- Send `Cache-Control: no-cache` with `c.SetHeader` before the first event. Behind nginx, also send `X-Accel-Buffering: no`, or it may hold events back.
- Each open stream holds a connection and a goroutine. Count them if clients can open many.
- Put the route behind your auth middleware. `EventSource` sends cookies for same-origin URLs.

## Good to know

### Reconnects and missed events

When `EventSource` reconnects, it sends the last `ID` it saw in the `Last-Event-ID` header. Read it with `c.Header("Last-Event-ID")` to resend what the client missed. Set `Retry` on an event to change how long the browser waits before reconnecting.

### Timeouts

`Config.WriteTimeout` applies to each event rather than to the whole stream. A stream can stay open for as long as the handler keeps sending.

## See also

- [Responses and Rendering](/guide/responses-and-rendering/): `c.SSE`, `c.Stream` and the other response helpers.
- [Streaming Response](/cookbook/streaming-response/): send plain text or any other format in chunks.
- [WebSocket](/cookbook/websocket/): messages in both directions.
