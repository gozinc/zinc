# Working on Zinc

Notes for AI coding assistants, and anyone else, changing this repository. To *use* Zinc in an app, read the docs, or https://zinc.carbonsoft.sh/llms.txt, instead. [CONTRIBUTING.md](CONTRIBUTING.md) has the human version of this page.

## Layout

- The core is the root package, `github.com/0mjs/zinc`. It has no dependencies outside the standard library; keep it that way.
- `middleware/<name>`: one dependency-free middleware per package, named with one word (`requestid`, never `request-id`).
- Middleware that needs a third-party library lives in a separate repository, `github.com/gozinc/contrib`.
- `openapitest/`: a test-only module that checks the generated spec against the official OpenAPI 3.1 schema and a generated client.
- `benchmarks/`: a separate module with the benchmark suite against Gin, Echo, Chi and BunRouter, and `ROUTER_SPEC.md`, the router's matching rules.
- `website/`: the docs site (Astro and Starlight). Pages are in `website/src/content/docs`.

Some files to know: `router*.go` is the router; `bind*.go` and `validate.go` bind and validate requests; `typed.go` is typed handlers; `openapi*.go` builds the spec.

## Checks

Run what CI runs before calling a change done:

```sh
gofmt -l .                     # no output
go vet ./...
go test -race ./...
(cd openapitest && go test ./...)
(cd benchmarks && go test ./...)
go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 -tests=false ./...
GOTOOLCHAIN=go1.25.14 go test ./...   # the oldest supported Go
```

Staticcheck runs twice because `-tests=false` finds code only tests use. For docs changes, run `npm run check:docs` and `npm run check:examples` in `website/`.

## Rules

- **A bug fix comes with a test that fails before the fix.** State the rule the test checks ("a route for one method never hides another's"), not just today's output.
- **Reject impossible declarations at registration**, with a message naming the route, field or tag, rather than failing on the first request.
- **The OpenAPI spec claims only what Zinc enforces.** If a tag reaches the spec, the server checks it; if it can't, leave it out of the spec.
- **No per-request cost without a measurement.** Router, binding and response changes need an A/B against the previous release (`go run ./cmd/zincbench ab` in `benchmarks/`), on a quiet machine.
- **Behavior changes go in the upgrade notes** (`website/src/content/docs/extra/migration-*.md`). Patch releases change behavior only where the old behavior was a bug; see `extra/compatibility.md`.
- **Docs state only what exists.** No "coming soon", "planned" or "not yet".
- **Comments explain why, briefly.** Don't narrate a change's history in comments.
