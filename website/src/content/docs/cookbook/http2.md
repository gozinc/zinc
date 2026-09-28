---
title: HTTP/2 Server
description: Serve Zinc over HTTP/2, with TLS from a certificate file or unencrypted (h2c) behind a proxy.
---

This program serves HTTP/2 over TLS and reports which protocol each request used. You'd want HTTP/2 when clients make many requests over one connection, such as a browser loading an API-heavy page. Go's server turns it on for any TLS listener, so `app.ListenTLS` is all Zinc needs.

## Run it

`app.ListenTLS` needs a certificate and key. Make a self-signed pair for `localhost` with the generator that ships with Go:

```bash
go run $(go env GOROOT)/src/crypto/tls/generate_cert.go --host localhost
# 2026/09/28 01:18:20 wrote cert.pem
# 2026/09/28 01:18:20 wrote key.pem
```

[mkcert](https://github.com/FiloSottile/mkcert) works too, and makes a certificate your browser trusts.

## The program

```go title="main.go"
package main

import (
	"log"

	"github.com/0mjs/zinc"
)

func main() {
	app := zinc.New()
	app.Get("/", func(c *zinc.Context) error {
		return c.JSON(zinc.Map{"protocol": c.Request().Proto})
	})

	log.Fatal(app.ListenTLS(":8443", "cert.pem", "key.pem"))
}
```

## Try it

`--insecure` lets `curl` accept the self-signed certificate. `curl` asks for HTTP/2 over TLS by default:

```bash
curl -i --insecure https://localhost:8443/
# HTTP/2 200
# content-type: application/json; charset=utf-8
# content-length: 24
#
# {"protocol":"HTTP/2.0"}
```

Clients that only speak HTTP/1.1 still work on the same port:

```bash
curl -i --http1.1 --insecure https://localhost:8443/
# HTTP/1.1 200 OK
# Content-Type: application/json; charset=utf-8
#
# {"protocol":"HTTP/1.1"}
```

Plain HTTP to the TLS port is refused:

```bash
curl -i http://localhost:8443/
# HTTP/1.0 400 Bad Request
#
# Client sent an HTTP request to an HTTPS server.
```

## How it works

- `app.ListenTLS(":8443", "cert.pem", "key.pem")` starts a TLS server with Zinc's read, write and idle timeouts.
- During the TLS handshake, client and server agree on HTTP/2 or HTTP/1.1. Go's server offers both, so you configure nothing.
- `c.Request().Proto` is the protocol the request arrived on: `HTTP/2.0` or `HTTP/1.1`.

## Serve HTTP/2 without TLS (h2c)

Behind a proxy or load balancer that ends TLS and talks HTTP/2 to your service, serve unencrypted HTTP/2, known as h2c. Zinc's `Listen` methods don't turn it on, so run Zinc on your own `http.Server`:

```go title="main.go"
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/0mjs/zinc"
)

func main() {
	app := zinc.New()
	app.Get("/", func(c *zinc.Context) error {
		return c.JSON(zinc.Map{"protocol": c.Request().Proto})
	})

	var protocols http.Protocols
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      app,
		Protocols:    &protocols,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
```

```bash
curl -i --http2-prior-knowledge http://localhost:8080/
# HTTP/2 200
# content-type: application/json; charset=utf-8
#
# {"protocol":"HTTP/2.0"}

curl -i http://localhost:8080/
# HTTP/1.1 200 OK
# Content-Type: application/json; charset=utf-8
#
# {"protocol":"HTTP/1.1"}
```

An `http.Server` you build yourself doesn't get Zinc's `ReadTimeout`, `WriteTimeout` and `IdleTimeout`, so the program sets the same values Zinc uses by default. Only expose h2c on a private network: it has no encryption.

## Before production

- Use a certificate issued for your service's hostname, and keep it and its key out of the repository.
- If the server faces the internet, [Automatic TLS](/cookbook/auto-tls/) can fetch and renew certificates for you.

## See also

- [Configuration](/guide/configuration/): the server timeouts `ListenTLS` applies.
- [Automatic TLS](/cookbook/auto-tls/): Let's Encrypt certificates with `autocert`.
- [Zinc and net/http](/guide/http-interoperability/): running Zinc on a server you configure.
