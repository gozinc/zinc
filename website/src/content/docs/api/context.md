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
| `GetHeader(name)` | A request header |
| `IsWebSocket()` | Whether this is a WebSocket upgrade |
| `IsPreflight()` | Whether this is a CORS preflight |
| `RequestID()` | The `X-Request-ID` request header |

## Parameters, query, and forms

| Method | Returns |
|---|---|
| `Param(name)`, `ParamOr(name, fallback)` | A route parameter |
| `Query(name)`, `QueryOr(name, fallback)` | A query value |
| `QueryArray(name)` | Every value for a repeated query key |
| `QueryMap(name)` | Bracket keys, such as `filter[status]`, as a map |
| `QueryValues()` | The full `url.Values` |
| `PostForm(name)`, `PostFormOr`, `PostFormArray`, `PostFormMap` | Body form values |
| `FormValue(name)` | A form value from the body or the query, like `http.Request.FormValue` |
| `FormFile(name)`, `FormFiles(name)` | Uploaded files |
| `MultipartForm()` | The parsed multipart form |
| `SaveFile(file, dst)` | Saves an uploaded file to disk |
| `Cookie(name)`, `Cookies()` | Request cookies |
| `BodyBytes()`, `BodyString()` | The raw body, cached for later reads |

## Binding and validation

| Method | Purpose |
|---|---|
| `Bind()` | The binder: `All`, `Path`, `Query`, `Header`, `Form`, `Body`, `JSON`, `XML`, `YAML`, `TOML`, `Text` |
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
| `SetSameSite(mode)` | The default `SameSite` for cookies set afterwards |

`SetCookie(cookie)` and `ClearCookie(names...)` write cookies.

## Response bodies

| Method | Sends |
|---|---|
| `JSON(v)`, `JSONPretty(v, indent)` | JSON |
| `XML(v)`, `YAML(v)`, `TOML(v)` | Other structured formats |
| `String(s)`, `HTML(s)` | Text or HTML |
| `Send(v)` | A string as text, `[]byte` as `application/octet-stream`, anything else as JSON |
| `Data(contentType, b)` | Bytes with a content type |
| `Blob(status, contentType, b)`, `JSONBlob`, `XMLBlob`, `HTMLBlob` | Pre-encoded bytes with a status |
| `NoContent()` | `204 No Content` |
| `Render(name, data)` | A template, through the configured `Renderer` |
| `File(path)`, `FileFS(name, fsys)` | A file |
| `Attachment(path, name...)`, `Download(path, name...)` | A file as a download |
| `Inline(path, name...)` | A file for display in the browser |
| `Stream(contentType, reader)` | Data copied from a reader |
| `SSE(event)` | One server-sent event |
| `Redirect(code, url)` | A redirect |
| `Accepts(types...)` | The best match for the `Accept` header |
| `Negotiate(status, offers)` | The offer that best matches `Accept` |

## Middleware and errors

| Method | Purpose |
|---|---|
| `Next()` | Runs the rest of the chain |
| `AbortWithStatus(code)` | Returns an HTTP error with that status |
| `AbortWithJSON(code, v)` | Writes JSON with that status |
| `Error(err)` | Sends an error to the error handler immediately |
| `LastError()` | The most recent error passed to the error handler |
| `Fail(err)` | Returns `err` unchanged |

## Request-local values

| Method | Returns |
|---|---|
| `Set(key, value)` | Stores a value for this request |
| `Get(key)` | `(any, bool)` |
| `MustGet(key)` | The value, or panics |
| `GetString`, `GetBool`, `GetInt`, `GetInt64`, `GetFloat64` | A typed value, or its zero value |
| `GetStringSlice`, `GetStringMap`, `GetStringMapString`, `GetStringMapStringSlice` | A typed collection, or `nil` |

## Client address

| Method | Returns |
|---|---|
| `IP()` | The client address, honoring trusted proxies |
| `IPs()` | The forwarded address chain from a trusted proxy |
| `RemoteIP()` | The direct peer address, ignoring headers |

See [Client IP and Proxies](/guide/ip-address/).

## Replacing request parts

For middleware that rewrites or wraps a request: `SetRequest(r)`, `SetContext(ctx)`, `SetPath(path)`, and `SetWriter(w)`. Middleware that wraps the writer should restore the original afterwards, as shown in [Response Writer](/api/response-writer/).
