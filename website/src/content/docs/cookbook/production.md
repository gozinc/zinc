---
title: A Production Service
description: "One complete program with what a deployed API needs: limits, timeouts, logs with request IDs, validation, a protected spec, and graceful shutdown."
---

This is a pet API set up the way you'd deploy it: requests are limited and timed out, each log line carries a request ID, input is validated against its struct tags, the OpenAPI spec and reference page sit behind a password, and the process drains its requests on `SIGTERM`. Each piece is ordinary Zinc. This page shows how they fit together.

## The program

```go title="main.go"
package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/apidocs"
	"github.com/0mjs/zinc/middleware/basicauth"
	"github.com/0mjs/zinc/middleware/healthcheck"
	"github.com/0mjs/zinc/middleware/logger"
	"github.com/0mjs/zinc/middleware/recover"
	"github.com/0mjs/zinc/middleware/requestid"
	"github.com/0mjs/zinc/middleware/secure"
	"github.com/0mjs/zinc/middleware/timeout"
)

type Pet struct {
	ID   int    `json:"id" openapi:"readonly"`
	Name string `json:"name" validate:"required,max=40"`
	Kind string `json:"kind" validate:"omitempty,oneof=cat dog"`
}

type CreatePet struct {
	Name string `json:"name" validate:"required,max=40"`
	Kind string `json:"kind" validate:"omitempty,oneof=cat dog"`
}

type PetID struct {
	ID int `path:"id"`
}

type Created struct {
	Location string `header:"Location" json:"-"`
	Pet
}

// store is shared by every request, so a lock guards it.
type store struct {
	mu   sync.RWMutex
	next int
	pets map[int]Pet
}

func (s *store) add(in CreatePet) Pet {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	pet := Pet{ID: s.next, Name: in.Name, Kind: in.Kind}
	s.pets[pet.ID] = pet
	return pet
}

func (s *store) get(id int) (Pet, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pet, ok := s.pets[id]
	return pet, ok
}

func main() {
	pets := &store{pets: map[int]Pet{}}
	spec := zinc.OpenAPIConfig{Title: "Pet Store", Version: "1.0.0"}

	app := zinc.New(zinc.Config{
		BodyLimit:       1 << 20, // 1 MiB
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    10 * time.Second,
		ShutdownTimeout: 15 * time.Second,
		OpenAPIPath:     "-", // served below, behind auth
		DocsPath:        "-",
	})

	// Order matters: the ID first, so every log line has it; the logger
	// before recover, so it logs the 500 a panic becomes.
	app.Use(requestid.New())
	app.Use(logger.New(logger.Config{Logger: slog.New(slog.NewJSONHandler(os.Stdout, nil))}))
	app.Use(recover.New())
	app.Use(secure.New())
	app.Use(healthcheck.New())

	api := app.Group("/api", timeout.New(timeout.Config{Timeout: 5 * time.Second}))
	api.Post("/pets", zinc.Typed(func(c *zinc.Context, in CreatePet) (Created, error) {
		pet := pets.add(in)
		return Created{Location: "/api/pets/" + strconv.Itoa(pet.ID), Pet: pet}, nil
	})).Status(http.StatusCreated)
	api.Get("/pets/{id}", zinc.Typed(func(c *zinc.Context, in PetID) (Pet, error) {
		pet, ok := pets.get(in.ID)
		if !ok {
			return Pet{}, zinc.NotFound("pet not found")
		}
		return pet, nil
	})).Errors(http.StatusNotFound)

	// The spec and its reference page are for the team, not the public.
	password := os.Getenv("DOCS_PASSWORD")
	if password == "" {
		log.Fatal("set DOCS_PASSWORD")
	}
	team := basicauth.New(basicauth.Config{Validator: basicauth.Static("team", password)})
	app.OpenAPI("/internal/openapi.json", spec, team)
	app.Get("/internal/docs", team, apidocs.New(apidocs.Config{Spec: "/internal/openapi.json"})).Hidden()

	// Catch spec and route mistakes before taking traffic.
	if err := app.Validate(); err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.ListenContext(ctx, ":8080"); err != nil {
		log.Fatal(err)
	}
}
```

```bash
DOCS_PASSWORD=s3cret go run .
```

## Try it

```bash
curl -i -X POST localhost:8080/api/pets -H 'Content-Type: application/json' -d '{"name":"Tom","kind":"cat"}'
# HTTP/1.1 201 Created
# Content-Type: application/json; charset=utf-8
# Location: /api/pets/1
# X-Request-Id: 846459a825f367bf014af6257b9d6d1f
#
# {"id":1,"name":"Tom","kind":"cat"}

curl -X POST localhost:8080/api/pets -H 'Content-Type: application/json' -d '{"kind":"cow"}'
# {"error":{"status":422,"message":"validation failed","fields":{"kind":"must be one of: cat, dog","name":"is required"}}}

curl localhost:8080/api/pets/9
# {"error":{"status":404,"message":"pet not found"}}

curl -o /dev/null -w '%{http_code}\n' localhost:8080/internal/openapi.json
# 401
curl -o /dev/null -w '%{http_code}\n' -u team:s3cret localhost:8080/internal/openapi.json
# 200
```

Each request is one JSON log line, with the same request ID the client got:

```json
{"time":"2026-10-02T20:13:17.916737+01:00","level":"INFO","msg":"REQUEST","method":"POST","uri":"/api/pets","route":"/api/pets","status":201,"latency":422417,"host":"localhost:8080","bytes_in":"27","bytes_out":35,"user_agent":"curl/8.7.1","remote_ip":"::1","request_id":"846459a825f367bf014af6257b9d6d1f"}
```

The secure middleware also adds headers such as `X-Content-Type-Options` and `X-Frame-Options` to every response; they're left out above.

## What each piece does

| Piece | Why |
|---|---|
| `BodyLimit`, `ReadTimeout`, `WriteTimeout` | A slow or oversized client can't hold a connection or memory. A body over 1 MiB gets `413`. |
| `requestid` first | Every later middleware and log line can use the ID, and the client gets it back in `X-Request-Id` to quote in a bug report. |
| `logger` before `recover` | A panic becomes a `500` inside `recover`, and the logger, outside it, logs that `500`. The logger passes the request's context to `slog`, so a handler that reads trace IDs from it, such as an OpenTelemetry bridge, can add them. |
| `healthcheck` | `/healthz` answers `204` for a load balancer without reaching your routes. |
| `timeout` on `/api` | Handler work that honors `c.Context()` stops after 5 seconds. |
| Typed handlers | `validate` tags are checked by Zinc's built-in rules, and the spec claims exactly those rules. `Created` sends `Location` as a header from its tagged field. |
| `store` with a lock | Requests run concurrently, so shared state needs a `sync.RWMutex`, or a database. |
| `app.OpenAPI` with `team` | The spec moves to `/internal/openapi.json` behind Basic Auth. `OpenAPIPath: "-"` and `DocsPath: "-"` turn off the public ones. |
| `app.Validate()` | Builds the spec and checks the routes before the server takes traffic, so a mistake fails the deploy, not a request. |
| `ListenContext` with `signal.NotifyContext` | On `SIGTERM` the server stops accepting connections and waits up to `ShutdownTimeout` for running requests. |

## Next steps

- [Graceful Shutdown](/cookbook/graceful-shutdown/): the shutdown sequence on its own.
- [Structured Logs with slog](/cookbook/structured-logging/): more on the log fields.
- [Binding](/guide/binding/#validation): the validation rules Zinc enforces.
