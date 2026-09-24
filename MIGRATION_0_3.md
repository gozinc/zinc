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

## Responses and returned errors

`Context.Writer()` is now an instrumented writer, including after `SetWriter`. Use its `Unwrap() http.ResponseWriter` method or `http.ResponseController` to reach the underlying writer; pointer equality with the server's original writer is not guaranteed. Supported Flusher, Hijacker, and Pusher capabilities are preserved without advertising capabilities absent from the transport.

Selecting a status or preparing an encoder does not commit a response. Actual writes, final headers, flushes, or a successful hijack do. Errors before commitment can produce an error response; errors after partial output do not append another body. The configured error handler runs at most once per request, including when middleware calls `Context.Error` and propagates the same error.

Informational headers do not consume the final status. Gzip flushes below MinLength send buffered bytes as identity and retain that representation for subsequent writes. `Vary: Accept-Encoding` also covers identity responses. Recovery preserves `http.ErrAbortHandler`.

Response header value slices belong to each response. This intentionally adds an allocation on common helpers that previously used mutable shared storage. Context release clears request-owned values and references immediately, including after panics.

Accept negotiation now retains explicit `q=0` exclusions and applies them ahead of matching wildcards.

## Signed cookie sessions

Session responses are no longer buffered. `Session.Set` and `Session.Delete` now return errors and persist cookie headers immediately; call them before writing or flushing a response. Late mutations return `ErrSessionCommitted`, and cookies above 4096 bytes return `ErrSessionTooLarge`. Failed mutations leave the previous values intact. Check these errors before sending a successful response.

Signing keys must contain at least 32 random bytes. Cookies use a versioned payload with authenticated server-side expiry. A positive MaxAge controls that lifetime; browser-session cookies (MaxAge zero) use Lifetime, defaulting to 24 hours. Removing a cookie from the browser is not server-side revocation.

**Existing session cookies are invalidated by this format change.** Plan for users to sign in again. PreviousSecrets permits a deliberate key-rotation window for the new format; cookies verified with a previous key are signed again with the current key without extending their authenticated expiry. Remove previous keys when that window ends.

## Bounded middleware state

Keyed rate limiters default to 10,000 buckets and 256-byte keys. New keys are denied when storage is full; idle, fully replenished buckets expire after five minutes. Configure `MaxKeys`, `MaxKeyBytes`, and `IdleTTL` for your traffic. Zero rate/capacity now use defaults, and the default rejection handler honors `StatusCode`.

Prometheus defaults are isolated per middleware construction. Use the same explicit registry for collection and scraping, or place the no-argument scrape handler behind its middleware. Unmatched routes use `unmatched`, unknown methods use `OTHER`, and registries cap series at 10,000 by default. Overflow is visible in `zinc_http_metrics_dropped_total`.

Credentialed CORS requires explicit origins. An empty origin list with credentials denies access; combining `*` with credentials now panics at construction instead of reflecting arbitrary origins.

## Proxy trust and retries

Trusted proxy addresses/CIDRs are compiled and copied at construction; invalid entries panic. Forwarded IP chains are evaluated right to left until the first untrusted hop, so `IP()` no longer accepts an attacker-controlled leftmost value. `Scheme()` uses only the final trusted `http`/`https` value, rather than arbitrary protocol strings or a client-supplied prefix.

Reverse proxy `Director` runs after inbound forwarding and hop-by-hop header removal. Fresh forwarding values describe the direct request. Retries default to idempotent methods with replayable bodies; a custom `RetryFilter` is an explicit override of method policy.
