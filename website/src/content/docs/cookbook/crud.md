---
title: A Small CRUD API
description: Build a complete in-memory resource API with groups, typed input, errors, and safe concurrent access.
---

This recipe is deliberately small but complete. It uses a group for the
resource prefix, a typed request body, brace parameters, and Zinc errors for
missing records.

## Setup

```bash
mkdir zinc-widgets
cd zinc-widgets
go mod init example.com/zinc-widgets
go get github.com/0mjs/zinc
```

## Application

```go
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

```bash
curl -i http://localhost:8080/widgets \
  -H 'Content-Type: application/json' \
  --data '{"name":"bracket"}'

curl http://localhost:8080/widgets/1

curl -i -X PUT http://localhost:8080/widgets/1 \
  -H 'Content-Type: application/json' \
  --data '{"name":"hinge"}'

curl -i -X DELETE http://localhost:8080/widgets/1
```

The in-memory store is safe for concurrent requests but intentionally does not
survive restarts. Continue with the [SQLite CRUD API](/cookbook/sqlite-crud-api/) when
you want persistence.
