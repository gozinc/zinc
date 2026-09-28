---
title: CORS for a Browser Front End
description: Let a web page on one origin call your Zinc API on another, with preflight requests answered for you.
---

This program runs a JSON API on port 8080 and a small web page on port 3000 that calls it with `fetch`. You'd need this whenever your front end and API live on different origins: a Vite dev server and a Go API, or `app.example.com` calling `api.example.com`. The [`cors`](/middleware/cors/) middleware sends the headers that let the browser read the API's responses.

## The program

```go title="main.go"
package main

import (
	"log"
	"os"
	"strconv"
	"sync"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/cors"
)

type Note struct {
	Text string `json:"text"`
}

// The front end, served from a different origin than the API.
const page = `<!doctype html>
<title>Notes</title>
<form id="add"><input name="text" placeholder="A note"> <button>Add</button></form>
<p id="count"></p>
<ul id="notes"></ul>
<script>
  const api = "http://localhost:8080/api/notes";

  async function load() {
    const res = await fetch(api);
    const notes = await res.json();
    document.querySelector("#count").textContent = res.headers.get("X-Total-Count") + " notes";
    document.querySelector("#notes").innerHTML = "";
    for (const note of notes) {
      const li = document.createElement("li");
      li.textContent = note.text;
      document.querySelector("#notes").append(li);
    }
  }

  document.querySelector("#add").addEventListener("submit", async (event) => {
    event.preventDefault();
    const text = new FormData(event.target).get("text");
    await fetch(api, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ text }),
    });
    event.target.reset();
    load();
  });

  load();
</script>
`

func main() {
	// The origin allowed to call the API. Set FRONTEND_ORIGIN in production.
	origin := os.Getenv("FRONTEND_ORIGIN")
	if origin == "" {
		origin = "http://localhost:3000"
	}

	var (
		mu    sync.Mutex
		notes = []Note{{Text: "first note"}}
	)

	api := zinc.New()
	api.Use(cors.New(cors.Config{
		AllowOrigins:  []string{origin},
		ExposeHeaders: []string{"X-Total-Count"},
		MaxAge:        600, // browsers may reuse a preflight answer for 10 minutes
	}))

	api.Get("/api/notes", func(c *zinc.Context) error {
		mu.Lock()
		defer mu.Unlock()
		c.SetHeader("X-Total-Count", strconv.Itoa(len(notes)))
		return c.JSON(notes)
	})

	api.Post("/api/notes", func(c *zinc.Context) error {
		var note Note
		if err := c.Bind().JSON(&note); err != nil {
			return err
		}
		if note.Text == "" {
			return zinc.UnprocessableEntity("text is required")
		}
		mu.Lock()
		notes = append(notes, note)
		mu.Unlock()
		return c.Status(zinc.StatusCreated).JSON(note)
	})

	front := zinc.New()
	front.Get("/", func(c *zinc.Context) error {
		return c.HTML(page)
	})

	go func() { log.Fatal(front.Listen(":3000")) }()
	log.Fatal(api.Listen(":8080"))
}
```

## Try it

Open `http://localhost:3000` in a browser. The page loads the notes from `http://localhost:8080`, shows the count from the `X-Total-Count` header, and adds a note when you submit the form.

To see what the browser sees, send the `Origin` header yourself. A request from the allowed origin gets `Access-Control-Allow-Origin` back:

```bash
curl -i -H "Origin: http://localhost:3000" http://localhost:8080/api/notes
# HTTP/1.1 200 OK
# Access-Control-Allow-Origin: http://localhost:3000
# Access-Control-Expose-Headers: X-Total-Count
# Content-Type: application/json; charset=utf-8
# Vary: Origin
# Vary: Access-Control-Request-Method
# Vary: Access-Control-Request-Headers
# X-Total-Count: 1
#
# [{"text":"first note"}]
```

The page's `POST` sends `Content-Type: application/json`, so the browser first asks permission with a preflight `OPTIONS` request. The middleware answers it without reaching your handler:

```bash
curl -i -X OPTIONS http://localhost:8080/api/notes \
  -H "Origin: http://localhost:3000" \
  -H "Access-Control-Request-Method: POST" \
  -H "Access-Control-Request-Headers: content-type"
# HTTP/1.1 204 No Content
# Access-Control-Allow-Headers: Origin,Content-Type,Accept,Authorization
# Access-Control-Allow-Methods: GET,POST,HEAD,PUT,DELETE,PATCH
# Access-Control-Allow-Origin: http://localhost:3000
# Access-Control-Max-Age: 600
# Content-Type: text/plain; charset=utf-8
# Vary: Origin
# Vary: Access-Control-Request-Method
# Vary: Access-Control-Request-Headers
```

Then the `POST` itself:

```bash
curl -i -H "Origin: http://localhost:3000" -H "Content-Type: application/json" \
  -d '{"text":"from curl"}' http://localhost:8080/api/notes
# HTTP/1.1 201 Created
# Access-Control-Allow-Origin: http://localhost:3000
# Access-Control-Expose-Headers: X-Total-Count
#
# {"text":"from curl"}
```

A page on any other origin gets no `Access-Control-Allow-Origin`, so the browser won't let its script read the response or send the `POST`:

```bash
curl -i -H "Origin: http://evil.example" http://localhost:8080/api/notes
# HTTP/1.1 200 OK
# Content-Type: application/json; charset=utf-8
# Vary: Origin
# ...
# [{"text":"first note"},{"text":"from curl"}]

curl -i -X OPTIONS http://localhost:8080/api/notes \
  -H "Origin: http://evil.example" \
  -H "Access-Control-Request-Method: POST" \
  -H "Access-Control-Request-Headers: content-type"
# HTTP/1.1 204 No Content
# Allow: GET, HEAD, POST, OPTIONS
# Vary: Origin
# ...
```

Notice the first response still has a body. CORS tells the browser what a page may read; it doesn't stop the server answering. Protect data with authentication, not with CORS.

## How it works

- `api.Use(cors.New(...))` runs before routing, so it can answer preflight `OPTIONS` requests for any route, even though no `OPTIONS` route exists.
- `AllowOrigins` lists the exact origins allowed. The default of `http://localhost:3000` makes the program work locally; set `FRONTEND_ORIGIN` to your front end's origin when you deploy.
- `ExposeHeaders` lets the page's script read `X-Total-Count`. Without it, `res.headers.get("X-Total-Count")` returns `null`, even though the header is there.
- `MaxAge: 600` lets the browser reuse a preflight answer for 10 minutes instead of sending one before every `POST`.
- The `Vary` headers tell caches that the response depends on the `Origin`, so one origin's answer isn't served to another.

## Before production

- List every origin exactly, with scheme and port: `https://app.example.com` and `http://app.example.com` are different origins.
- If the page sends cookies (`fetch(url, { credentials: "include" })`), set `AllowCredentials: true` and add [CSRF](/middleware/csrf/) protection. Zinc panics at startup if you combine credentials with `*`.
- To add CORS headers to `/api` only, register it with `api.UsePrefix("/api", cors.New(...))`. Prefix middleware also runs before routing, so preflights still work.
- When one server hosts both page and API, you don't need CORS at all. Serve both from the same origin, or proxy `/api` through your front-end dev server.

## See also

- [CORS middleware](/middleware/cors/): defaults and every configuration field.
- [Groups and Middleware](/guide/groups-and-middleware/): middleware order, and `app.UsePrefix` for running it under `/api` only.
- [Single-Page App](/cookbook/spa-fallback/): serve the front end and the API from one origin.
