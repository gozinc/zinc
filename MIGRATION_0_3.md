# 0.3 hardening notes

## Groups, prefix middleware, and static files

Group `Mount`, `Static`, `StaticFS`, `File`, and `FileFS` now inherit the group's middleware, including parent groups. The chain is captured at registration, just as it is for ordinary routes.

Global middleware runs before prefix middleware. Prefix scopes are selected from the path after global rewrites. If a prefix middleware rewrites into another scope, that scope also runs before dispatch; each registered prefix chain runs at most once. Use route groups for authorization of a specific set of handlers. Middleware that directly serves a response still short-circuits downstream middleware.

Static serving consumes the already-decoded URL path once. Paths containing backslashes, dot segments, or repeated separators are rejected instead of being silently normalized. Literal percent-encoded-looking filenames are supported without a second unescape. Operating-system static roots now contain symlinks; use `StaticFS` only with a filesystem whose access policy you intend to expose.

Trailing slashes in grouped routes are preserved when strict routing is enabled.

## Request-body budgets

`Config.BodyLimit` now applies to URL-encoded forms and multipart binding/file helpers as well as JSON, XML, YAML, TOML, and raw body helpers. Exactly the configured number of bytes is accepted; larger bodies return an identifiable 413 error. Form-value helpers whose signatures have no error return yield empty values on parse/limit errors; use binding or multipart helpers when the application needs to handle an error explicitly.

Content-Length no longer permits allocating buffers above the configured budget. Multipart temporary files belong to the request and are removed when its Context is released; save uploads during the handler.

`Decompress()` now limits expansion using the application's body budget, falling back to 4 MiB when no positive budget exists. A positive `MaxDecompressedSize` overrides that default. `Context.BodyLimit()` exposes the application budget for middleware implementing body transformations.
