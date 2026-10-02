---
title: OpenAPI
description: Zinc describes your API as an OpenAPI 3.1 spec from the types you already write, serves it, and can show a docs page. No comments to maintain.
---

Zinc describes your API as an [OpenAPI 3.1](https://spec.openapis.org/oas/v3.1.0) spec: every route, its parameters, request body and responses. Every app serves it at `/openapi.json`. Use it to show a browsable docs page, generate client code, or check a change doesn't break your API's shape.

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
	app := zinc.New(zinc.Config{
		OpenAPI: zinc.OpenAPIConfig{Title: "Pet Store", Version: "1.0.0"},
	})

	app.Post("/stores/{store}/pets", zinc.Typed(func(c *zinc.Context, in CreatePet) (Pet, error) {
		return Pet{ID: 7, Name: in.Name, Kind: in.Kind}, nil
	})).Status(http.StatusCreated).Summary("Add a pet").Tags("pets")

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
      "422": { "description": "Unprocessable Entity", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Error" } } } },
      "500": { "description": "Internal Server Error", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Error" } } } }
    }
  }
}
```

The spec is public by default. For a private API, [protect it or turn it off](#serve-the-spec).

## What a typed handler gives the spec

| From | Becomes |
|---|---|
| `path:"store"` fields | Path parameters, with the field's type. Every `{name}` in the route is listed, as a string if no field binds it. |
| `query`, `header` and `cookie` fields | Query, header and cookie parameters. `validate:"required"` makes one required, and `default:"20"` documents the value [binding](/guide/binding/#default-values) fills in when the request leaves it out. |
| `json` fields | The JSON request body. Fields bound from the path, query or headers are left out of it. A field is required with `validate:"required"`, or when its rules reject a missing value. The same schema is listed as `application/xml` when the input has `xml` tags, and under each [configured decoder](/guide/customization/#body-formats)'s media type. |
| `form` fields | A form body, or `multipart/form-data` when a field holds a file. A field tagged both `json` and `form` is in both bodies, since binding reads it from either. A file field's `media:"image/png"` tag documents the part's content type, and binding rejects other types. |
| The output type | The success response. `zinc.NoContent` gives `204` with no body. Fields without `omitempty` are required, since they're always sent. The [output types](/guide/typed-handlers/#send-text-files-and-redirects) document their media type: `text/plain`, `text/html`, `application/octet-stream`, or a redirect with `Location`. |
| `.Status(201)` | The success status. |
| A route with input | A `400` response, and a `422` when the input has [validation](/guide/binding/#validation) rules or enums, or the app has its own `Validator`. |
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

Without `Output`, a `.Response` or `.Produces` for a success status, or `.Status`, Zinc can't know what the handler sends, so it doesn't guess. The spec lists a `default` response with a body of any type:

```json
"default": {
  "description": "The handler's response. Route.Output or Route.Response describes it.",
  "content": { "*/*": {} }
}
```

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
| `.Example(201, "a cat", Pet{...})` | Adds a named example of a response; see [Add examples](#add-examples) |
| `.RequestExample("a cat", CreatePet{...})` | Adds a named example of the request body |
| `.Response(409, Conflict{})` | Adds a response the handler writes itself; pass `nil` for one with no body |
| `.Produces(200, "text/csv")` | The media types a status is sent as, when Zinc can't tell: a plain handler, or a `Bytes`, `File` or `Stream` output |
| `.Consumes("text/csv")` | The media types the request body is accepted as, in place of the ones Zinc infers |
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

## Add examples

Give a response or the request body named examples, written as Go values. Zinc encodes each as clients would receive it and puts it beside the schema:

```go
app.Post("/pets", zinc.Typed(createPet)).
	Status(http.StatusCreated).
	RequestExample("a cat", CreatePet{Name: "Tom"}).
	Example(http.StatusCreated, "a cat", Pet{ID: "7", Name: "Tom"}).
	Errors(http.StatusConflict).
	Example(http.StatusConflict, "name taken", zinc.NewError(http.StatusConflict, "a pet named Tom exists"))
```

```json
"409": {
  "description": "Conflict",
  "content": {
    "application/json": {
      "schema": { "$ref": "#/components/schemas/Error" },
      "examples": {
        "name taken": {
          "value": { "error": { "status": 409, "message": "a pet named Tom exists" } }
        }
      }
    }
  }
}
```

An error example is an `*HTTPError`, shown as your error handler writes it. A request example leaves out fields bound from the path, query, headers or cookies, and nil fields.

Each example is checked when the spec is built. A value of the wrong type is an error that names the route:

```text
zinc: GET /: example "x" for 200: value is a main.petRecord; the response is a main.Pet
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

app := zinc.New(zinc.Config{OpenAPI: cfg})
admin := app.Group("/admin", requireToken).Tags("admin").Security("bearer") // requireToken: your auth middleware
admin.Delete("/pets/{id}", zinc.Typed(deletePet))
```

`Security` only documents how the route is protected. Enforcing it is your middleware's job, here `requireToken`. A scheme name that isn't in `SecuritySchemes` is an error when the spec is built.

| Call | Means |
|---|---|
| `.Security("key", "bearer")` | Either scheme is enough |
| `.SecurityAll("key", "bearer")` | Both are needed, together |
| `.Security("oauth:pets:read")` | Scheme `oauth`, with scope `pets:read` |
| `.Security()` | Public, overriding the group's or the default |

A secured route documents a `401`, and a `403` when it needs a scope, since its auth answers with them. Set `NoAuthResponses` in `OpenAPIConfig` to leave them out.

`Hidden` on a group leaves its routes, and its child groups' routes, out of the spec:

```go
internal := app.Group("/internal").Hidden()
```

An OAuth 2 scheme lists its flows:

```go
"oauth": {Type: "oauth2", Flows: &zinc.OpenAPIOAuthFlows{
	AuthorizationCode: &zinc.OpenAPIOAuthFlow{
		AuthorizationURL: "https://id.example.com/authorize",
		TokenURL:         "https://id.example.com/token",
		Scopes:           map[string]string{"pets:read": "Read pets"},
	},
}},
```

A scheme missing something its type needs, such as an `oauth2` scheme without flows, is also an error, since the spec would be invalid.

## Serve the spec

Every app serves its spec at `/openapi.json`, and a browsable reference page for it at `/docs`, for `GET` and `HEAD`. Describe the API with `Config.OpenAPI`:

```go
app := zinc.New(zinc.Config{
	OpenAPI: zinc.OpenAPIConfig{Title: "Pet Store", Version: "1.0.0"},
})
```

The spec is built on the first request and kept; routes you register later are picked up on the next one. Neither endpoint is a route: a route of your own at the same path wins, and neither is listed in the spec. `Routes()` lists both, with `Builtin` set. Middleware added with `app.Use` runs for them like any request.

The page at `/docs` loads [Scalar](https://github.com/scalar/scalar) from a CDN, pinned to one version with an integrity hash. To use another renderer or your own copy of the files, turn the page off and add [API Docs](/middleware/apidocs/).

:::caution[The spec and docs are public by default]
The spec lists every route that isn't hidden, including admin and internal ones. For a private API, protect them or turn them off.
:::

| To | Do |
|---|---|
| Serve the spec at another path | `zinc.Config{OpenAPIPath: "/api/openapi.json"}`; the page follows it |
| Serve the page at another path | `zinc.Config{DocsPath: "/reference"}` |
| Serve no page | `zinc.Config{DocsPath: "-"}` |
| Serve no spec, and so no page | `zinc.Config{OpenAPIPath: "-"}` |
| Protect it with middleware | `app.OpenAPI("/openapi.json", cfg, requireAPIKey)` (`requireAPIKey`: your middleware). It replaces the default spec. |
| Serve several specs | Call `app.OpenAPI` once for each path and config |

| `OpenAPIConfig` field | Default | Meaning |
|---|---|---|
| `Title` | Your main module's name | The API's name |
| `Version` | Your main module's version, or `0.0.0` | Your API's version, not Zinc's |
| `Description` | none | Markdown |
| `TermsOfService` | none | A URL |
| `Contact` | none | `&zinc.OpenAPIContact{Name, URL, Email}` |
| `License` | none | `&zinc.OpenAPILicense{Name: "MIT", Identifier: "MIT"}`; an SPDX `Identifier` or a `URL`, not both |
| `ExternalDocs` | none | `&zinc.OpenAPIExternalDocs{URL: "https://example.com/docs"}` |
| `Servers` | none | Base URLs, such as `{URL: "https://api.example.com"}` |
| `Tags` | none | Tag descriptions, in the order docs pages list them. Tags that routes use but `Tags` leaves out follow, in the order they're used |
| `SecuritySchemes` | none | The schemes `Security` refers to, by name |
| `Security` | none | Schemes for every route that doesn't set its own |
| `NoAuthResponses` | `false` | Leaves out the `401` and `403` that secured routes get |
| `Schemas` | none | Schemas for types from other packages; see [below](#types-from-other-packages) |

A config the spec can't be valid with, such as a license without a name, panics in `zinc.New` and `app.OpenAPI`, and is an error from `app.OpenAPISpec`.

To show the spec as a page, add [API Docs](/middleware/apidocs/).

## Edit the spec

For what no option covers, edit the spec as JSON. `Extensions` adds `x-` members to its root, `Route.Operation` edits one operation, and `Mutate` edits the whole spec last:

```go
app.Get("/pets", listPets).Operation(func(op map[string]any) { // listPets: your handler
	op["x-rate-limit"] = 100
})

app.OpenAPI("/openapi.json", zinc.OpenAPIConfig{
	Title: "Pet Store", Version: "1.0.0",
	Extensions: map[string]any{"x-logo": map[string]any{"url": "https://example.com/logo.png"}},
	Mutate: func(spec map[string]any) error {
		spec["info"].(map[string]any)["x-audience"] = "partners"
		return nil
	},
})
```

Zinc keeps its own order and adds new members after it. The result is checked again, so a hook can't leave the spec broken:

```text
zinc: the spec after its hooks: #/paths/~1pets/get/responses/200: $ref #/components/responses/Pets doesn't resolve
```

## Check the app before serving

`app.Validate()` builds every spec the app serves and checks the routes, without a request. Call it at startup or in a test, so a mistake fails there:

```go
if err := app.Validate(); err != nil {
	log.Fatal(err)
}
```

```text
zinc: paths /pets/{id} and /pets/{name} differ only in parameter names, which OpenAPI doesn't allow; give the parameters the same names
POST /pets: main.Created.Location has header:"Location" but is sent in the JSON body, so no header is sent; add json:"-" to send it as a header
```

It also reports `openapi` tag words it doesn't know, and validate rules the validator doesn't enforce on types given to `.Input` and `.Output`.

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
| integers | `integer` with format `int32` or `int64`. 8- and 16-bit types, and `uint32`, have their range as `minimum` and `maximum`; unsigned types have `minimum: 0`; `uint`, `uint64` and `uintptr` have no format, since `int64` can't hold their largest values |
| `float32`, `float64` | `number` with format `float` or `double` |
| `[]T`, `[N]T` | `array`; in a response, a slice can also be `null` |
| `[]byte` | a base64 string |
| `map[string]T` | `object` with `additionalProperties`; in a response, it can also be `null` |
| `*T` | T, or `null` |
| `time.Time` | a `date-time` string |
| `json.Number` | `number` |
| `uuid.UUID` ([google](https://pkg.go.dev/github.com/google/uuid) or [gofrs](https://pkg.go.dev/github.com/gofrs/uuid)) | a `uuid` string |
| `*big.Int`, `netip.Addr`, `net.IP`, `decimal.Decimal` | `integer` for `big.Int`; the others are strings |
| other types with `MarshalText` | a string |
| named structs | a shared schema under `components/schemas`, referenced with `$ref`. A generic one is named after its arguments: `Page[User]` is `PageUser` |

Struct fields follow `encoding/json`: `json` tag names, `-`, `,string`, and embedded structs. Fields promoted from an embedded pointer aren't required in a response, since a nil pointer leaves them out. Three more tags add detail:

| Tag | Adds |
|---|---|
| `validate:"required,min=1,max=40"` | Required fields, lengths, ranges, `email`, `uuid` and `url` formats, and `oneof` choices: the rules the validator [enforces](#the-spec-claims-the-rules-that-are-enforced). With `omitempty`, the zero value is allowed too, as the validator allows it. |
| `doc:"The pet's ID."` | A description |
| `example:"7"` | An example value |
| `enum:"s,m,l"` | The values the field takes, or its elements for a slice. Zinc checks them on input, whatever the validator |

The request body of the first example, `CreatePetBody`, becomes:

```json
"CreatePetBody": {
  "type": "object",
  "properties": {
    "name": { "type": "string", "minLength": 1, "maxLength": 40 },
    "kind": { "type": "string", "enum": ["cat", "dog"] }
  },
  "required": ["name", "kind"]
}
```

`kind` is required although it has no `required` rule: `oneof=cat dog` rejects the empty string a missing field binds as, so a request without it gets a `422`. Add `omitempty` to make it optional.

### Enums

Go can't list a type's constants at run time, so a named type lists its values with an `Enum` method. It becomes one shared schema, which every field of the type refers to:

```go
type Kind string

const (
	Cat Kind = "cat"
	Dog Kind = "dog"
)

func (Kind) Enum() []any { return []any{Cat, Dog} }
```

```json
"Kind": { "type": "string", "enum": ["cat", "dog"] }
```

### Describe a type yourself

When a type's JSON doesn't match its Go shape, such as one with its own `MarshalJSON`, describe it yourself with `zinc.SchemaProvider`:

```go
type Date struct{ time.Time }

func (Date) OpenAPISchema() map[string]any {
	return map[string]any{"type": "string", "format": "date"}
}
```

### Mark fields read-only, write-only or deprecated

The `openapi` tag adds a field's role to the schema: `readonly` for a field the server sets, such as an ID, `writeonly` for one clients send but never get back, such as a password, and `deprecated`. Separate several with commas:

```go
type Pet struct {
	ID       string `json:"id" openapi:"readonly"`
	Name     string `json:"name"`
	Password string `json:"password,omitempty" openapi:"writeonly"`
	Nick     string `json:"nick,omitempty" openapi:"deprecated"`
}
```

```json
"id": { "type": "string", "readOnly": true },
"password": { "type": "string", "writeOnly": true },
"nick": { "type": "string", "deprecated": true }
```

The tag describes the field; it doesn't filter it. Binding still fills a `readonly` field from the body, and a response still sends a `writeonly` one. Use separate input and output types for a field that must never cross.

### Name a component

A struct's component is named after its Go type. To choose another name, such as for an unexported type or a generic one, add an `OpenAPIName` method (`zinc.SchemaNamer`):

```go
type petRecord struct{ ID string `json:"id"` }

func (petRecord) OpenAPIName() string { return "PetSummary" }
```

Its input variant is then `PetSummaryInput`. If two types claim one name, the later one is listed under its package.

### Types from other packages

A type from another module can't gain a method. Give it a schema in `OpenAPIConfig.Schemas` instead:

```go
cfg := zinc.OpenAPIConfig{
	Schemas: map[reflect.Type]map[string]any{
		reflect.TypeFor[money.Amount](): {"type": "string", "format": "decimal"}, // money: another module
	},
}
```

An entry there wins over everything else, including the types Zinc knows, such as `uuid.UUID`.

## Good to know

### The spec claims the rules that are enforced

`validate` tags become schema keywords, such as `minLength` and `format: email`, only for rules something enforces: Zinc's [built-in rules](/guide/binding/#validation), or the rules your `Validator` lists with `RuleSet`. A validator without `RuleSet` makes no claims. A field whose rules reject a missing value, such as `oneof=cat dog` or `min=1` without `omitempty`, is listed as required, because a request without it gets a `422`. `422` is documented only on routes that can send it.

### Error bodies follow your error handler

With Zinc's default error handler, the `400`, `422` and `500` responses describe its error body, the `Error` schema. With `zinc.ProblemErrors`, they describe RFC 9457 problem details, the `Problem` schema, as `application/problem+json`. With your own `ErrorHandler`, Zinc can't know what you send, so those responses list the status alone.

### Slices and maps in responses can be null

`encoding/json` writes a nil slice or map as `null`, so a response schema allows `null` for them, and a generated client may type such a field as a pointer, such as `*[]string`. A field with `omitempty` or `omitzero` is left out instead of sent as `null`, so its schema doesn't allow `null`.

### Wrapping a typed handler hides its types

Register `zinc.Typed(...)` directly. A typed handler called from inside a plain one isn't recognized; add `.Input` and `.Output` to that route instead.

### Routes OpenAPI can't tell apart

Zinc's router can tell `/files/{path}` from `/files/{path...}`, and `/pets/{id}` on GET from `/pets/{name}` on POST. OpenAPI can't: the first pair is one operation, and paths that differ only in parameter names aren't allowed. Building the spec returns an error naming both routes:

```text
zinc: paths /pets/{id} and /pets/{name} differ only in parameter names, which OpenAPI doesn't allow; give the parameters the same names
```

Rename the parameters to match, or leave one route out of the spec with `.Hidden()`.

### A type named Error

With Zinc's default error handler, the spec's `Error` schema is Zinc's error body. Your own type named `Error` is listed under its package, such as `main.Error`.

### What's left out

Hidden routes, mounts and static files, and methods OpenAPI 3.1 has no field for, such as `PURGE`.

## Next steps

- [API Docs](/middleware/apidocs/): show the spec as a page with Scalar, Swagger UI, Stoplight Elements or ReDoc.
- [Typed Handlers](/guide/typed-handlers/): write handlers whose types describe themselves.
- [Binding](/guide/binding/): the tags the spec reads, and the validation rules.
