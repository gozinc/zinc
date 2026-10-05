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
//  3. Custom methods are listed in Allow in sorted order (0.5 P5). Not
//     applied here: the fuzz test doesn't generate custom methods;
//     TestAllowSortsCustomMethods covers it.
//  4. ':' and '*' are literal except at a segment's start (0.5.1). v0.4.0
//     rejected them anywhere in a pattern, so /v1/users:batch couldn't be
//     registered.
//  5. A request with a trailing slash is tried for its method without the
//     slash and then as sent, before any 405 (0.7.2). v0.4.0 skipped the
//     spelling as sent when another method matched the path without the
//     slash, so with GET /x/{id} and POST /x/{id}/, POST /x/123/ answered 405,
//     but with 405s off it reached the POST route. Allow merges both
//     spellings' methods.
//  6. A catch-all's value is the rest of the path as sent (0.7.2). v0.4.0
//     dropped a trailing slash under default routing (/files/a/ gave "a")
//     and a leading one ("/files//a" gave "a"), so a URL built from such a
//     value routed back to a different one.
//  7. A parameter route with a trailing slash is also recorded without it,
//     unless routing is strict (0.7.2), as static routes always were. v0.4.0
//     couldn't reach GET /x/{id}/ at /x/1, and accepted GET /x/{id} beside
//     it though that made it unreachable; the pair now conflicts.
//  8. A registration conflict names both routes (0.7.2): "route already
//     registered: GET /x/{id}/ matches the same requests as GET /x/{id}".
//
// Otherwise, regenerate them from the v0.4.0 module:
//
//	src=$(go env GOMODCACHE)/github.com/0mjs/zinc@v0.4.0
//	for f in $src/*.go (excluding *_test.go): sed 's/^package zinc$/package zinc040/'
package zinc040
