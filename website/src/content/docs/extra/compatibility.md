---
title: Compatibility
description: What Zinc promises between versions before 1.0, what counts as a breaking change, which Go versions are supported, and how to report a security problem.
---

Zinc is before 1.0, so any minor version, such as 0.6 to 0.7, can change the public API. This page says what changes are possible in each kind of release, and where you'll read about them first.

## What each release can change

| Release | Example | Can change |
|---|---|---|
| Patch | 0.6.0 → 0.6.1 | Fixes. Behavior changes only where the old behavior was a bug, each listed in the upgrade notes. No exported Go API changes. |
| Minor | 0.6 → 0.7 | Anything, including exported names and default behavior. Every change is in that version's [upgrade notes](/extra/migration-0.6/). |

A patch release can still change what your program does, when what it did was wrong. 0.6.1, for example, binds embedded structs that 0.6.0 ignored. Read the upgrade notes for every release, not only minor ones.

## What counts as a breaking change

Zinc treats these as part of its API, and changes them only in a minor release, with an entry in the upgrade notes:

- Exported Go names: functions, types, methods, fields and constants, and their signatures.
- Struct tag meanings, such as what `query`, `default` or `validate` do.
- Default behavior an app relies on without configuring it, such as which status a typed handler sends or which endpoints an app serves.
- In the generated OpenAPI spec: operation IDs, component names, and which fields are required. A generated client can stop compiling when one of these changes, even though your Go code still builds.
- Error responses from the default error handler: their status codes and JSON shape.

These aren't covered, and can change in any release:

- Unexported names and anything in an `internal` package.
- The exact text of error messages and log lines.
- Performance, in either direction. The [benchmarks](/extra/benchmarks/) are re-run for every release.
- The order of keys in the generated spec, as long as the content is the same.

## Supported Go versions

Each release supports the Go version in its `go.mod` and newer. CI tests the three most recent Go releases. Zinc 0.7 needs Go 1.25 or newer.

## Security problems

Report a security problem privately, through the **Report a vulnerability** button on the repository's [Security tab](https://github.com/0mjs/zinc/security), not in a public issue. Fixes go into the latest minor version.

## Next steps

- [Upgrading to 0.7](/extra/migration-0.7/): what changed in 0.7.
- [Upgrading to 0.6](/extra/migration-0.6/): what changed in each 0.6 release.
- [Release notes](https://github.com/0mjs/zinc/releases): every change in every version.
