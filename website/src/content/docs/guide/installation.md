---
title: Installation
description: Add Zinc to a Go module, import its packages, and manage versions.
---

Zinc is an ordinary Go module with no code generation or CLI. If you want a guided first run instead, start with the [Quickstart](/guide/quickstart/).

## Requirements

- Go **1.25** or newer
- A Go module, created with `go mod init` if you do not have one yet

## Install

```bash
go get github.com/0mjs/zinc
```

## Import

The core package is `github.com/0mjs/zinc`. Each first-party middleware is its own small package in the same module, so one `go get` installs them all:

```go
import (
	"github.com/0mjs/zinc"                      // app, router, context, binding, responses
	"github.com/0mjs/zinc/middleware/logger"    // one package per middleware
	"github.com/0mjs/zinc/middleware/requestid"
)
```

The core package depends only on the standard library plus encoders for YAML and TOML, and the middleware packages add nothing. Middleware with third-party dependencies, such as [JWT](/middleware/jwtauth/), lives in [`github.com/0mjs/contrib`](/middleware/overview/#contrib) and is installed separately. Integrations with larger dependencies, such as OpenTelemetry or the official Prometheus client, plug in through `net/http`.

## Check the installed version

```bash
go list -m github.com/0mjs/zinc
```

## Upgrade

```bash
go get github.com/0mjs/zinc@latest
go mod tidy
```

:::caution[Zinc is pre-1.0]
Minor releases can contain deliberate API changes. Pin a version in production and read the [release notes](https://github.com/0mjs/zinc/releases) before upgrading. Upgrading from 0.2? Follow [Migrating to 0.3](/extra/migration-0.3/). From 0.1, start with [Migrating to 0.2](/extra/migration-0.2/).
:::

## Next steps

- [Quickstart](/guide/quickstart/) runs your first server.
- [Your First Route](/guide/first-route/) introduces handlers, input, and errors.
