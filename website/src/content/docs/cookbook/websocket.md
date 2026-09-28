---
title: WebSocket
description: Upgrade a Zinc route to a WebSocket and send messages both ways over one connection.
---

This program is a WebSocket echo server: every message a client sends comes straight back. You'd use WebSockets for chat, multiplayer editing or games, where both sides send messages at any time. Zinc passes the standard `http.ResponseWriter` to the upgrader, so any `net/http` WebSocket library works. This recipe uses [Gorilla WebSocket](https://github.com/gorilla/websocket).

## Run it

```bash
go mod init example.com/echo
go get github.com/0mjs/zinc github.com/gorilla/websocket
```

## The program

```go title="main.go"
package main

import (
	"log"
	"net/http"

	"github.com/0mjs/zinc"
	"github.com/gorilla/websocket"
)

// Browsers send an Origin header; only accept pages you trust. Clients such as
// websocat send none, so they're allowed for local testing.
var allowedOrigins = map[string]bool{
	"https://app.example.com": true,
	"http://localhost:8080":   true,
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return origin == "" || allowedOrigins[origin]
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

		conn.SetReadLimit(4096)

		for {
			kind, message, err := conn.ReadMessage()
			if err != nil {
				log.Println("closed:", err)
				return nil
			}
			if err := conn.WriteMessage(kind, message); err != nil {
				return nil
			}
		}
	})

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

With [websocat](https://github.com/vi/websocat) installed, every line you type comes back:

```bash
websocat ws://localhost:8080/ws
```

Or use a small Go client. Save it as `client/main.go` in the same module. It sends two short messages, then one over the 4096-byte limit:

```go title="client/main.go"
package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/gorilla/websocket"
)

func main() {
	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/ws", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	for _, msg := range []string{"hello", "zinc", strings.Repeat("x", 5000)} {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			log.Fatal(err)
		}
		_, reply, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		fmt.Println("echo:", string(reply))
	}
}
```

```bash
go run ./client
# echo: hello
# echo: zinc
# error: websocket: close 1009 (message too big)
```

The server logs why the connection ended:

```text
2026/09/28 01:21:23 closed: websocket: read limit exceeded
```

A plain HTTP request, or a browser page from an origin that isn't allowed, is refused before the upgrade:

```bash
curl -i http://localhost:8080/ws
# HTTP/1.1 400 Bad Request
#
# Bad Request

curl -i http://localhost:8080/ws \
  -H "Connection: Upgrade" -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
  -H "Origin: https://evil.example"
# HTTP/1.1 403 Forbidden
#
# Forbidden
```

## How it works

- `upgrader.Upgrade(c.Writer(), c.Request(), nil)` switches the connection from HTTP to WebSocket. When it fails, it has already sent the `400` or `403`, so Zinc doesn't write a second response.
- `CheckOrigin` decides which web pages may connect. Browsers always send `Origin`, so a page on another site can't open a socket to your server unless you list it.
- `conn.SetReadLimit(4096)` caps the size of one message. A bigger one closes the connection with code `1009`, and `ReadMessage` returns an error.
- The loop returns `nil` when a read or write fails. By then the connection isn't HTTP any more, so there's no response to put an error in. A closed tab is also a normal way for the loop to end.
- `defer conn.Close()` closes the socket when the handler returns.

## Before production

- Authenticate before calling `Upgrade`, with your usual middleware on the route. After the upgrade, you can't send a `401`.
- Keep `allowedOrigins` to the sites that serve your front end. Drop `http://localhost:8080` outside development.
- Set read and write deadlines (`conn.SetReadDeadline`, `conn.SetWriteDeadline`) and send pings, so dead connections don't stay open forever.
- Only one goroutine may write to a connection at a time. If several goroutines send, give each connection one writer goroutine fed by a channel.

## Good to know

### Shutdown doesn't wait for sockets

`app.Shutdown` waits for HTTP requests to finish, but an upgraded connection is no longer one. Go's server neither waits for nor closes it. To close sockets on shutdown, keep track of open connections and close them yourself, or register a function with `http.Server.RegisterOnShutdown` when you run your own server.

## See also

- [Zinc and net/http](/guide/http-interoperability/): using `net/http` libraries with a Zinc handler.
- [Server-Sent Events](/cookbook/sse/): a simpler option when only the server sends.
- [Graceful Shutdown](/cookbook/graceful-shutdown/): stop the server without cutting off requests.
