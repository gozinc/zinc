---
title: Trailing Slash
description: Add, remove, or redirect trailing slash variants.
---

Zinc already treats `/users` and `/users/` as the same route unless `StrictRouting` is on. `TrailingSlash` is for strict apps, or when clients should be sent to one canonical URL. By default it removes the trailing slash before routing.

```go
app.Use(middleware.TrailingSlash())
```

This lets `/users/` match a route registered as `/users`.

Use `AddTrailingSlash` for the opposite behavior.

```go
app.Use(middleware.AddTrailingSlash())
```

Use redirects when the client should see the canonical URL.

```go
app.Use(middleware.TrailingSlashWithConfig(middleware.TrailingSlashConfig{
	Redirect:   true,
	StatusCode: zinc.StatusPermanentRedirect,
}))
```
