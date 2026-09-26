---
title: Casbin Auth
description: Authorize requests with a Casbin-compatible enforcer.
---

`casbin` checks each request against a [Casbin](https://casbin.org) policy and answers `403` when it is denied. It accepts any enforcer with this method, so Zinc does not depend on Casbin itself:

```go
Enforce(args ...any) (bool, error)
```

```go
import "github.com/0mjs/zinc/middleware/casbin"

app.Use(casbin.New(casbin.Config{
	Enforcer: enforcer,
	Subject:  casbin.SubjectFromBasicAuth(),
}))
```

`Enforcer` and `Subject` are required. By default Zinc enforces:

```go
enforcer.Enforce(subject, c.Path(), c.Method())
```

Choose a different subject, object, or action with the other fields:

```go
app.Use(casbin.New(casbin.Config{
	Enforcer: enforcer,
	Subject:  casbin.SubjectFromContext(userKey),
	Object:   casbin.ObjectPath(),
	Action:   casbin.ActionMethod(),
}))
```

Subjects can also come from [Key Auth](/middleware/keyauth/) with `casbin.SubjectFromKeyAuth()`.
