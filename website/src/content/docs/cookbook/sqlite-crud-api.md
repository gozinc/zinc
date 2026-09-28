---
title: SQLite CRUD API
description: Build a to-do API that keeps its data in a SQLite file, with JSON in and out and 404 and 422 answers.
---

This program is a to-do API that stores its data in SQLite: create, list, complete and delete to-dos, with JSON in and out. It's the [CRUD API](/cookbook/crud/) with a database behind it, so your data survives a restart. It uses Go's `database/sql`, so the handlers look the same with Postgres or MySQL.

## Run it

```bash
mkdir zinc-sqlite && cd zinc-sqlite
go mod init example.com/zinc-sqlite
go get github.com/0mjs/zinc github.com/mattn/go-sqlite3
```

`go-sqlite3` uses cgo, so you need a C compiler, such as Xcode's command line tools on macOS or `gcc` on Linux. Save the program as `main.go` and run `go run .`. It listens on port 8080 and creates `todo.db` in the current directory.

## The program

```go title="main.go"
package main

import (
	"database/sql"
	"log"

	"github.com/0mjs/zinc"
	_ "github.com/mattn/go-sqlite3"
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
			return err // 400 with the failing field
		}
		if input.Title == "" {
			return zinc.UnprocessableEntity("title is required")
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
		id, err := zinc.Param[int](c, "id")
		if err != nil {
			return err // 400 when the id isn't a number
		}
		result, err := db.Exec("update todos set done = 1 where id = ?", id)
		if err != nil {
			return err
		}
		if n, _ := result.RowsAffected(); n == 0 {
			return zinc.NotFound("todo not found")
		}
		return c.NoContent()
	})

	app.Delete("/todos/{id}", func(c *zinc.Context) error {
		id, err := zinc.Param[int](c, "id")
		if err != nil {
			return err
		}
		result, err := db.Exec("delete from todos where id = ?", id)
		if err != nil {
			return err
		}
		if n, _ := result.RowsAffected(); n == 0 {
			return zinc.NotFound("todo not found")
		}
		return c.NoContent()
	})

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

```bash
curl -i -X POST http://localhost:8080/todos \
  -H "Content-Type: application/json" \
  -d '{"title":"ship docs"}'
# HTTP/1.1 201 Created
# Content-Type: application/json; charset=utf-8
#
# {"id":1,"title":"ship docs","done":false}

curl -i -X PATCH http://localhost:8080/todos/1/done
# HTTP/1.1 204 No Content

curl http://localhost:8080/todos
# [{"id":1,"title":"ship docs","done":true}]

curl -i -X DELETE http://localhost:8080/todos/1
# HTTP/1.1 204 No Content
```

A missing to-do is a `404`, and an empty title is a `422`:

```bash
curl -X PATCH http://localhost:8080/todos/99/done
# {"error":{"status":404,"message":"todo not found"}}

curl -X POST http://localhost:8080/todos -H "Content-Type: application/json" -d '{"title":""}'
# {"error":{"status":422,"message":"title is required"}}
```

Stop the server and start it again: the to-dos you created are still there.

## How it works

- `sql.Open("sqlite3", "file:todo.db?_fk=1")` opens (or creates) the database file, and the `create table if not exists` statement sets up the table on first run.
- `db.Query` and `rows.Scan` read rows into `Todo` values. `todos` starts as an empty slice, so an empty table answers `[]` rather than `null`.
- Returning a database error from a handler sends a `500`. The client sees a generic message, not the SQL error.
- `result.RowsAffected()` tells `PATCH` and `DELETE` whether the row existed. Zero rows means the id wasn't found, so they answer `404`.
- `?` placeholders pass values to SQLite separately from the SQL, so a title can't change the query.

## Before production

- Set `db.SetMaxOpenConns(1)` if you see `database is locked` errors under concurrent writes, or enable WAL mode with `_journal_mode=WAL` in the connection string.
- Pass `c.Context()` to `db.QueryContext` and `db.ExecContext`, so a query stops when the client goes away.
- For a pure-Go build without cgo, swap the driver for `modernc.org/sqlite` (driver name `sqlite`).

## See also

- [CRUD API](/cookbook/crud/): the in-memory version with a full set of routes.
- [Errors](/guide/errors/): how returned errors become responses.
- [Validate Input](/cookbook/validation/): check input with rules on struct fields.
