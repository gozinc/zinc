---
title: Typed CRUD API
description: Build an in-memory resource API where each handler's signature is its request contract.
---

This is the [CRUD API](/cookbook/crud/) recipe written with [typed handlers](/guide/typed-handlers/). Each handler declares what it reads and what it returns, and Zinc does the binding, validation, and response writing.

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

type WidgetID struct {
	ID int `path:"id"`
}

type WidgetInput struct {
	ID   int    `path:"id"`
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
	widgets.Get("/{id}", zinc.Typed(s.get)).Name("widgets.show")
	widgets.Put("/{id}", zinc.Typed(s.update))
	widgets.Delete("/{id}", zinc.Typed(s.remove))

	log.Fatal(app.Listen(":8080"))
}
```

```bash
curl -i -X POST localhost:8080/widgets -H 'Content-Type: application/json' -d '{"name":"sprocket"}'
# HTTP/1.1 201 Created
# {"id":1,"name":"sprocket"}

curl -i localhost:8080/widgets/abc
# HTTP/1.1 400 Bad Request
# {"error":{"status":400,"message":"invalid path parameter","fields":{"id":"must be an integer"}}}

curl -i -X DELETE localhost:8080/widgets/1
# HTTP/1.1 204 No Content
```

Compared with the hand-written recipe, the handlers contain no parsing, binding, or response code: an `id` that isn't a number is a `400` naming the field before the function runs, and the `DELETE` answers `204` because it returns `zinc.NoContent`. To check `name` with a validation library instead of by hand, set a [`Validator`](/guide/binding/#validation); failures answer `422`.
