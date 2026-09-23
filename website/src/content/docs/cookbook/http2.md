---
title: HTTP/2 Server
description: Serve Zinc over TLS with Go's built-in HTTP/2 support.
---

Go enables HTTP/2 automatically for normal TLS servers when the runtime and client support it.

```go
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

Test with an HTTP/2-capable client:

```sh
curl --http2 --insecure https://localhost:8443/
```

Production certificates should be issued for the service hostname and managed outside the repository.
