---
title: Recover
description: Recover panics and route them through Zinc's error flow.
---

`recover` turns a panic in any later middleware or handler into a `500 Internal Server Error` response, instead of a dropped connection. Register it near the start of the chain, after logging, so the logger records the `500`.

```go
import "github.com/0mjs/zinc/middleware/recover"

app.Use(recover.New())
```

The recovered panic goes through your error handler like any other error.

:::note[The package name shadows the builtin]
Inside a file that imports this package, `recover` refers to the package, not Go's builtin. If that file also calls the builtin, import the package under another name, such as `recovermw "github.com/0mjs/zinc/middleware/recover"`.
:::

## Config

| Field | Default | Meaning |
|---|---|---|
| `Handler` | returns a 500 error | Turns the recovered `*recover.Error` into a response |
| `StackSize` | 4 KiB | Largest stack trace captured |
| `DisableStack` | `false` | Skips stack capture |

```go
app.Use(recover.New(recover.Config{
	Handler: func(c *zinc.Context, err *recover.Error) error {
		log.Printf("panic: %v\n%s", err.Value, err.Stack)
		return zinc.InternalServerError("internal server error")
	},
}))
```
