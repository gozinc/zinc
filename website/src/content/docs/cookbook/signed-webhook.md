---
title: Verify a Signed Webhook
description: Accept webhook deliveries only when their HMAC signature matches, and get 401 for anything forged or changed.
---

This program receives webhooks from a provider such as GitHub or a billing service, and accepts a delivery only if its signature matches. Use it whenever a third party calls your API and you need to know the request really came from them. It reads the raw body once with `c.BodyBytes`, checks the signature, then decodes the same bytes.

## Run it

```bash
mkdir zinc-webhook && cd zinc-webhook
go mod init example.com/zinc-webhook
go get github.com/0mjs/zinc
```

Save the program as `main.go` and run `go run .`. It listens on port 8080. Without `WEBHOOK_SECRET` set, it uses the secret `local-test-secret` so you can try it locally.

## The program

```go title="main.go"
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"strings"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/bodylimit"
)

type event struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// verify reports whether header holds the HMAC-SHA256 of body under secret.
func verify(body []byte, header, secret string) bool {
	signature := strings.TrimPrefix(header, "sha256=")
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func main() {
	secret := os.Getenv("WEBHOOK_SECRET")
	if secret == "" {
		secret = "local-test-secret"
		log.Print("WEBHOOK_SECRET not set; using the local test secret")
	}

	app := zinc.New()
	app.Post("/webhooks/billing",
		bodylimit.New(bodylimit.Config{Limit: bodylimit.MB}),
		func(c *zinc.Context) error {
			body, err := c.BodyBytes()
			if err != nil {
				return err // 413 when the body is over the limit
			}

			if !verify(body, c.Header("X-Hub-Signature-256"), secret) {
				return zinc.Unauthorized("invalid webhook signature")
			}

			var incoming event
			if err := json.Unmarshal(body, &incoming); err != nil {
				return zinc.BadRequest("invalid webhook payload")
			}

			log.Printf("accepted webhook %s (%s)", incoming.ID, incoming.Type)
			return c.Status(zinc.StatusAccepted).JSON(zinc.Map{"accepted": true})
		},
	)

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

Sign a body with the test secret, the way a provider would, and send it:

```bash
body='{"id":"evt_123","type":"invoice.paid"}'
signature=$(printf %s "$body" | openssl dgst -sha256 -hmac 'local-test-secret' -hex | sed 's/^.* //')

curl -i http://localhost:8080/webhooks/billing \
  -H 'Content-Type: application/json' \
  -H "X-Hub-Signature-256: sha256=$signature" \
  --data "$body"
# HTTP/1.1 202 Accepted
# Content-Type: application/json; charset=utf-8
#
# {"accepted":true}
```

The server logs the delivery:

```text
2026/09/28 01:17:01 accepted webhook evt_123 (invoice.paid)
```

Change one byte of the body and keep the old signature, and the delivery is rejected:

```bash
curl http://localhost:8080/webhooks/billing \
  -H 'Content-Type: application/json' \
  -H "X-Hub-Signature-256: sha256=$signature" \
  --data '{"id":"evt_123","type":"invoice.void"}'
# {"error":{"status":401,"message":"invalid webhook signature"}}
```

A body over 1 MB never reaches the check:

```bash
head -c 2000000 /dev/zero | curl -i http://localhost:8080/webhooks/billing --data-binary @-
# HTTP/1.1 413 Request Entity Too Large
#
# {"error":{"status":413,"message":"Request Entity Too Large"}}
```

## How it works

- `bodylimit.New(bodylimit.Config{Limit: bodylimit.MB})` runs before the handler and caps the body at 1 MB. A larger body answers `413`, whether or not the client sent `Content-Length`.
- `c.BodyBytes()` reads the exact bytes the provider signed. The signature covers those bytes, so verify them before decoding; re-encoding the JSON could change them.
- `verify` computes the HMAC-SHA256 of the body and compares it with `hmac.Equal`, which takes the same time whether the first or last byte differs.
- `json.Unmarshal(body, &incoming)` decodes the same bytes only after the signature passes.
- `202 Accepted` tells the provider the delivery arrived, so it doesn't retry.

## Before production

- Set `WEBHOOK_SECRET` from your deployment environment, never commit it, and remove the local fallback so a missing secret stops the server.
- Keep the body limit close to the provider's documented maximum.
- Store processed event IDs, so a delivery the provider retries is handled once.
- Answer quickly and hand slow work to a background worker or queue. Providers often time out after a few seconds and retry.

## See also

- [Body Limit](/middleware/bodylimit/): the middleware that caps the body size.
- [Request Data](/guide/request/): `c.BodyBytes`, `c.Header` and other ways to read a request.
- [JWT](/cookbook/jwt/): protect routes that your own clients call.
