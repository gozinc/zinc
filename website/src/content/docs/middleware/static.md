---
title: Static
description: Serve static files from middleware.
---

`Static` serves files from `app.Use`, and falls through to your routes when a requested file does not exist. For a dedicated asset prefix, the [app-level helpers](/guide/static-files/) are usually simpler.

```go
app.Use(middleware.Static("./public"))
```

Serve from a prefix:

```go
app.Use(middleware.StaticFrom("/assets", "./public"))
```

Serve from an `fs.FS`:

```go
app.Use(middleware.StaticFS(embeddedFiles))
```

By default, `Static` falls through to the next handler when a file is not found.

```go
app.Use(middleware.StaticWithConfig(middleware.StaticConfig{
	Root:           "./public",
	Prefix:         "/assets",
	NextOnNotFound: true,
}))
```
