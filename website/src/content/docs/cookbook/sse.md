---
title: Server-Sent Events
description: Stream structured one-way events to the browser, each delivered as soon as it is sent.
---

Server-sent events push a stream of updates from server to browser over one long-lived HTTP response. They are simpler than WebSockets when data flows only one way. This program sends the time every second until the client disconnects.

```go
package main

import (
	"log"
	"strconv"
	"time"

	"github.com/0mjs/zinc"
)

func main() {
	app := zinc.New()

	app.Get("/events", func(c *zinc.Context) error {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case now := <-ticker.C:
				if err := c.SSE(zinc.SSEvent{
					Event: "clock",
					ID:    strconv.FormatInt(now.Unix(), 10),
					Data:  zinc.Map{"time": now.UTC()},
				}); err != nil {
					return err
				}
			case <-c.Context().Done():
				return nil
			}
		}
	})

	log.Fatal(app.Listen(":8080"))
}
```

Browser client:

```js
const events = new EventSource("/events");
events.addEventListener("clock", (event) => {
  console.log(JSON.parse(event.data));
});
```

`c.SSE` sets the event-stream headers, writes one event, and flushes it so it
reaches the client immediately. `Config.WriteTimeout` applies to each event
rather than to the whole stream, so the stream can stay open for as long as the
handler keeps sending. The handler exits when the client disconnects and the
request context is cancelled.
