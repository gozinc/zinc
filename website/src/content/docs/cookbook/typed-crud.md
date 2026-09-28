---
title: Typed CRUD API
description: Build the widgets API with typed handlers, where each function's signature says what it reads and what it returns.
---

This program is the [CRUD API](/cookbook/crud/) written with [typed handlers](/guide/typed-handlers/). Each handler takes a struct and returns a struct, and Zinc does the binding, the error answers and the JSON response. Reach for it when you want handlers that read like plain Go functions.

## Run it

```bash
mkdir zinc-typed-widgets && cd zinc-typed-widgets
go mod init example.com/zinc-typed-widgets
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

type WidgetID struct {
	ID int `path:"id" json:"-"` // from the path only; a body can't change it
}

type WidgetInput struct {
	ID   int    `path:"id" json:"-"` // from the path only; a body can't change it
	Name string `json:"name"`
}

type store struct {
	mu      sync.Mutex
	nextID  int
	widgets map[int]Widget
}

func (s *store) list(c *zinc.Context, _ struct{}) ([]Widget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Widget, 0, len(s.widgets))
	for _, w := range s.widgets {
		out = append(out, w)
	}
	return out, nil
}

func (s *store) create(c *zinc.Context, in WidgetInput) (Widget, error) {
	if in.Name == "" {
		return Widget{}, zinc.UnprocessableEntity("name is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	w := Widget{ID: s.nextID, Name: in.Name}
	s.widgets[w.ID] = w
	return w, nil
}

func (s *store) get(c *zinc.Context, in WidgetID) (Widget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.widgets[in.ID]
	if !ok {
		return Widget{}, zinc.NotFound("widget not found")
	}
	return w, nil
}

func (s *store) update(c *zinc.Context, in WidgetInput) (Widget, error) {
	if in.Name == "" {
		return Widget{}, zinc.UnprocessableEntity("name is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.widgets[in.ID]; !ok {
		return Widget{}, zinc.NotFound("widget not found")
	}
	w := Widget{ID: in.ID, Name: in.Name}
	s.widgets[in.ID] = w
	return w, nil
}

func (s *store) remove(c *zinc.Context, in WidgetID) (zinc.NoContent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.widgets[in.ID]; !ok {
		return zinc.NoContent{}, zinc.NotFound("widget not found")
	}
	delete(s.widgets, in.ID)
	return zinc.NoContent{}, nil
}

func main() {
	s := &store{widgets: map[int]Widget{}}
	app := zinc.New()

	widgets := app.Group("/widgets")
	widgets.Get("/", zinc.Typed(s.list))
	widgets.Post("/", zinc.Typed(s.create)).Status(zinc.StatusCreated)
	widgets.Get("/{id}", zinc.Typed(s.get))
	widgets.Put("/{id}", zinc.Typed(s.update))
	widgets.Delete("/{id}", zinc.Typed(s.remove))

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

```bash
curl -i -X POST localhost:8080/widgets -H 'Content-Type: application/json' -d '{"name":"sprocket"}'
# HTTP/1.1 201 Created
# Content-Type: application/json; charset=utf-8
#
# {"id":1,"name":"sprocket"}

curl -X PUT localhost:8080/widgets/1 -H 'Content-Type: application/json' -d '{"name":"gear"}'
# {"id":1,"name":"gear"}

curl localhost:8080/widgets
# [{"id":1,"name":"gear"}]

curl -i -X DELETE localhost:8080/widgets/1
# HTTP/1.1 204 No Content
```

Errors come back before or from your function:

```bash
curl -i localhost:8080/widgets/abc
# HTTP/1.1 400 Bad Request
#
# {"error":{"status":400,"message":"invalid path parameter","fields":{"id":"must be an integer"}}}

curl -i localhost:8080/widgets/9
# HTTP/1.1 404 Not Found
#
# {"error":{"status":404,"message":"widget not found"}}

curl -X POST localhost:8080/widgets -H 'Content-Type: application/json' -d '{}'
# {"error":{"status":422,"message":"name is required"}}
```

## How it works

- `zinc.Typed(s.create)` turns a `func(*zinc.Context, In) (Out, error)` into a normal handler. It binds `In` from the path, query, headers and body, then writes `Out` as JSON.
- `path:"id"` fills `ID` from `{id}`. An `id` that isn't a number is a `400` naming the field, before your function runs.
- `json:"-"` on the path fields stops a body from setting them. Go's JSON decoder ignores case, so without it a body such as `{"ID":2}` on `DELETE /widgets/1` would delete widget 2.
- `.Status(zinc.StatusCreated)` on the `POST` route makes a successful create answer `201` instead of `200`.
- `remove` returns `zinc.NoContent`, so a successful delete answers `204` with no body.

## Before production

- The store is in memory, so widgets are gone when the process stops. See the [SQLite CRUD API](/cookbook/sqlite-crud-api/) for a database-backed version.
- To check `name` with rules instead of by hand, set a [`Validator`](/guide/binding/#validation). Failures answer `422` before your function runs. [Validate Input](/cookbook/validation/) shows the full setup.

## See also

- [Typed Handlers](/guide/typed-handlers/): every rule for `zinc.Typed`, inputs and outputs.
- [CRUD API](/cookbook/crud/): the same API with hand-written handlers.
- [Validate Input](/cookbook/validation/): rules on struct fields, and `422` answers that name them.
