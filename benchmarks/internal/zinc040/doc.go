// Package zinc040 is Zinc v0.4.0's root package, copied verbatim from the
// released module with only the package name changed. It is the frozen
// reference that FuzzRouterReference compares the router under development
// against, so 0.5's router work can't change routing behaviour unnoticed.
// It lives in the benchmarks module, outside Zinc's dependency-free core.
//
// Do not edit these files. Regenerate them from the v0.4.0 module:
//
//	src=$(go env GOMODCACHE)/github.com/0mjs/zinc@v0.4.0
//	for f in $src/*.go (excluding *_test.go): sed 's/^package zinc$/package zinc040/'
package zinc040
