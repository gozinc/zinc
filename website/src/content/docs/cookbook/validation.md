---
title: Validate Input
description: Check request bodies with go-playground/validator rules and send a 422 that names every failing field.
---

This program checks a sign-up request against rules written on the struct, such as `required`, `email` and `min=12`. When a rule fails, the client gets a `422` listing each bad field by its JSON name. Use it once your handlers start filling up with `if in.Name == ""` checks. It plugs [go-playground/validator](https://github.com/go-playground/validator) into `zinc.Config.Validator`.

## Run it

```bash
mkdir zinc-validation && cd zinc-validation
go mod init example.com/zinc-validation
go get github.com/0mjs/zinc github.com/go-playground/validator/v10
```

Save the program as `main.go` and run `go run .`. It listens on port 8080.

## The program

```go title="main.go"
package main

import (
	"errors"
	"log"
	"reflect"
	"strings"

	"github.com/0mjs/zinc"
	"github.com/go-playground/validator/v10"
)

type SignUp struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=12"`
	Age      int    `json:"age" validate:"omitempty,gte=13"`
}

// fieldErrors maps each failing field to a short reason. Its Fields method
// is what puts "fields" into Zinc's 422 response.
type fieldErrors map[string]string

func (f fieldErrors) Error() string             { return "validation failed" }
func (f fieldErrors) Fields() map[string]string { return f }

// structValidator adapts go-playground/validator to zinc.Validator.
type structValidator struct{ v *validator.Validate }

func newValidator() structValidator {
	v := validator.New(validator.WithRequiredStructEnabled())
	// Name fields by their json tag, so errors say "email", not "Email".
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			return ""
		}
		return name
	})
	return structValidator{v: v}
}

func (s structValidator) Validate(target any) error {
	err := s.v.Struct(target)
	var invalid validator.ValidationErrors
	if !errors.As(err, &invalid) {
		return err
	}
	fields := fieldErrors{}
	for _, fe := range invalid {
		fields[fe.Field()] = message(fe)
	}
	return fields
}

// message turns a validator rule into a sentence for the client.
func message(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be an email address"
	case "min":
		return "must be at least " + fe.Param() + " characters"
	case "gte":
		return "must be at least " + fe.Param()
	default:
		return "failed " + fe.Tag()
	}
}

func main() {
	app := zinc.New(zinc.Config{Validator: newValidator()})

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

A body that isn't JSON fails earlier, with a `400`, and the rules never run:

```bash
curl localhost:8080/signup -H 'Content-Type: application/json' -d '{"email":'
# {"error":{"status":400,"message":"invalid request body"}}
```

## How it works

- `zinc.Config{Validator: newValidator()}` sets the validator for the whole app. Every bind method, and every [typed handler](/guide/typed-handlers/), runs it after reading the request.
- `structValidator.Validate` is the one method Zinc needs. It runs the struct's `validate` rules and turns each failure into an entry in `fieldErrors`.
- `fieldErrors` has a `Fields() map[string]string` method, so Zinc answers `422` and puts the map under `"fields"`. Without it, you still get a `422`, but only `"validation failed"`.
- `RegisterTagNameFunc` makes `fe.Field()` return the JSON name (`email`) instead of the Go name (`Email`), so the keys match what the client sent.
- `omitempty,gte=13` on `Age` checks the value only when the client sends one.

## Before production

- Keep `message` short and free of the submitted value, so errors never echo input such as a password back to the client.
- For a check that needs your database, such as "email already registered", do it in the handler and return `zinc.NewError(zinc.StatusConflict, "email already registered")`.
- Create the validator once, as here: it caches what it learns about each struct type.

## See also

- [Binding](/guide/binding/#validation): the `Validator` interface and how bind errors map to status codes.
- [Errors](/guide/errors/): the error JSON format and how to change it.
- [Typed CRUD API](/cookbook/typed-crud/): typed handlers, which run the same validator before your function.
