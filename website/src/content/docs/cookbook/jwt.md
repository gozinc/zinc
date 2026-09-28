---
title: JWT
description: Protect a group of routes with signed bearer tokens, read the token's claims in a handler, and get 401 without a valid token.
---

This program protects every route under `/api` with a JSON Web Token (JWT). A request with a valid `Authorization: Bearer <token>` header reaches the handler, which reads the user from the token; anything else gets `401`. Use it for an API called by your own apps or services, where a login step hands out tokens.

## Run it

The JWT middleware depends on [golang-jwt](https://github.com/golang-jwt/jwt), so it lives in Zinc's contrib repository:

```bash
mkdir zinc-jwt && cd zinc-jwt
go mod init example.com/zinc-jwt
go get github.com/0mjs/zinc github.com/0mjs/contrib/jwtauth github.com/golang-jwt/jwt/v5
```

Save the program as `main.go` and run `go run .`. It listens on port 8080. Without `JWT_SECRET` set, it signs and checks tokens with a local test secret.

## The program

```go title="main.go"
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/0mjs/contrib/jwtauth"
	"github.com/0mjs/zinc"
	"github.com/golang-jwt/jwt/v5"
)

func main() {
	secret := []byte(os.Getenv("JWT_SECRET"))
	if len(secret) == 0 {
		secret = []byte("local-test-secret-at-least-32-bytes")
		log.Print("JWT_SECRET not set; using the local test secret")
	}
	if len(secret) < 32 {
		log.Fatal("JWT_SECRET must be at least 32 bytes")
	}

	// `go run . token ada` prints a token for user "ada" and exits.
	if len(os.Args) == 3 && os.Args[1] == "token" {
		fmt.Println(mint(secret, os.Args[2]))
		return
	}

	app := zinc.New()

	api := app.Group("/api", jwtauth.New(jwtauth.Config{
		KeyFunc: func(_ *zinc.Context, token *jwt.Token) (any, error) {
			// Accept only the algorithm you sign with.
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method %v", token.Header["alg"])
			}
			return secret, nil
		},
	}))

	api.Get("/me", func(c *zinc.Context) error {
		claims := jwtauth.MustClaims[jwt.MapClaims](c)
		user, _ := claims.GetSubject()
		return c.JSON(zinc.Map{"user": user})
	})

	log.Fatal(app.Listen(":8080"))
}

// mint signs a token for user that expires in an hour.
func mint(secret []byte, user string) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": user,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString(secret)
	if err != nil {
		log.Fatal(err)
	}
	return signed
}
```

## Try it

Mint a token for the user `ada` with the same secret, then call the protected route with it:

```bash
TOKEN=$(go run . token ada)

curl -i localhost:8080/api/me -H "Authorization: Bearer $TOKEN"
# HTTP/1.1 200 OK
# Content-Type: application/json; charset=utf-8
#
# {"user":"ada"}
```

Without a token, the answer is `401` with a `WWW-Authenticate` header:

```bash
curl -i localhost:8080/api/me
# HTTP/1.1 401 Unauthorized
# Content-Type: application/json; charset=utf-8
# Www-Authenticate: Bearer
#
# {"error":{"status":401,"message":"Unauthorized"}}
```

A token signed with another secret, changed by hand, or past its `exp` time is rejected too, and the header says why:

```bash
curl -i localhost:8080/api/me -H "Authorization: Bearer ${TOKEN}x"
# HTTP/1.1 401 Unauthorized
# Www-Authenticate: Bearer error="invalid_token"
#
# {"error":{"status":401,"message":"Unauthorized"}}
```

## How it works

- `jwtauth.New` on the `/api` group checks the token before any handler in the group runs. By default it reads it from `Authorization: Bearer <token>`.
- `KeyFunc` returns the key that checks the signature. The type check on `token.Method` refuses tokens signed with any other algorithm, such as `none`.
- `jwtauth.MustClaims[jwt.MapClaims](c)` returns the claims of the token that passed. `MustClaims` panics if the middleware didn't run; use `jwtauth.Claims`, which also returns an `ok` flag, in code that may run without it.
- `mint` signs a token with a `sub` (the user) and an `exp` one hour ahead. golang-jwt rejects the token once `exp` has passed.

## Before production

- Set `JWT_SECRET` to at least 32 random bytes from your deployment environment, and remove the local fallback.
- Replace the `token` command with a login route that checks the user's password before calling `mint`.
- Keep tokens short-lived. A JWT stays valid until `exp`, even after the user logs out, unless you also keep a server-side list of revoked tokens.
- To read a token from a cookie, a custom header or the query string, set `Extractor` to one of the package's extractors.

## See also

- [JWT middleware](/middleware/jwtauth/): extractors, custom claims and every config field.
- [Groups and Middleware](/guide/groups-and-middleware/): run middleware on a set of routes.
- [Session Login](/cookbook/session-login/): log users in with a signed cookie instead of a bearer token.
