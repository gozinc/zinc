# Security

## Reporting a problem

Report a security problem privately, through **Report a vulnerability** on the repository's [Security tab](https://github.com/0mjs/zinc/security), not in a public issue. Include what you found, the Zinc version, and the smallest program that shows it.

A fix ships in a release with a security advisory that credits you, unless you'd rather not be named.

## Supported versions

Zinc is before 1.0. Fixes go into the latest minor version only:

| Version | Gets security fixes |
| --- | --- |
| 0.7.x | Yes |
| Older | No: upgrade to 0.7 |

## What's in scope

The core module (`github.com/0mjs/zinc`) and its middleware packages (`github.com/0mjs/zinc/middleware/...`). The benchmark suite, the docs site and the example apps aren't.
