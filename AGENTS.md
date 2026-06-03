# AGENTS.md

## Summary

Single-module Go library + CLI (`cmd/ipstream`) that extracts IPv4/IPv6 addresses from byte streams. Module path: `github.com/kibaamor/ipstream`.

## Build tags (critical)

Most tests and benchmarks require a build tag. Without it, `go test ./...` runs nothing.

```
ipstreamtests         required for all tests and benchmarks
ipstreamstats         enables parse-stat counters (atomics) in benches
```

Default Makefile targets use `TEST_TAGS ?= ipstreamtests`.
Coverage (`make coverage`) uses `PARSE_STATS_BUILD_FLAGS` which adds `ipstreamstats`.
VSCode settings already set both tags.

**Commands:**
```bash
make test      # go test -tags=ipstreamtests ./...
make bench     # go test -tags=ipstreamtests -bench . -benchmem ./...
make coverage  # go test -tags=ipstreamtests,ipstreamstats -covermode=atomic -coverprofile ...
```

## Lint and format

```bash
make lint      # golangci-lint run ./...
make lint-fix  # golangci-lint run --fix ./...
```

Formatter: `gofumpt` + `goimports` (enforced by `.golangci.yml` v2). Must be installed separately — not in `go.mod`.

## Architecture

```
streamer.go              # core: Streamer (Write/Flush), parseIPv4Fast, tryParse
streamer_chartype.go     # 256-byte lookup table for character classification
parse_stats_enabled.go   # (+ipstreamstats) atomics for counting parse calls
parse_stats_disabled.go  # (-ipstreamstats) no-op
cmd/ipstream/main.go     # CLI: reads stdin in 32KB chunks, emits one IP per line
```

**Key design notes:**
- `Streamer.Write` is streaming across calls — IP tokens can span `Write` boundaries via `carrier`.
- `Flush` emits any buffered partial token and resets state; same Streamer can continue receiving `Write` afterwards.
- IPv4 uses a custom `parseIPv4Fast` parser (no allocations); IPv6 delegates to `netip.ParseAddr`.
- Oversized tokens (>61 bytes, the max IPv6-with-zone length) are rejected early via `overflowing` flag.
- Handler segments are emitted in order; reconstruction of raw segments must equal original input (tested).

## Test conventions

- Tests live in `streamer_test.go` (external `ipstream_test` package) and `streamer_internal_test.go` (internal `ipstream` package).
- CLI tests in `cmd/ipstream/main_test.go` (internal `package main`).
- All test files require `//go:build ipstreamtests`.
- Parse-stat bench files use conditional compilation: `ipstreamstats` tag selects `streamer_bench_parse_stats_enabled_test.go` vs disabled.
- Benchmarks reset parse stats per bench, report as custom metrics.

## Release

GoReleaser v2 via `.github/workflows/release.yml`:
- Triggered by `v*` tags or manual `workflow_dispatch` (snapshot for non-tag).
- CI runs tests first: `go clean -testcache && go test -tags=ipstreamtests ./...`.
- Builds use `CGO_ENABLED=0`, UPX compression, multi-platform (linux/darwin/windows, amd64/arm64).
- Docker image pushed to `ghcr.io/kibaamor/ipstream`.