// Package zinc040 is Zinc v0.4.0's root package, copied verbatim from the
// released module with only the package name changed. It is the frozen
// reference that FuzzRouterReference compares the router under development
// against, so 0.5's router work can't change routing behaviour unnoticed.
// It lives in the benchmarks module, outside Zinc's dependency-free core.
//
// These files change only for deliberate behaviour changes, each listed here,
// marked in the code, and described in ROUTER_SPEC.md:
//
//  1. Allow lists every method of a static path (0.5 P4). v0.4.0 indexed only
//     static paths with more than one accepted spelling, so a path like /v1
//     registered for GET as /v1/ (two spellings) and for DELETE as /v1 (one)
//     answered OPTIONS and 405 with Allow: GET, HEAD, OPTIONS, leaving out
//     DELETE, though DELETE /v1 was served.
//  2. Allow merges the methods of every spelling of a static path (0.5 P5).
//     v0.4.0 returned the first spelling that had any, so with POST /Users/users
//     and DELETE /users/users/, GET /Users/users answered Allow: POST, OPTIONS,
//     leaving out DELETE, though DELETE /Users/users was served.
//
// Otherwise, regenerate them from the v0.4.0 module:
//
//	src=$(go env GOMODCACHE)/github.com/0mjs/zinc@v0.4.0
//	for f in $src/*.go (excluding *_test.go): sed 's/^package zinc$/package zinc040/'
package zinc040
