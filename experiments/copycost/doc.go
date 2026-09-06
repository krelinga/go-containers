// Package copycost measures what a defensive copy actually costs in Go.
//
// It exists to answer a design question for the parent container library: when
// an accessor hands out a slice or map, should it return a copy or a read-only
// view? The benchmarks here isolate the four costs that decide it — allocation
// floor, memory bandwidth, GC scanning of retained pointers, and the
// asymptotic blowup of copying inside a caller's loop.
//
// This package is a measurement harness, not a test of the library. It is a
// separate module so that `go test ./...` at the repo root never runs it.
//
// Run ./run.sh to regenerate bench.txt. See RESULTS.md for the recorded
// numbers and their provenance, and docs/adr/0001-views-vs-defensive-copies.md
// for the decision they informed.
package copycost
