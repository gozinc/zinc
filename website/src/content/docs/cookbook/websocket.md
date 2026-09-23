---
title: WebSocket
description: Upgrade a Zinc route with a net/http-compatible WebSocket library.
---

Zinc preserves the response-writer interfaces required by WebSocket upgraders. This example uses Gorilla WebSocket:

```go
package main

import (
	"log"
	"net/http"

	"github.com/0mjs/zinc"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return r.Header.Get("Origin") == "https://app.example.com"
	},
}

func main() {
	app := zinc.New()

	app.Get("/ws", func(c *zinc.Context) error {
		conn, err := upgrader.Upgrade(c.Writer(), c.Request(), nil)
		if err != nil {
			return err
		}
		defer conn.Close()

		for {
			kind, message, err := conn.ReadMessage()
			if err != nil {
				return nil
			}
			if err := conn.WriteMessage(kind, message); err != nil {
				return nil
			}
		}
	})

	log.Fatal(app.Listen())
}
```

:::caution[Before production]
Validate origins, apply authentication before upgrading, and configure read/write limits for production connections.
:::
