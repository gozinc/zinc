---
title: OpenAPI
description: Describe your API as an OpenAPI 3.1 spec from the types you already write, serve it, and show a docs page. No comments to maintain.
---

Zinc can describe your API as an [OpenAPI 3.1](https://spec.openapis.org/oas/v3.1.0) spec: every route, its parameters, request body and responses. Use it to show a browsable docs page, generate client code, or check a change doesn't break your API's shape.

The spec comes from your code, not from comments. A typed handler already says what it takes and returns, so it needs nothing extra:

```go
package main

import (
	"log"
	"net/http"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/apidocs"
)

type Pet struct {
	ID   int64  `json:"id" doc:"The pet's ID." example:"7"`
	Name string `json:"name" validate:"required,min=1,max=40"`
	Kind string `json:"kind" validate:"oneof=cat dog"`
}

type CreatePet struct {
	Store string `path:"store"`
	Name  string `json:"name" validate:"required,min=1,max=40"`
	Kind  string `json:"kind" validate:"oneof=cat dog"`
}

func main() {
	app := zinc.New()

	app.Post("/stores/{store}/pets", zinc.Typed(func(c *zinc.Context, in CreatePet) (Pet, error) {
		return Pet{ID: 7, Name: in.Name, Kind: in.Kind}, nil
	})).Status(http.StatusCreated).Summary("Add a pet").Tags("pets")

	app.OpenAPI("/openapi.json", zinc.OpenAPIConfig{Title: "Pet Store", Version: "1.0.0"})
	app.Get("/docs", apidocs.New()).Hidden()

	log.Fatal(app.Listen(":8080"))
}
```

Open `http://localhost:8080/docs` for the docs page, or fetch the spec itself:

```bash
curl http://localhost:8080/openapi.json
```

```json
"/stores/{store}/pets": {
  "post": {
    "tags": ["pets"],
    "summary": "Add a pet",
    "parameters": [
      { "name": "store", "in": "path", "required": true, "schema": { "type": "string" } }
    ],
    "requestBody": {
      "content": { "application/json": { "schema": { "$ref": "#/components/schemas/CreatePetBody" } } }
    },
    "responses": {
      "201": { "description": "Created", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Pet" } } } },
      "400": { "description": "Bad Request", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Error" } } } },
      "500": { "description": "Internal Server Error", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Error" } } } }
    }
  }
}
```

The spec is off until you call `app.OpenAPI`, so an app never exposes its routes' shape by accident.

## What a typed handler gives the spec

| From | Becomes |
|---|---|
| `path:"store"` fields | Path parameters, with the field's type. Every `{name}` in the route is listed, as a string if no field binds it. |
| `query` and `header` fields | Query and header parameters. `validate:"required"` makes one required. |
| `json` fields | The JSON request body. Fields bound from the path, query or headers are left out of it. A field is required only with `validate:"required"` and a validator. |
| `form` fields | A form body, or `multipart/form-data` when a field holds a file. |
| The output type | The success response. `zinc.NoContent` gives `204` with no body. Fields without `omitempty` are required, since they're always sent. |
| `.Status(201)` | The success status. |
| A route with input | A `400` response, and a `422` when the app has a [validator](/guide/binding/#validation). |
| Every route | A `500` response. |

Routes are listed in the order you register them. GET and HEAD routes never get a request body in the spec.

## Describe a plain handler

A handler that isn't typed is still in the spec, with its method, path and path parameters. Tell Zinc its input and output types to describe the rest:

```go
app.Put("/pets/{id}", func(c *zinc.Context) error {
	var in CreatePet
	if err := c.Bind().All(&in); err != nil {
		return err
	}
	return c.JSON(update(in)) // update: your code
}).Input(CreatePet{}).Output(Pet{})
```

`Input` and `Output` take a value of the type. Zinc doesn't check that the handler really uses them, so keep them in step with the code. On a typed handler they panic: its types are already known.

## Add summaries, tags and more

Every route has these methods. Each returns the route, so they chain:

| Method | What it does |
|---|---|
| `.Name("createPet")` | Names the route, and sets the operation ID |
| `.Summary("Add a pet")` | One line shown in docs pages |
| `.Description("…")` | Longer text; Markdown works |
| `.Tags("pets")` | Groups the route in docs pages |
| `.Deprecated()` | Marks the route deprecated; it still serves requests |
| `.Hidden()` | Leaves the route out of the spec |
| `.Errors(404, 409)` | Adds error statuses the handler answers by returning an error, such as `zinc.NotFound(...)` |
| `.Response(409, Conflict{})` | Adds a response the handler writes itself; pass `nil` for one with no body |
| `.Security("bearer")` | Names the security schemes that protect the route; with no names, marks it public |

Unset fields stay out of the spec. Zinc doesn't invent summaries or operation IDs.

Use `Errors` for a status your handler reaches by returning an error: the error handler writes that response, so the spec describes it with the error handler's body. Use `Response` only when the handler writes the body itself:

```go
app.Get("/pets/{id}", zinc.Typed(func(c *zinc.Context, in PetID) (Pet, error) {
	pet, ok := pets.Find(in.ID) // pets: your store
	if !ok {
		return Pet{}, zinc.NotFound("no such pet")
	}
	return pet, nil
})).Errors(http.StatusNotFound)
```

## Tag and protect a group

`Tags` and `Security` on a group apply to every route registered in it afterwards, and to its child groups. Like `Use`, call them before the group's first route:

```go
cfg := zinc.OpenAPIConfig{
	Title:   "Pet Store",
	Version: "1.0.0",
	SecuritySchemes: map[string]zinc.OpenAPISecurityScheme{
		"bearer": {Type: "http", Scheme: "bearer", BearerFormat: "JWT"},
	},
}

admin := app.Group("/admin", requireToken).Tags("admin").Security("bearer") // requireToken: your auth middleware
admin.Delete("/pets/{id}", zinc.Typed(deletePet))
app.OpenAPI("/openapi.json", cfg)
```

`Security` only documents how the route is protected. Enforcing it is your middleware's job, here `requireToken`. A scheme name that isn't in `SecuritySchemes` is an error when the spec is built.

## Serve the spec

`app.OpenAPI(path, cfg)` adds a hidden `GET` route for the spec. The spec is built on the first request and kept; routes you register later are picked up on the next one.

Pass middleware to protect it:

```go
app.OpenAPI("/openapi.json", cfg, requireAPIKey) // requireAPIKey: your middleware
```

| `OpenAPIConfig` field | Default | Meaning |
|---|---|---|
| `Title` | Your main module's name | The API's name |
| `Version` | Your main module's version, or `0.0.0` | Your API's version, not Zinc's |
| `Description` | none | Markdown |
| `Servers` | none | Base URLs, such as `{URL: "https://api.example.com"}` |
| `SecuritySchemes` | none | The schemes `Security` refers to, by name |
| `Security` | none | Schemes for every route that doesn't set its own |

To show the spec as a page, add [API Docs](/middleware/apidocs/).

## Write the spec to a file

`app.OpenAPISpec(cfg)` returns the spec as JSON without serving anything. Build it in a test to keep a copy in your repository, and a change to the API's shape shows up in the diff:

```go
func TestOpenAPISpec(t *testing.T) {
	spec, err := newApp().OpenAPISpec(specConfig) // newApp, specConfig: your code
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("openapi.json", spec, 0o644); err != nil {
		t.Fatal(err)
	}
}
```

The output is the same for the same routes, so the file only changes when the API does.

## How Go types become schemas

| Go | Schema |
|---|---|
| `string`, `bool` | `string`, `boolean` |
| integers | `integer` with format `int32` or `int64`; unsigned types have `minimum: 0` |
| `float32`, `float64` | `number` with format `float` or `double` |
| `[]T`, `[N]T` | `array` |
| `[]byte` | a base64 string |
| `map[string]T` | `object` with `additionalProperties` |
| `*T` | T, or `null` |
| `time.Time` | a `date-time` string |
| named structs | a shared schema under `components/schemas`, referenced with `$ref` |

Struct fields follow `encoding/json`: `json` tag names, `-`, `,string`, and embedded structs. Three more tags add detail:

| Tag | Adds |
|---|---|
| `validate:"required,min=1,max=40"` | Required fields, lengths, ranges, `email`, `uuid` and `url` formats, and `oneof` choices. Only when the app has a validator. |
| `doc:"The pet's ID."` | A description |
| `example:"7"` | An example value |

With a [validator](/guide/binding/#validation) configured, the `Pet` above becomes:

```json
"Pet": {
  "type": "object",
  "properties": {
    "id": { "type": "integer", "format": "int64", "description": "The pet's ID.", "examples": [7] },
    "name": { "type": "string", "minLength": 1, "maxLength": 40 },
    "kind": { "type": "string", "enum": ["cat", "dog"] }
  },
  "required": ["name"]
}
```

Without a validator, the same schema keeps `description` and `examples` but drops `minLength`, `maxLength`, `enum` and `required`: nothing would enforce them.

When a type's JSON doesn't match its Go shape, such as one with its own `MarshalJSON`, describe it yourself with `zinc.SchemaProvider`:

```go
type Date struct{ time.Time }

func (Date) OpenAPISchema() map[string]any {
	return map[string]any{"type": "string", "format": "date"}
}
```

## Good to know

### Validation rules appear only with a validator

Zinc doesn't check `validate` tags itself: a [validator](/guide/binding/#validation) you configure does. Without one, nothing enforces the rules, so the spec leaves them out: no required fields, lengths or choices, and no `422`. `doc` and `example` tags always apply.

### Error bodies follow your error handler

With Zinc's default error handler, the `400`, `422` and `500` responses describe its error body, the `Error` schema. With your own `ErrorHandler`, Zinc can't know what you send, so those responses list the status alone.

### A nil slice is sent as null

`encoding/json` writes a nil slice as `null`, while the spec says `array`. Initialise slices you return, such as `pets := []Pet{}`, to match.

### Wrapping a typed handler hides its types

Register `zinc.Typed(...)` directly. A typed handler called from inside a plain one isn't recognised; add `.Input` and `.Output` to that route instead.

### A body can still fill a query or header field

Binding reads the body before the URL, so a query parameter or header the request leaves out can be set by a matching body key. The spec leaves those fields out of the body, which describes how the route should be called.

### What's left out

Hidden routes, mounts and static files, and methods OpenAPI 3.1 has no field for, such as `PURGE`.

## Next steps

- [API Docs](/middleware/apidocs/): show the spec as a page with Scalar, Swagger UI, Stoplight Elements or ReDoc.
- [Typed Handlers](/guide/typed-handlers/): write handlers whose types describe themselves.
- [Binding](/guide/binding/): the tags the spec reads, and how to add a validator.
