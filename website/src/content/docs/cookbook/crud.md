---
title: CRUD API
description: Build an in-memory JSON API that creates, lists, reads, updates and deletes widgets, with 404 and 422 answers you can see.
---

This program is a JSON API for one resource, widgets, kept in memory. You can create, list, read, update and delete them, and bad input gets a clear error. Use it as the starting point for any resource API; it shows route groups, `c.Bind().JSON`, `zinc.Param[int]` and Zinc's error helpers.

## Run it

```bash
mkdir zinc-widgets && cd zinc-widgets
go mod init example.com/zinc-widgets
go get github.com/0mjs/zinc
```

Save the program as `main.go`, then run `go run .`. It listens on port 8080.

## The program

```go title="main.go"
package main

import (
	"log"
	"sync"

	"github.com/0mjs/zinc"
)

type Widget struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type WidgetInput struct {
	Name string `json:"name"`
}

type widgetStore struct {
	mu     sync.RWMutex
	nextID int
	items  map[int]Widget
}

func newWidgetStore() *widgetStore {
	return &widgetStore{nextID: 1, items: make(map[int]Widget)}
}

func main() {
	store := newWidgetStore()
	app := zinc.New()
	widgets := app.Group("/widgets")

	widgets.Get("/", func(c *zinc.Context) error {
		store.mu.RLock()
		defer store.mu.RUnlock()

		items := make([]Widget, 0, len(store.items))
		for _, widget := range store.items {
			items = append(items, widget)
		}
		return c.JSON(items)
	})

	widgets.Post("/", func(c *zinc.Context) error {
		var input WidgetInput
		if err := c.Bind().JSON(&input); err != nil {
			return err // 400 with the failing field
		}
		if input.Name == "" {
			return zinc.UnprocessableEntity("name is required")
		}

		store.mu.Lock()
		widget := Widget{ID: store.nextID, Name: input.Name}
		store.items[widget.ID] = widget
		store.nextID++
		store.mu.Unlock()

		return c.Status(zinc.StatusCreated).JSON(widget)
	})

	widgets.Get("/{id}", func(c *zinc.Context) error {
		id, err := zinc.Param[int](c, "id")
		if err != nil {
			return err // 400: {"fields":{"id":"must be an integer"}}
		}

		store.mu.RLock()
		widget, ok := store.items[id]
		store.mu.RUnlock()
		if !ok {
			return zinc.NotFound("widget not found")
		}
		return c.JSON(widget)
	})

	widgets.Put("/{id}", func(c *zinc.Context) error {
		id, err := zinc.Param[int](c, "id")
		if err != nil {
			return err // 400: {"fields":{"id":"must be an integer"}}
		}

		var input WidgetInput
		if err := c.Bind().JSON(&input); err != nil {
			return err
		}
		if input.Name == "" {
			return zinc.UnprocessableEntity("name is required")
		}

		store.mu.Lock()
		if _, ok := store.items[id]; !ok {
			store.mu.Unlock()
			return zinc.NotFound("widget not found")
		}
		widget := Widget{ID: id, Name: input.Name}
		store.items[id] = widget
		store.mu.Unlock()

		return c.JSON(widget)
	})

	widgets.Delete("/{id}", func(c *zinc.Context) error {
		id, err := zinc.Param[int](c, "id")
		if err != nil {
			return err // 400: {"fields":{"id":"must be an integer"}}
		}

		store.mu.Lock()
		if _, ok := store.items[id]; !ok {
			store.mu.Unlock()
			return zinc.NotFound("widget not found")
		}
		delete(store.items, id)
		store.mu.Unlock()

		return c.NoContent()
	})

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

Create a widget, then read it back:

```bash
curl -i http://localhost:8080/widgets \
  -H 'Content-Type: application/json' \
  --data '{"name":"bracket"}'
# HTTP/1.1 201 Created
# Content-Type: application/json; charset=utf-8
#
# {"id":1,"name":"bracket"}

curl http://localhost:8080/widgets
# [{"id":1,"name":"bracket"}]

curl http://localhost:8080/widgets/1
# {"id":1,"name":"bracket"}
```

Rename it, then delete it:

```bash
curl -X PUT http://localhost:8080/widgets/1 \
  -H 'Content-Type: application/json' \
  --data '{"name":"hinge"}'
# {"id":1,"name":"hinge"}

curl -i -X DELETE http://localhost:8080/widgets/1
# HTTP/1.1 204 No Content

curl -i http://localhost:8080/widgets/1
# HTTP/1.1 404 Not Found
#
# {"error":{"status":404,"message":"widget not found"}}
```

Bad input gets a `400` or a `422`:

```bash
curl http://localhost:8080/widgets/abc
# {"error":{"status":400,"message":"invalid path parameter","fields":{"id":"must be an integer"}}}

curl http://localhost:8080/widgets -H 'Content-Type: application/json' --data '{"name":5}'
# {"error":{"status":400,"message":"invalid request body","fields":{"name":"must be a string"}}}

curl http://localhost:8080/widgets -H 'Content-Type: application/json' --data '{"name":""}'
# {"error":{"status":422,"message":"name is required"}}
```

## How it works

- `app.Group("/widgets")` gives every route the `/widgets` prefix, so each handler registers only the part after it.
- `c.Bind().JSON(&input)` decodes the body into `WidgetInput`. A body that isn't valid JSON, or a field of the wrong type, comes back as a `400`.
- `zinc.Param[int](c, "id")` reads `{id}` as an `int`. When it isn't a number, returning the error sends the `400` that names the field.
- `zinc.NotFound` and `zinc.UnprocessableEntity` return errors that Zinc turns into the JSON error bodies shown above.
- The `sync.RWMutex` in `widgetStore` keeps the map safe when requests run at the same time. `GET` routes take the read lock, so they don't block each other.

## Before production

- The store lives in memory, so every widget is gone when the process stops. The [SQLite CRUD API](/cookbook/sqlite-crud-api/) keeps them in a database.
- `GET /widgets` returns widgets in random order, because Go maps have no order. Sort the slice, or read from a database with `ORDER BY`.
- To check input with rules instead of `if` statements, plug in a validator. See [Validate Input](/cookbook/validation/).

## See also

- [Typed CRUD API](/cookbook/typed-crud/): the same API with typed handlers.
- [Binding](/guide/binding/): every way to read a request into a struct.
- [Errors](/guide/errors/): how returned errors become responses.
