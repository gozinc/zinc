---
title: Method Override
description: Let HTML forms reach PUT, PATCH and DELETE routes by sending a POST.
---

HTML forms can only send `GET` and `POST`. Method Override lets a `POST` say which method it means, so a form can reach your `PUT`, `PATCH` and `DELETE` routes. API clients that can send any method don't need it.

## Usage

```go
import "github.com/0mjs/zinc/middleware/methodoverride"

app.Use(methodoverride.New())

app.Delete("/posts/{id}", func(c *zinc.Context) error {
	return c.JSON(zinc.Map{
		"deleted":  c.Param("id"),
		"method":   c.Method(),
		"original": c.Header("X-Original-Method"),
	})
})
```

```bash
curl -X POST http://localhost:8080/posts/7 -H "X-HTTP-Method-Override: DELETE"
# {"deleted":"7","method":"DELETE","original":"POST"}
```

The method is changed before routing, so the `DELETE` route runs. The original method is kept in the `X-Original-Method` request header.

Register it with `app.Use`, so it runs before routing picks a route.

## Defaults

| Setting | Default |
|---|---|
| Where the method is read | `X-HTTP-Method-Override` request header |
| Methods that can be overridden | `POST` |
| Methods that can be requested | `PUT`, `PATCH`, `DELETE` |

## Configuration

Read the method from the header, or from a `_method` query parameter when the header is missing:

```go
app.Use(methodoverride.New(methodoverride.Config{
	Getter: methodoverride.FromFirst(
		methodoverride.FromHeader("X-HTTP-Method-Override"),
		methodoverride.FromQuery("_method"),
	),
}))
```

```bash
curl -X POST "http://localhost:8080/posts/7?_method=DELETE"
# {"deleted":"7","method":"DELETE","original":"POST"}
```

| Field | Default | Meaning |
|---|---|---|
| `Getter` | `FromHeader("X-HTTP-Method-Override")` | Reads the requested method from the request. Case and spaces are ignored. |
| `SourceMethods` | `POST` | Methods that may be overridden. Other requests pass through untouched. |
| `Methods` | `PUT`, `PATCH`, `DELETE` | Methods a request may ask for. Anything else is refused with `405`. |

The package has three getters: `FromHeader(name)`, `FromQuery(name)`, and `FromFirst(getters...)`, which returns the first non-empty result. A `Getter` is any `func(*zinc.Context) string`.

## Read the method from a form field

Forms usually send the method as a hidden field. There's no built-in getter for that, so write one with `c.FormValue`:

```go
app.Use(methodoverride.New(methodoverride.Config{
	Getter: func(c *zinc.Context) string {
		return c.FormValue("_method")
	},
}))
```

```html
<form method="post" action="/posts/7">
  <input type="hidden" name="_method" value="DELETE">
  <button>Delete</button>
</form>
```

```bash
curl -X POST http://localhost:8080/posts/7 -d "_method=DELETE"
# {"deleted":"7","method":"DELETE","original":"POST"}
```

Your handler can still read the other form fields afterwards.

## Errors

A request for a method outside `Methods` gets `405 Method Not Allowed`, and no route runs:

```bash
curl -i -X POST http://localhost:8080/posts/7 -H "X-HTTP-Method-Override: CONNECT"
# HTTP/1.1 405 Method Not Allowed
#
# {"error":{"status":405,"message":"method override not allowed"}}
```

`methodoverride.New` panics at startup when it's given more than one `Config`.

## Security

Only requests whose method is in `SourceMethods` are changed, and only to a method in `Methods`. A `GET` carrying the header stays a `GET`, so a link or an image tag can't trigger a `DELETE`.

A form that deletes or changes data still needs protection from forged submissions. Add [CSRF](/middleware/csrf/) to cookie-authenticated forms.

## Related

- [CSRF](/middleware/csrf/): protects form submissions from other sites.
- [Templates](/guide/templates/): rendering the HTML forms.
- [Routing](/guide/routing/): registering `PUT`, `PATCH` and `DELETE` routes.
