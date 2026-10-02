---
title: Validate Input
description: Check request bodies with rules written on the struct, and send a 422 that names every failing field. Then swap in go-playground/validator for more rules.
---

This program checks a sign-up request against rules written on the struct, such as `required`, `email` and `min=12`. When a rule fails, the client gets a `422` listing each bad field by its JSON name. Use it once your handlers start filling up with `if in.Name == ""` checks. Zinc checks these rules itself, so there's nothing to install.

## Run it

```bash
mkdir zinc-validation && cd zinc-validation
go mod init example.com/zinc-validation
go get github.com/0mjs/zinc
```

Save the program as `main.go` and run `go run .`. It listens on port 8080.

## The program

```go title="main.go"
package main

import (
	"log"

	"github.com/0mjs/zinc"
)

type Address struct {
	Country string `json:"country" validate:"required,len=2"`
}

type SignUp struct {
	Email     string    `json:"email" validate:"required,email"`
	Password  string    `json:"password" validate:"required,min=12"`
	Age       int       `json:"age" validate:"omitempty,gte=13"`
	Plan      string    `json:"plan" validate:"omitempty,oneof=free pro"`
	Addresses []Address `json:"addresses" validate:"max=3"`
}

func main() {
	app := zinc.New()

	app.Post("/signup", func(c *zinc.Context) error {
		var in SignUp
		if err := c.Bind().JSON(&in); err != nil {
			return err // 400 for bad JSON, 422 for failed rules
		}
		return c.Status(zinc.StatusCreated).JSON(zinc.Map{"email": in.Email})
	})

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

A valid request passes:

```bash
curl -i localhost:8080/signup -H 'Content-Type: application/json' \
  -d '{"email":"ada@example.com","password":"correct-horse-battery"}'
# HTTP/1.1 201 Created
# Content-Type: application/json; charset=utf-8
#
# {"email":"ada@example.com"}
```

Break three rules and all three come back together:

```bash
curl -i localhost:8080/signup -H 'Content-Type: application/json' \
  -d '{"email":"ada","password":"short","age":9}'
# HTTP/1.1 422 Unprocessable Entity
# Content-Type: application/json; charset=utf-8
#
# {"error":{"status":422,"message":"validation failed","fields":{"age":"must be at least 13","email":"must be an email address","password":"must be at least 12 characters"}}}

curl localhost:8080/signup -H 'Content-Type: application/json' -d '{}'
# {"error":{"status":422,"message":"validation failed","fields":{"email":"is required","password":"is required"}}}
```

Nested structs are checked too, and a failure names its place:

```bash
curl localhost:8080/signup -H 'Content-Type: application/json' \
  -d '{"email":"ada@example.com","password":"correct-horse-battery","addresses":[{"country":"GB"},{"country":"France"}]}'
# {"error":{"status":422,"message":"validation failed","fields":{"addresses[1].country":"must be exactly 2 characters"}}}
```

A body that isn't JSON fails earlier, with a `400`, and the rules never run:

```bash
curl localhost:8080/signup -H 'Content-Type: application/json' -d '{"email":'
# {"error":{"status":400,"message":"invalid request body"}}
```

## How it works

- Every bind method, and every [typed handler](/guide/typed-handlers/), checks the struct's `validate` tags after reading the request. The [rules](/guide/binding/#validation) mean what they mean to go-playground/validator.
- `omitempty,gte=13` on `Age` checks the value only when the client sends one.
- Fields are named as the client sent them: the `json` name, or the `query`, `header` or `path` name.
- The [OpenAPI](/guide/openapi/) spec lists the same rules, such as `minLength: 12` and `format: email`, because they're the ones enforced.

## Use go-playground/validator instead

For rules Zinc doesn't have, such as `alphanum` or `dive`, plug in [go-playground/validator](https://github.com/go-playground/validator). It replaces Zinc's rules. `RuleSet` lists the rules it enforces, so the spec can claim them:

```go
import (
	"errors"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// fieldErrors maps each failing field to a short reason. Its Fields method
// is what puts "fields" into Zinc's 422 response.
type fieldErrors map[string]string

func (f fieldErrors) Error() string             { return "validation failed" }
func (f fieldErrors) Fields() map[string]string { return f }

type playground struct{ v *validator.Validate }

func newValidator() playground {
	v := validator.New(validator.WithRequiredStructEnabled())
	// Name fields by their json tag, so errors say "email", not "Email".
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			return ""
		}
		return name
	})
	return playground{v: v}
}

func (p playground) Validate(target any) error {
	err := p.v.Struct(target)
	var invalid validator.ValidationErrors
	if !errors.As(err, &invalid) {
		return err
	}
	fields := fieldErrors{}
	for _, fe := range invalid {
		fields[fe.Field()] = "failed " + fe.Tag()
	}
	return fields
}

// RuleSet lists the rules your structs use that go-playground enforces.
func (playground) RuleSet() []string {
	return []string{"required", "omitempty", "min", "max", "len", "gte", "oneof", "email", "dive", "alphanum"}
}
```

```go
app := zinc.New(zinc.Config{Validator: newValidator()})
```

A typed handler whose struct uses a rule `RuleSet` doesn't list fails to register, so a rule can't be silently ignored. Leave `RuleSet` out and the validator still runs, but the spec claims no rules.

## Before production

- Keep messages short and free of the submitted value, so errors never echo input such as a password back to the client.
