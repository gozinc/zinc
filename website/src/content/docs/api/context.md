---
title: Context
description: Reference for zinc.Context, grouped by task, with every request, response, and state method.
---

`*zinc.Context` is passed to every handler and middleware. It is pooled, and is valid only until the handler returns. See [Context](/guide/context/) for lifetime rules.

## Request

| Method | Returns |
|---|---|
| `Request()` | The underlying `*http.Request` |
| `Context()` | The request's `context.Context` |
| `Method()` | The HTTP method |
| `Path()` | The request path |
| `OriginalURL()` | The original request URI, before any rewrite |
| `FullPath()` | The matched route pattern, such as `/users/{id}` |
| `Route()` | The matched `RouteInfo` |
| `Scheme()` | `"http"` or `"https"`, honoring forwarded protocol headers from trusted proxies |
| `Secure()` | Whether `Scheme()` is `"https"` |
| `ContentType()` | The request media type, without parameters |
| `Header(name)` | A request header |
| `IsWebSocket()` | Whether this is a WebSocket upgrade |
| `IsPreflight()` | Whether this is a CORS preflight |

## Parameters, query, and forms

| Method | Returns |
|---|---|
| `Param(name)` | A route parameter, as a string |
| `Query(name)` | A query value, as a string |
| `QueryArray(name)` | Every value for a repeated query key |
| `QueryMap(name)` | Bracket keys, such as `filter[status]`, as a map |
| `QueryValues()` | The full `url.Values` |
| `FormValue(name)` | A form value from the body or the query, like `http.Request.FormValue` |
| `FormFile(name)`, `FormFiles(name)` | Uploaded files |
| `MultipartForm()` | The parsed multipart form |
| `SaveFile(file, dst)` | Saves an uploaded file to disk |
| `Cookie(name)`, `Cookies()` | Request cookies |
| `BodyBytes()`, `BodyString()` | The raw body, cached for later reads |

Typed versions are package functions, because Go methods can't be generic:

| Function | Returns |
|---|---|
| `zinc.Param[T](c, name)` | `(T, error)`: a route parameter parsed as `T` |
| `zinc.Query[T](c, name)` | `(T, error)`: a query value; missing counts as an error |
| `zinc.QueryOr(c, name, fallback)` | `T`: a query value, or `fallback` when missing or unparsable |
| `zinc.Form[T](c, name)`, `zinc.FormOr(c, name, fallback)` | The same for form values |

`T` may be a string, bool, integer, or float type, an `encoding.TextUnmarshaler`, a named type over one of those, or a pointer to any of them. Errors are `*zinc.BindError` and answer 400 with the value's name.

## Binding and validation

| Method | Purpose |
|---|---|
| `Bind()` | The binder: `All`, `Path`, `Query`, `Header`, `Form`, `Body`, `JSON`, `XML`, `Text` |
| `Validate(v)` | Runs the configured `Validator` directly |
| `BodyLimit()` | The application's request-body budget, for middleware that transforms bodies |

See [Binding](/guide/binding/).

## Response headers and status

These return the context, so they chain into a body method.

| Method | Sets |
|---|---|
| `Status(code)` | The status code |
| `SetHeader(key, value)`, `AppendHeader(key, values...)` | Response headers |
| `Type(ext)` | `Content-Type` from a file extension, such as `"json"` |
| `Location(url)` | The `Location` header |
| `Vary(fields...)` | The `Vary` header |

`SetCookie(cookie)` and `ClearCookie(cookie)` write cookies. `Config.CookieSameSite` sets a default `SameSite` mode.

## Response bodies

| Method | Sends |
|---|---|
| `JSON(v)`, `JSONPretty(v, indent)` | JSON |
| `XML(v)` | XML |
| `Encode(mediaType, v)` | Any format with a configured encoder, or JSON and XML |
| `String(s)`, `HTML(s)` | Text or HTML |
| `Send(v)` | A string as text, `[]byte` as `application/octet-stream`, anything else as JSON |
| `Data(contentType, b)` | Bytes with a content type, such as `zinc.MIMEJSON` for pre-encoded JSON |
| `NoContent()` | `204 No Content` |
| `Render(name, data)` | A template, through the configured `Renderer` |
| `File(path)`, `FileFS(name, fsys)` | A file |
| `Attachment(path, name...)` | A file as a download |
| `Inline(path, name...)` | A file for display in the browser |
| `Stream(contentType, reader)` | Data copied from a reader |
| `SSE(event)` | Writes and flushes one server-sent event, a `zinc.Event{Event, ID, Retry, Data}`; `WriteTimeout` applies per event |
| `Redirect(url)` | A redirect: 302, or the 3xx status set by `Status` |
| `Accepts(types...)` | The best match for the `Accept` header |
| `Negotiate(offers)` | The offer that best matches `Accept` |

## Middleware and errors

| Method | Purpose |
|---|---|
| `Next()` | Runs the rest of the chain |
| `HandleError(err)` | Sends an error to the error handler immediately, for middleware that observes the final response |
| `LastError()` | The most recent error passed to `HandleError` |

## Request-local values

| Method | Returns |
|---|---|
| `Set(key, value)` | Stores a value for this request |
| `Get(key)` | `(any, bool)` |
| `zinc.Value[T](c, key)` | `(T, bool)`: the value if it has type `T` |
| `zinc.MustValue[T](c, key)` | `T`, or panics when missing or of another type |

## Client address

| Method | Returns |
|---|---|
| `IP()` | The client address, honoring trusted proxies |
| `IPs()` | The forwarded address chain from a trusted proxy |
| `RemoteIP()` | The direct peer address, ignoring headers |

See [Client IP and Proxies](/guide/ip-address/).

## Replacing request parts

For middleware that rewrites or wraps a request: `SetRequest(r)`, `SetContext(ctx)`, `SetPath(path)`, and `SetWriter(w)`. Middleware that wraps the writer should restore the original afterwards, as shown in [Response Writer](/api/response-writer/).
