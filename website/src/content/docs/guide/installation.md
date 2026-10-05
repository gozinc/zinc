---
title: Installation
description: Add Zinc to a Go module, import its packages, and upgrade between versions.
---

Zinc installs with one `go get`, like any other Go module. There's no code generator or CLI to set up. For a guided first run, start with the [Quickstart](/guide/quickstart/) instead.

## Requirements

- Go **1.25** or newer
- A Go module. Run `go mod init` if you don't have one yet.

## Install

```bash
go get github.com/0mjs/zinc
```

## Import what you use

The core package is `github.com/0mjs/zinc`. Each built-in middleware is its own small package in the same module, so the one `go get` above already installed them all:

```go
import (
	"github.com/0mjs/zinc"                      // app, router, context, binding, responses
	"github.com/0mjs/zinc/middleware/logger"    // one package per middleware
	"github.com/0mjs/zinc/middleware/requestid"
)
```

Zinc's `go.mod` requires no other modules, so adding Zinc adds nothing else to your build.

## What else you might install

Some features need a library of your choice. They plug in without changes to Zinc:

| You want | Install | See |
|---|---|---|
| YAML, TOML or another body format | The library you prefer, registered as a decoder or encoder | [Customization](/guide/customization/) |
| JWT authentication | `go get github.com/0mjs/contrib/jwtauth` | [JWT](/middleware/jwtauth/) |
| OpenTelemetry tracing | The official `otelhttp` package, wrapped around your app | [OpenTelemetry](/middleware/open-telemetry/) |
| The official Prometheus client | `promhttp.Handler()`, served with `app.HandleHTTP` | [Prometheus](/middleware/prometheus/) |

Middleware that needs a third-party library, such as JWT, lives in [`github.com/0mjs/contrib`](/middleware/overview/#contrib). Each contrib package is its own module, so you only download what you import.

## Check the installed version

```bash
go list -m github.com/0mjs/zinc
# github.com/0mjs/zinc v0.7.3
```

## Upgrade

```bash
go get github.com/0mjs/zinc@latest
go mod tidy
```

Your `go.mod` records the exact version you build with, so Zinc only changes when you run `go get`.

:::caution[Zinc is pre-1.0]
A minor release, such as 0.4 to 0.5, can change the API. Read the [release notes](https://github.com/gozinc/zinc/releases) before you upgrade, and run your tests afterwards.
:::

Each minor release has an upgrade guide:

- From 0.4: [Upgrading to 0.5](/extra/migration-0.5/). No code changes are needed.
- From 0.3: [Migrating to 0.4](/extra/migration-0.4/), then the 0.5 guide.
- From an older release: work through [0.2](/extra/migration-0.2/) and [0.3](/extra/migration-0.3/) first.

## Next steps

- [Your First Route](/guide/first-route/): handlers, input, and errors.
- [Coming from Gin or Echo](/guide/coming-from-gin-or-echo/): the Zinc version of what you already write.
