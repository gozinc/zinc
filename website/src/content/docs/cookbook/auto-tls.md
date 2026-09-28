---
title: Automatic TLS
description: Serve Zinc over HTTPS with Let's Encrypt certificates that are fetched and renewed for you.
---

This program serves your app on HTTPS with a certificate from Let's Encrypt, and renews it before it expires. You'd use it when the Go process faces the internet directly, with no load balancer or proxy handling TLS in front of it. Zinc is an `http.Handler`, so Go's `autocert` package can run the certificates while Zinc handles the requests.

:::caution[You can't fully run this locally]
Let's Encrypt only issues a certificate for a public domain that resolves to the machine running the program, with ports 80 and 443 reachable from the internet. On a laptop you can check the redirect and the host allow-list, shown under [Try it](#try-it), but not the certificate itself.
:::

## Run it

```bash
go mod init example.com/autotls
go get github.com/0mjs/zinc golang.org/x/crypto/acme/autocert
```

Replace `api.example.com` in the program with your domain, and point its DNS at the server. Binding ports 80 and 443 on Linux needs root or the `CAP_NET_BIND_SERVICE` capability.

## The program

```go title="main.go"
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/0mjs/zinc"
	"golang.org/x/crypto/acme/autocert"
)

func main() {
	app := zinc.New()
	app.Get("/", func(c *zinc.Context) error {
		return c.String("secure\n")
	})

	manager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist("api.example.com"),
		Cache:      autocert.DirCache("./certs"),
	}

	// Port 80 answers Let's Encrypt's HTTP-01 challenges and
	// redirects everything else to HTTPS.
	go func() {
		log.Fatal(http.ListenAndServe(":80", manager.HTTPHandler(nil)))
	}()

	server := &http.Server{
		Addr:         ":443",
		Handler:      app,
		TLSConfig:    manager.TLSConfig(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	log.Fatal(server.ListenAndServeTLS("", ""))
}
```

## Try it

On the server, with your domain in place of `api.example.com`:

```bash
curl https://api.example.com/
# secure
```

The first request takes a few seconds while the certificate is issued; later ones use the copy saved in `./certs`.

What you can check on your own machine: port 80 redirects to HTTPS, keeping the path.

```bash
curl -i -H 'Host: api.example.com' http://localhost/users
# HTTP/1.1 302 Found
# Location: https://api.example.com/users
```

A hostname that isn't in `HostWhitelist` fails the TLS handshake, and the server logs why:

```bash
curl -i --resolve other.example.com:443:127.0.0.1 https://other.example.com/
# curl: (35) LibreSSL/3.3.6: error:1404B438:SSL routines:ST_CONNECT:tlsv1 alert internal error
```

```text
http: TLS handshake error from 127.0.0.1:65249: acme/autocert: host "other.example.com" not configured in HostWhitelist
```

## How it works

- `autocert.Manager` asks Let's Encrypt for a certificate the first time a client connects for a hostname, and renews it before it expires.
- `HostWhitelist("api.example.com")` limits which names it will request. Without it, anyone pointing a domain at your server could make it request certificates.
- `manager.HTTPHandler(nil)` on port 80 answers the challenge Let's Encrypt uses to confirm you control the domain, and redirects other requests to HTTPS.
- `manager.TLSConfig()` hands each TLS handshake the right certificate. `ListenAndServeTLS("", "")` takes empty file names because the certificates come from that config.
- `DirCache("./certs")` saves certificates and the account key, so a restart doesn't request new ones.

## Before production

- Set the server timeouts yourself, as the program does. An `http.Server` you build doesn't get Zinc's `ReadTimeout`, `WriteTimeout` and `IdleTimeout`; these are Zinc's defaults.
- Keep `./certs` on persistent storage. Let's Encrypt rate-limits issuance, and a container that loses its cache on every deploy can hit the limit.
- While testing, point `manager.Client` at the Let's Encrypt staging directory (`&acme.Client{DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory"}`, from `golang.org/x/crypto/acme`). Its limits are higher, and its certificates aren't trusted by browsers.
- For graceful shutdown, call `server.Shutdown` and then `app.Close()`, as in [Graceful Shutdown](/cookbook/graceful-shutdown/#running-your-own-httpserver).

## Good to know

### Certificate files you already have

If you already have a certificate and key, you don't need `autocert`. Use `app.ListenTLS(":443", "cert.pem", "key.pem")`, which applies Zinc's timeouts for you. See [HTTP/2 Server](/cookbook/http2/).

## See also

- [HTTP/2 Server](/cookbook/http2/): TLS from certificate files, and h2c.
- [Zinc and net/http](/guide/http-interoperability/): serving Zinc from any `http.Server`.
- [Configuration](/guide/configuration/): Zinc's default server timeouts.
