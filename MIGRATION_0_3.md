# 0.3 hardening notes

## Groups, prefix middleware, and static files

Group `Mount`, `Static`, `StaticFS`, `File`, and `FileFS` now inherit the group's middleware, including parent groups. The chain is captured at registration, just as it is for ordinary routes.

Global middleware runs before prefix middleware. Prefix scopes are selected from the path after global rewrites. If a prefix middleware rewrites into another scope, that scope also runs before dispatch; each registered prefix chain runs at most once. Use route groups for authorization of a specific set of handlers. Middleware that directly serves a response still short-circuits downstream middleware.

Static serving consumes the already-decoded URL path once. Paths containing backslashes, dot segments, or repeated separators are rejected instead of being silently normalized. Literal percent-encoded-looking filenames are supported without a second unescape. Operating-system static roots now contain symlinks; use `StaticFS` only with a filesystem whose access policy you intend to expose.

Trailing slashes in grouped routes are preserved when strict routing is enabled.
