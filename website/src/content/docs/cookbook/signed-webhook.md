---
title: Verify a Signed Webhook
description: Limit, authenticate, decode, and acknowledge a webhook without losing the raw request body.
---

Webhook signatures are normally calculated from the exact bytes sent by the
provider. Read those bytes once, verify them with `crypto/hmac`, then decode the
same payload.

## Setup

```bash
mkdir zinc-webhook
cd zinc-webhook
go mod init example.com/zinc-webhook
go get github.com/0mjs/zinc
```

## Application

```go
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"strings"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/bodylimit"
)

type event struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

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
	const secret = "replace-me-from-your-environment"

	app := zinc.New()
	app.Post("/webhooks/billing",
		bodylimit.New(bodylimit.Config{Limit: bodylimit.MB}),
		func(c *zinc.Context) error {
			body, err := c.BodyBytes()
			if err != nil {
				return zinc.BadRequest("could not read webhook body")
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

## Generate a local signature

```bash
body='{"id":"evt_123","type":"invoice.paid"}'
signature=$(printf %s "$body" | openssl dgst -sha256 -hmac 'replace-me-from-your-environment' -hex | sed 's/^.* //')

curl -i http://localhost:8080/webhooks/billing \
  -H 'Content-Type: application/json' \
  -H "X-Hub-Signature-256: sha256=$signature" \
  --data "$body"
```

## Production notes

- Read the secret from your deployment environment; never commit it.
- Keep the body limit close to the provider's documented maximum.
- Store processed event IDs so provider retries are idempotent.
- Return quickly and move slow work to a queue or Zinc job.
