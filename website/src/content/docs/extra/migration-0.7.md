---
title: Upgrading to 0.7
description: What changed in Zinc 0.7, 0.7.1 and 0.7.2. Zinc checks validate tags itself, the OpenAPI spec claims only the rules something enforces, 0.7.1 tidies the spec's names, and 0.7.2 fixes binding, status and redirect bugs.
slug: extra/migration-0.7
---

0.7 makes the spec a contract: every rule it states is a rule a request is checked against. Update the module and run your tests:

```bash
go get github.com/0mjs/zinc@v0.7.2
go test ./...
```

## What might need a change

### Validate tags are checked without a validator

An app without `Config.Validator` used to ignore `validate` tags. Zinc now checks them with its [built-in rules](/guide/binding/#validation), the common go-playground ones, so a request that breaks a rule gets a `422` naming the field:

```bash
# {"error":{"status":422,"message":"validation failed","fields":{"email":"is required","password":"must be at least 12 characters"}}}
```

If your structs carry `validate` tags you didn't mean to enforce, remove them. A tag with a rule Zinc doesn't have, such as `alphanum` or `dive`, makes its typed handler panic at registration:

```text
panic: zinc: Zinc's built-in validator doesn't enforce these validate rules: main.Code.Value: alphanum; use rules it declares, or a Validator that declares them with RuleSet
```

Use a rule Zinc has, or plug in go-playground/validator for that rule.

### A validator lists the rules it enforces

With `Config.Validator` set, the spec used to claim every rule in your tags. Now it claims only those your validator lists in a `RuleSet() []string` method, and a typed handler using another rule panics at registration. Without `RuleSet` your validator still runs, but the spec claims no rules. Add one to your adapter:

```go
func (playground) RuleSet() []string {
	return []string{"required", "omitempty", "min", "max", "len", "oneof", "email", "uuid", "url", "dive"}
}
```

### Enum values are checked

Values listed with an `enum` tag or an `EnumProvider` type were documented but not checked. They're checked now, with any validator: a value outside the list gets a `422`. A zero value, a field left out, still passes unless a rule requires it.

### The spec is more exact

- A field whose rules reject a missing value, such as `oneof=cat dog` or `min=1` without `omitempty` or a `default`, is listed as required. Add `omitempty` to make it optional.
- `422` is listed only on routes that can send it: input with rules or enums, or an app with its own `Validator`.

Generated clients may change with these: fields that become required lose their pointer.

### 0.7.1: component names start with a capital

A component used to keep its Go type's name as written, so an unexported `createInput` was `createInputInput` and a clash was `flags.createInput`. Now:

| Go type | 0.7.0 | 0.7.1 |
|---|---|---|
| `pet` in a response | `pet` | `Pet` |
| `createPetInput` as a request | `createPetInputInput` | `CreatePetInput` |
| a second `createPet`, in package `admin` | `admin.createPet` | `AdminCreatePet` |

A generated client's type names may change with them; most generators already capitalized, so many won't. A name you chose with `OpenAPIName` is kept as you wrote it, without a repeated suffix.

### 0.7.1: header names are spelled as you tag them

The spec used to show headers in Go's canonical form, such as `Etag` for `header:"ETag"` and `X-Csrf-Token` for `header:"X-CSRF-Token"`. It now spells them as the tag does. Header names aren't case-sensitive, so clients keep working; a test that compares the spec's text may need the new spelling.

### 0.7.2: binding fixes

0.7.2 fixes bugs. Each item below changes what a program does only where the old behavior was wrong.

- **A body can't fill a field tagged only for the path, query, headers or cookies**, whatever the key's spelling or the decoder. An escaped key such as `{"\u0072ole":"admin"}`, or a [configured decoder](/guide/customization/#body-formats) that maps its own key names, could set one.
- **A `default` tag applies to a field from any source.** A body field's default was never set, and a path field's was set only sometimes. A body that leaves the key out now gets the default; `{"count":0}` still sets 0.
- **A form field comes from the form body only.** A URL-encoded request also read the query string, so `?name=x` could fill a `form:"name"` field.
- **`Content-Type` matches whatever its case**: `Application/JSON` is JSON, where it was a `400`.
- **A struct binds its path and query when the body is `text/plain`.** All its fields used to be skipped. A non-empty text body, which can't fill a struct, is a `400`.

### 0.7.2: validation checks everything it describes

- **Recursive types are checked at every depth**: an empty `name` in a tree's grandchild is a `422` naming `child.child.name`, where only the top level was checked. A value that refers back to itself is checked once.
- **An `enum` on a slice checks each element**, naming it as `roles[0]`. It used to check nothing.
- **An `enum` value the field can't hold**, such as `enum:"lots"` on an `int`, panics at registration for a typed handler and is reported by `app.Validate()`. It used to be ignored.
- **Number limits are exact**: `max=9007199254740992` on a `uint64` rejects `9007199254740993`, which it used to let through. A limit the field can't hold, such as `min=-1` on a `uint`, fails at registration.
- **Rules run in the order you write them**, and `omitempty` skips only the rules after it, as go-playground's validator does: `required,omitempty` rejects `""`. A non-nil pointer counts as set, so a `*bool` holding `false` passes `required`.
- `uuid4` requires lower case, and `uuid4_rfc4122` checks the RFC 4122 variant.

### 0.7.2: routing

- **A route for one method never hides another method's route.** `POST /x/123/` reaches `POST /x/{id}/` beside `GET /x/{id}`, where it was a `405`.
- **A rest-of-path value keeps the path as sent**, trailing slash included: `/files/docs/` gives `docs/`, where it gave `docs`.
- **A route registered with a trailing slash**, such as `/users/{id}/`, is reached without it unless `StrictRouting` is on. So `/users/{id}` and `/users/{id}/` together now panic at startup, naming both.
- **`app.URL` returns an error** when the URL it would build reaches a different route or different values.
- **`FindRoute` finds the `GET` route for a `HEAD` request**, as the app answers it.
- **A `nil` handler or middleware, or a route with more than 256 parameters, panics at registration**, where it used to fail on the first request.

### 0.7.2: typed handlers send the status the spec documents

- A `NoContent` output on a route declared `Status(200)` sends 200, as the spec says, where it sent 204.
- A `NoContent` output keeps a status middleware set with `c.Status`, where it replaced it with 204.
- A `Redirect` output on a route declared with a status that isn't a redirect, such as `Status(201)`, panics at registration, naming the route. It used to send 302 while the spec said 201.
- When an output's body fails to encode, its header fields, such as `Set-Cookie` or `Location`, are no longer sent on the `500`.

### 0.7.2: middleware

- **`Skip` keeps each middleware's own behavior.** Wrapping a rewrite in `Skip` made every `Skip`-wrapped middleware count as a rewrite, so putting one on a group panicked. Wrapping CORS made every `Skip`-wrapped middleware, auth included, run on automatic `OPTIONS` requests.
- **A redirect built from the request path stays on this site.** Trailing Slash in redirect mode and redirect rules with a wildcard sent `GET /%2Fevil.example/` to `//evil.example`. Targets now keep the path's escaping, so a space is sent as `%20`, and a local target always starts with a single `/`. A rule whose target names a scheme or host, such as `https://example.com/*`, still redirects there.

## What's new

- **[Validation](/guide/binding/#validation)** without a dependency: `required`, `min`, `max`, `len`, `gt`, `gte`, `lt`, `lte`, `oneof`, `email`, `uuid`, `url` and more, through nested structs, slices and maps, with each field named as the client sent it.
- **`Config.ValidateResponses`** checks typed handlers' outputs before they're sent, for development and tests.
- **[`app.Validate()`](/guide/openapi/#check-the-app-before-serving)** builds every spec and checks the routes without a request, so a mistake fails a test or deploy.
- **[Spec hooks](/guide/openapi/#edit-the-spec)**: `OpenAPIConfig.Extensions`, `Route.Operation` and `OpenAPIConfig.Mutate` edit the spec as JSON, and the result is checked again.
- **The request logger** passes the request's context to `slog`, so a handler that reads trace IDs from it can add them.
- **[A Production Service](/cookbook/production/)**: one program with limits, timeouts, request IDs, validation, a protected spec and graceful shutdown.
- **0.7.1: [`pattern` tags](/guide/binding/#match-a-pattern)**: a regular expression a string must match, checked with any validator and listed in the spec.
- **0.7.1: [Describe what middleware adds](/guide/openapi/#describe-what-middleware-adds)**: `App.Document`, `Group.Document` and `Route.Document` add a middleware's credentials, headers and errors to the spec, and the CSRF, Timeout, Limiter and Body Limit middleware each have a `Doc()`.
- **0.7.1: `SchemaNamer` names enum types** as well as structs; it always could, and the docs now say so.
- **0.7.2: The Limiter middleware sends `Retry-After`** on a `429`: the whole seconds until a request would be allowed, rounded up.
- **0.7.2: [Security policy](https://github.com/gozinc/zinc/security/policy)**: how to report a vulnerability privately.

## Not in 0.7

Zinc doesn't support polymorphic types, such as a `oneOf` with a discriminator.

## Next steps

- [Upgrading to 0.6](/extra/migration-0.6/): the changes before this release.
- [Compatibility](/extra/compatibility/): what each kind of release can change.
