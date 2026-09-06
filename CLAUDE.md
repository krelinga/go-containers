# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Status

This repo is greenfield: `go.mod` and `.devcontainer/` are the only tracked files. There is no
package source, no tests, and no architecture to preserve yet. Treat design decisions as open, and
do not assume a prior structure exists.

Intent, per the module path `github.com/krelinga/go-containers`: a generic (type-parameterized)
container library.

## Commands

```sh
go build ./...
go vet ./...
go test ./...
go test -run '^TestName$' ./...        # single test
go test -run '^TestName$/^subtest$' ./...
go test -race ./...
gofmt -l .                             # list unformatted files; -w to rewrite
```

## Conventions

- **Package name is `containers`, not `go-containers`.** The `go-` prefix belongs to the repo name
  only; it is not part of the import identifier.
- `go.mod` pins `go 1.26.7` at patch granularity, so the toolchain must be at least that version.
  The devcontainer's Go feature supplies it; a host Go older than 1.26.7 will refuse to build.
- Single flat package at the repo root — add new container types as sibling files, not subpackages,
  unless there is a reason to split.
