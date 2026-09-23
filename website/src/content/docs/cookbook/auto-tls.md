---
title: Automatic TLS
description: Serve a Zinc application with automatically managed Let's Encrypt certificates.
---

Zinc exposes a standard `http.Handler`, so Go's `autocert` package can own certificate issuance and renewal.

```go
package main

import (
	"log"
	"net/http"

	"github.com/0mjs/zinc"
	"golang.org/x/crypto/acme/autocert"
)

func main() {
	app := zinc.New()
	app.Get("/", func(c *zinc.Context) error {
		return c.String("secure")
	})

	manager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist("api.example.com"),
		Cache:      autocert.DirCache("./certs"),
	}

	go func() {
		log.Fatal(http.ListenAndServe(":80", manager.HTTPHandler(nil)))
	}()

	server := &http.Server{
		Addr:      ":443",
		Handler:   app,
		TLSConfig: manager.TLSConfig(),
	}
	log.Fatal(server.ListenAndServeTLS("", ""))
}
```

The hostname must resolve to the server and ports 80 and 443 must be reachable. For existing certificate files, use `app.ListenTLS` instead.
