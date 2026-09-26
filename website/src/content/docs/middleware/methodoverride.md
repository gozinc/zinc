---
title: Method Override
description: Override POST methods from headers or custom getters.
---

HTML forms can only send `GET` and `POST`. `methodoverride` lets a `POST` request say which method it really means, so forms can reach `PUT`, `PATCH`, and `DELETE` routes. It runs before routing.

```go
import "github.com/0mjs/zinc/middleware/methodoverride"

app.Use(methodoverride.New())
```

The default source is the `X-HTTP-Method-Override` header, and the original method is kept in `X-Original-Method`. Only the methods in `Config.Methods` can be requested, and only from those in `Config.SourceMethods`, so header input cannot widen route access.

Read the method from a form field or query parameter as well:

```go
app.Use(methodoverride.New(methodoverride.Config{
	Getter: methodoverride.FromFirst(
		methodoverride.FromHeader("X-HTTP-Method-Override"),
		methodoverride.FromQuery("_method"),
	),
}))
```
