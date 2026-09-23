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

Zinc has two packages. Both come from the same module, so one `go get` installs both.

```go
import (
	"github.com/0mjs/zinc"            // app, router, context, binding, responses
	"github.com/0mjs/zinc/middleware" // first-party middleware
)
```

The core package depends only on the standard library plus encoders for YAML and TOML. The middleware package adds `golang-jwt` for the JWT middleware. Integrations with larger dependencies, such as OpenTelemetry or the official Prometheus client, stay in your code and plug in through `net/http`.

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
Minor releases can contain deliberate API changes. Pin a version in production and read the [release notes](https://github.com/0mjs/zinc/releases) before upgrading. Moving from 0.1? Follow [Migrating to 0.2](/extra/migration-0.2/).
:::

## Next steps

- [Quickstart](/guide/quickstart/) runs your first server.
- [Your First Route](/guide/first-route/) introduces handlers, input, and errors.
