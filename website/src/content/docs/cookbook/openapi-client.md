---
title: Generate an API Client
description: Serve your API's OpenAPI spec from Zinc and generate a typed Go client for it with oapi-codegen.
---

This recipe serves an OpenAPI spec from a small Zinc API, generates a Go client from it with [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen), and calls the API through that client. Use it when another Go service, a CLI or a test suite talks to your API: the client's types come from your handlers, so they can't drift from them.

## Run it

The server:

```bash
mkdir pets-server && cd pets-server
go mod init example.com/pets-server
go get github.com/0mjs/zinc github.com/go-playground/validator/v10
```

Save the program below as `main.go` and run `go run .`. It listens on port 8080.

## The program

```go title="main.go"
package main

import (
	"log"
	"net/http"
	"sync"

	"github.com/0mjs/zinc"
	"github.com/go-playground/validator/v10"
)

type Pet struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type CreatePet struct {
	Name string `json:"name" validate:"required,max=40"`
	Kind string `json:"kind" validate:"required,oneof=cat dog"`
}

// playground runs go-playground/validator on every bound input.
type playground struct{ v *validator.Validate }

func (p playground) Validate(v any) error { return p.v.Struct(v) }

type PetID struct {
	ID int64 `path:"id"`
}

var (
	mu   sync.Mutex
	pets = map[int64]Pet{}
)

func main() {
	app := zinc.New(zinc.Config{Validator: playground{validator.New()}})

	app.Post("/pets", zinc.Typed(func(c *zinc.Context, in CreatePet) (Pet, error) {
		mu.Lock()
		defer mu.Unlock()
		p := Pet{ID: int64(len(pets) + 1), Name: in.Name, Kind: in.Kind}
		pets[p.ID] = p
		return p, nil
	})).Status(http.StatusCreated).Name("createPet")

	app.Get("/pets/{id}", zinc.Typed(func(c *zinc.Context, in PetID) (Pet, error) {
		mu.Lock()
		defer mu.Unlock()
		p, ok := pets[in.ID]
		if !ok {
			return Pet{}, zinc.NotFound("no such pet")
		}
		return p, nil
	})).Name("getPet").Errors(http.StatusNotFound)

	app.OpenAPI("/openapi.json", zinc.OpenAPIConfig{Title: "Pets", Version: "1.0.0"})

	log.Fatal(app.Listen(":8080"))
}
```

## Try it

In a second terminal, create a separate module for the client, with oapi-codegen as a [tool](https://go.dev/doc/modules/managing-dependencies#tools):

```bash
mkdir pets-client && cd pets-client
go mod init petapp
go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0
curl -s localhost:8080/openapi.json -o openapi.json
```

Tell oapi-codegen what to generate:

```yaml title="petclient/cfg.yaml"
package: petclient
generate:
  models: true
  client: true
output: petclient/petclient.gen.go
```

```bash
go tool oapi-codegen -config petclient/cfg.yaml openapi.json
go mod tidy
```

The generated types match the handlers:

```go
type CreatePet struct {
	Kind CreatePetKind `json:"kind"` // cat or dog, from oneof
	Name string        `json:"name"`
}

type Pet struct {
	Id   int64  `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}
```

Call the API through the client:

```go title="main.go" check=false
package main

import (
	"context"
	"fmt"
	"log"

	"petapp/petclient"
)

func main() {
	client, err := petclient.NewClientWithResponses("http://localhost:8080")
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	created, err := client.CreatePetWithResponse(ctx, petclient.CreatePetJSONRequestBody{Name: "Rex", Kind: petclient.Dog})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(created.StatusCode(), created.JSON201.Id, created.JSON201.Name)

	got, err := client.GetPetWithResponse(ctx, created.JSON201.Id)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(got.StatusCode(), got.JSON200.Name, got.JSON200.Kind)

	missing, err := client.GetPetWithResponse(ctx, 99)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(missing.StatusCode(), missing.JSON404.Error.Message)
}
```

```bash
go run .
# 201 1 Rex
# 200 Rex dog
# 404 no such pet
```

## How it works

- `zinc.Typed` handlers describe their input and output types, and `app.OpenAPI` serves the spec built from them at `/openapi.json`.
- `.Name("createPet")` sets the operation ID, which oapi-codegen turns into the method `CreatePetWithResponse`. Unnamed routes get a name from their method and path instead.
- `validate:"required"` with a validator makes a request field required in the spec, so the client's field is a `string`, not a `*string`. Response fields are always required unless they're `omitempty`: `encoding/json` always sends them.
- `oneof=cat dog` becomes an enum, and oapi-codegen generates constants for it, such as `petclient.Dog`.
- `.Errors(http.StatusNotFound)` documents the 404 with Zinc's error body, so `JSON404.Error.Message` is typed.

## Before production

- Generate from a file you keep in the repository rather than from a running server, and regenerate when the API changes. [Write the spec to a file](/guide/openapi/#write-the-spec-to-a-file) from a test to keep it up to date.
- Pin oapi-codegen's version, as `go get -tool` above does, so regenerating doesn't change code you didn't touch.
- For clearer validation errors in the 422 response, use the adapter from [Validate Input](/cookbook/validation/).

## See also

- [OpenAPI](/guide/openapi/): everything the spec describes, and how to add to it.
- [Typed Handlers](/guide/typed-handlers/): handlers whose types describe themselves.
- [API Docs](/middleware/apidocs/): a browsable page for the same spec.
