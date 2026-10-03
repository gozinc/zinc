---
title: Coming from Huma or Fuego
description: Huma, Fuego and Zinc all build an OpenAPI spec from Go types. How the same API looks in each, where Zinc differs, and what the others do that Zinc doesn't.
---

Huma and Fuego exist to turn Go types into an OpenAPI spec, and they do it well. Zinc does it too, since 0.6, as part of a general web framework. This page shows the same API in all three, then says plainly where they differ, including what Huma and Fuego offer that Zinc doesn't.

The Huma and Fuego code is written from each project's README and documentation, Huma v2 and Fuego v0.20, as of October 2026. If something here is out of date, [open an issue](https://github.com/0mjs/zinc/issues).

## The same endpoint three ways

Create a pet, answering `201`, with a name of 1 to 40 characters and an optional kind.

**Zinc:** the input is one struct, the output is the return type, and `validate` tags are checked by Zinc itself.

```go
type CreatePet struct {
	Name string `json:"name" validate:"required,min=1,max=40"`
	Kind string `json:"kind" validate:"omitempty,oneof=cat dog"`
}

app.Post("/pets", zinc.Typed(func(c *zinc.Context, in CreatePet) (Pet, error) {
	return pets.Create(in) // pets: your store
})).Status(http.StatusCreated)
```

**Huma:** the request body is a `Body` field of the input struct, rules are JSON Schema tags, and the route is described by a `huma.Operation`.

```go
type CreatePetInput struct {
	Body struct {
		Name string `json:"name" minLength:"1" maxLength:"40"`
		Kind string `json:"kind,omitempty" enum:"cat,dog"`
	}
}

type PetOutput struct {
	Body Pet
}

huma.Register(api, huma.Operation{
	Method:        http.MethodPost,
	Path:          "/pets",
	DefaultStatus: http.StatusCreated,
}, func(ctx context.Context, in *CreatePetInput) (*PetOutput, error) {
	pet, err := pets.Create(in.Body.Name, in.Body.Kind)
	return &PetOutput{Body: pet}, err
})
```

**Fuego:** a generic context carries the body type, and options passed at registration add what the types don't say.

```go
type CreatePet struct {
	Name string `json:"name" validate:"required,min=1,max=40"`
	Kind string `json:"kind" validate:"omitempty,oneof=cat dog"`
}

fuego.Post(s, "/pets", func(c fuego.ContextWithBody[CreatePet]) (Pet, error) {
	body, err := c.Body() // decodes and validates
	if err != nil {
		return Pet{}, err
	}
	return pets.Create(body)
}, option.DefaultStatusCode(201))
```

All three produce an OpenAPI 3.1 operation with the body schema, the `201` response and an error response.

## Side by side

| | Huma | Fuego | Zinc |
|---|---|---|---|
| What it is | An OpenAPI layer over a router you bring, through adapters: chi, the standard `ServeMux` and others | A framework on `net/http`, with adaptors to add OpenAPI to an existing Gin or Echo server | A framework and router on `net/http`; the app is an `http.Handler` |
| Register a typed route | `huma.Register(api, huma.Operation{...}, fn)` or `huma.Get(api, path, fn)` | `fuego.Post(s, path, fn, options...)` | `app.Post(path, zinc.Typed(fn))`, the same methods as any route |
| Handler | `func(context.Context, *In) (*Out, error)` | `func(fuego.ContextWithBody[B]) (Out, error)` | `func(*zinc.Context, In) (Out, error)` |
| Where inputs come from | Tags on the input struct: `path`, `query`, `header`, `cookie`; the body is a `Body` field | The body type, a params struct, and `option.Path`/`option.Query` | Tags on one struct: `path`, `query`, `header`, `cookie`, and `json` for the body |
| Rules | JSON Schema tags (`minLength`, `enum`, …), checked against the request | go-playground/validator tags | go-playground-style `validate` tags, [checked by Zinc](/guide/binding/#validation), or by your validator with a declared `RuleSet` |
| Response headers | Fields of the output struct | Set in the handler | [Fields of the output struct](/guide/typed-handlers/#send-headers-and-cookies) tagged `header` |
| Errors | RFC 9457 problem details by default | RFC 9457 problem details | Zinc's JSON envelope by default; [`zinc.ProblemErrors`](/guide/errors/#problem-details-rfc-9457) for RFC 9457 |
| Docs page | Stoplight Elements at `/docs` | Stoplight Elements at `/swagger/` | Scalar at `/docs`; Swagger UI, Elements or ReDoc with [API Docs](/middleware/apidocs/) |
| Handlers without types | Through the router you brought | `fuego.GetStd` and friends | Every route; [`.Input` and `.Output`](/guide/openapi/#describe-a-plain-handler) add their types to the spec |

## Where Zinc is different

- **One framework, not a layer.** Zinc is the router, binding, errors and middleware, so the spec sees everything the app does: groups, their security, CORS on a group, and routes with no types at all. There's no adapter between the spec and the router.
- **Typed handlers are optional.** A typed handler is an ordinary route, next to plain `func(c *zinc.Context) error` handlers in the same app, and middleware works the same for both.
- **The spec states only what's enforced.** A rule appears in the spec only when Zinc, or a validator that declares it, checks it, and a field whose rule rejects a missing value is listed as required. A rule nothing checks fails at registration. See [The spec claims the rules that are enforced](/guide/openapi/#the-spec-claims-the-rules-that-are-enforced).
- **Mistakes fail at startup.** Routes OpenAPI can't tell apart, examples of the wrong type and unknown security schemes fail when the spec is built, and [`app.Validate()`](/guide/openapi/#check-the-app-before-serving) checks the lot before the server takes traffic.
- **The spec is audited.** Zinc's OpenAPI output is checked in CI against 147 scenarios, the OpenAPI 3.1 schema, real responses, and a client generated by oapi-codegen.

## What Huma and Fuego do that Zinc doesn't

Be clear on these before you move:

- **YAML.** Huma serves `/openapi.yaml` as well as JSON. Zinc writes JSON only.
- **Polymorphism.** Zinc doesn't support `oneOf` with a discriminator.
- **More formats on the wire.** Huma negotiates CBOR as well as JSON, generates PATCH operations from your PUT, and has helpers for conditional requests. Zinc negotiates the formats you configure, with JSON and XML built in.
- **A built-in CLI.** Huma can configure a service from flags and environment variables.
- **Existing Gin or Echo apps.** Fuego's adaptors document a Gin or Echo server you already have. Zinc documents Zinc routes; it can [take over routes gradually](/cookbook/existing-net-http-service/) inside a `net/http` service, not inside Gin or Echo.
- **Rules beyond the common ones.** Huma validates requests against the JSON Schema it generates, so its rules are JSON Schema's. Zinc's built-in rules cover `required`, lengths, ranges, `oneof`, `email`, `uuid` and `url`, and a `pattern` tag takes a regular expression; for more, plug in go-playground/validator.

## Next steps

- [OpenAPI](/guide/openapi/): everything Zinc puts in the spec, and how to add to it.
- [Typed Handlers](/guide/typed-handlers/): the input struct, output types, headers and status.
- [Coming from Gin or Echo](/guide/coming-from-gin-or-echo/): the same comparison for the frameworks Zinc is benchmarked against.
