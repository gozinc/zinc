---
title: JWT
description: Protect Zinc routes with signed bearer tokens and read typed claims.
---

Protect a group of routes with signed JSON Web Tokens, and read the token's claims in handlers. Requests without a valid `Authorization: Bearer <token>` header get `401`.

```go
package main

import (
	"fmt"
	"log"
	"os"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware"
)

func main() {
	app := zinc.New()
	secret := []byte(os.Getenv("JWT_SECRET"))
	if len(secret) < 32 {
		log.Fatal("JWT_SECRET must be at least 32 bytes")
	}

	api := app.Group("/api", middleware.JWT(
		func(_ *zinc.Context, token *jwt.Token) (any, error) {
			// Accept only the algorithm you sign with.
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method %v", token.Header["alg"])
			}
			return secret, nil
		},
	))

	api.Get("/me", func(c *zinc.Context) error {
		claims, ok := middleware.JWTClaims[jwt.MapClaims](c)
		if !ok {
			return zinc.NewError(zinc.StatusUnauthorized)
		}
		return c.JSON(claims)
	})

	log.Fatal(app.Listen())
}
```

The default extractor expects `Authorization: Bearer <token>`. Zinc also provides extractors for custom headers, cookies, query parameters, and fallback chains. See [JWT middleware](/middleware/jwt/).
