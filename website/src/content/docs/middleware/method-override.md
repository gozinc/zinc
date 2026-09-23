---
title: Method Override
description: Override POST methods from headers or custom getters.
---

HTML forms can only send `GET` and `POST`. `MethodOverride` lets a `POST` request say which method it really means, so forms can reach `PUT`, `PATCH`, and `DELETE` routes. It runs before routing.

```go
app.Use(middleware.MethodOverride())
```

The default source is:

```text
X-HTTP-Method-Override
```

The original method is preserved in:

```text
X-Original-Method
```

Use query-based or custom lookup when needed.

```go
app.Use(middleware.MethodOverrideWithConfig(middleware.MethodOverrideConfig{
	Getter: middleware.MethodOverrideFromFirst(
		middleware.MethodOverrideFromHeader("X-HTTP-Method-Override"),
		middleware.MethodOverrideFromQuery("_method"),
	),
}))
```
