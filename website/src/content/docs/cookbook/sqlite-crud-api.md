---
title: SQLite CRUD API
description: A compact API example with request binding, route params, and JSON responses.
---

This recipe is the simplest "real API" example in the cookbook:

- SQLite storage
- request binding
- route params
- JSON responses
- create, list, update, and delete flows

## Setup

```bash
go mod init zinc-sqlite
go get github.com/0mjs/zinc
go get github.com/mattn/go-sqlite3
```

## Application

```go
package main

import (
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3"
	"github.com/0mjs/zinc"
)

type Todo struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

type CreateTodoInput struct {
	Title string `json:"title"`
}

func main() {
	db, err := sql.Open("sqlite3", "file:todo.db?_fk=1")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`
		create table if not exists todos (
			id integer primary key autoincrement,
			title text not null,
			done boolean not null default 0
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	app := zinc.New()

	app.Get("/todos", func(c *zinc.Context) error {
		rows, err := db.Query("select id, title, done from todos order by id asc")
		if err != nil {
			return err
		}
		defer rows.Close()

		todos := make([]Todo, 0)
		for rows.Next() {
			var t Todo
			if err := rows.Scan(&t.ID, &t.Title, &t.Done); err != nil {
				return err
			}
			todos = append(todos, t)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		return c.JSON(todos)
	})

	app.Post("/todos", func(c *zinc.Context) error {
		var input CreateTodoInput
		if err := c.Bind().JSON(&input); err != nil {
			return zinc.ErrBadRequest.WithMessage("invalid JSON body").WithCause(err)
		}
		if input.Title == "" {
			return zinc.ErrBadRequest.WithMessage("title is required")
		}

		result, err := db.Exec("insert into todos(title, done) values(?, 0)", input.Title)
		if err != nil {
			return err
		}

		id, _ := result.LastInsertId()
		return c.Status(zinc.StatusCreated).JSON(Todo{
			ID:    int(id),
			Title: input.Title,
			Done:  false,
		})
	})

	app.Patch("/todos/{id}/done", func(c *zinc.Context) error {
		id := c.Param("id")
		if id == "" {
			return zinc.ErrBadRequest.WithMessage("id is required")
		}

		_, err := db.Exec("update todos set done = 1 where id = ?", id)
		if err != nil {
			return err
		}

		return c.NoContent()
	})

	app.Delete("/todos/{id}", func(c *zinc.Context) error {
		id := c.Param("id")
		if id == "" {
			return zinc.ErrBadRequest.WithMessage("id is required")
		}

		_, err := db.Exec("delete from todos where id = ?", id)
		if err != nil {
			return err
		}

		return c.NoContent()
	})

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

```bash
curl -X POST http://localhost:8080/todos \
  -H "Content-Type: application/json" \
  -d '{"title":"ship docs"}'

curl http://localhost:8080/todos
curl -X PATCH http://localhost:8080/todos/1/done
curl -X DELETE http://localhost:8080/todos/1
```

## Why this recipe matters

This is the most direct example of Zinc as an API framework:

- bind request input
- validate what you need
- use route params
- return JSON and status codes
- keep the handler flow explicit
