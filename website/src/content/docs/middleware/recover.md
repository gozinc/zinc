---
title: Recover
description: Recover panics and route them through Zinc's error flow.
---

`Recover` turns a panic in any later middleware or handler into a `500 Internal Server Error` response, instead of a dropped connection. Register it near the start of the chain, after logging, so the logger records the `500`.

```go
app.Use(middleware.Recover())
```

The recovered panic goes through your error handler like any other error.

Use `RecoverWithConfig` to customize the response or log the stack.

```go
app.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
	Handler: func(c *zinc.Context, err *middleware.RecoverError) error {
		log.Printf("panic: %v\n%s", err.Value, err.Stack)
		return c.Status(zinc.StatusInternalServerError).JSON(zinc.Map{
			"error": "internal server error",
		})
	},
}))
```

Set `DisableStack` when stack capture is not needed.
