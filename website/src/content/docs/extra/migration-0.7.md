---
title: Upgrading to 0.7
description: What changed in Zinc 0.7. Zinc checks validate tags itself, and the OpenAPI spec claims only the rules something enforces.
slug: extra/migration-0.7
---

0.7 makes the spec a contract: every rule it states is a rule a request is checked against. Update the module and run your tests:

```bash
go get github.com/0mjs/zinc@v0.7.0
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

## What's new

- **[Validation](/guide/binding/#validation)** without a dependency: `required`, `min`, `max`, `len`, `gt`, `gte`, `lt`, `lte`, `oneof`, `email`, `uuid`, `url` and more, through nested structs, slices and maps, with each field named as the client sent it.
- **`Config.ValidateResponses`** checks typed handlers' outputs before they're sent, for development and tests.
- **[`app.Validate()`](/guide/openapi/#check-the-app-before-serving)** builds every spec and checks the routes without a request, so a mistake fails a test or deploy.
- **[Spec hooks](/guide/openapi/#edit-the-spec)**: `OpenAPIConfig.Extensions`, `Route.Operation` and `OpenAPIConfig.Mutate` edit the spec as JSON, and the result is checked again.
- **The request logger** passes the request's context to `slog`, so a handler that reads trace IDs from it can add them.
- **[A Production Service](/cookbook/production/)**: one program with limits, timeouts, request IDs, validation, a protected spec and graceful shutdown.

## Not in 0.7

Polymorphic types, such as a `oneOf` with a discriminator, are planned separately. They touch the schema, decoding, errors and generated clients together, and ship when all of those are ready.

## Next steps

- [Upgrading to 0.6](/extra/migration-0.6/): the changes before this release.
- [Compatibility](/extra/compatibility/): what each kind of release can change.
