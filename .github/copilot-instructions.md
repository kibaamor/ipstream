# Copilot Instructions

## Commands

```bash
go build ./...
go test -tags=ipstreamtests ./...
go test -tags=ipstreamtests -run TestName ./...   # single test
go vet ./...
goreleaser check
goreleaser healthcheck
```

## Architecture

Small dependency-free Go library (`streamer.go`, `streamer_chartype.go`, package `ipstream`).

`Streamer` implements `io.Writer` + `io.Closer` and scans an arbitrary byte stream for IPv4 and IPv6 addresses. For each segment it calls a `Handler`:

```go
type Handler interface {
	Handle(raw []byte, addr netip.Addr)
}
```

- `addr.IsValid() == true`: `raw` parsed successfully; `addr` is the result
- `addr.IsValid() == false`: `raw` is a non-IP segment (delimiter text, oversized token, or failed parse)

Concatenating all emitted `raw` slices reconstructs the input.

## Key Conventions

**Token accumulation:** `carrier` buffers IP-character runs across `Write` calls. `Close` drains it.

**Pre-filtering:** counters (`dotCount`, `colonCount`, `pctCount`, `maxColonRun`) reject obvious non-IP tokens before parsing. Apply the rejection gates in this order, and only continue while the relevant candidate still satisfies its gate:

1. IPv4 shape: candidates require exactly 3 dots.
2. IPv6 shape: colon-shaped candidates require 2-7 colons.
3. IPv6 compression: colon runs must be no longer than `::`.
4. IPv6 zone IDs: candidates allow at most one `%` zone separator.
5. IPv6 parse eligibility: after the gates above pass, parse only tokens with full 7-colon spelling, compressed `::` spelling, dotted-quad tails, or a `%` zone separator on an IPv6-shaped token.

Tokens that fail the heuristic are emitted with an invalid zero `addr` without calling `ParseAddr`.

**`maxTokenLen = maxIPv6WithZoneLen`:** longer tokens are emitted with an invalid zero `addr`. Zone IDs are capped at `maxIPv6ZoneLen`.

**IP characters:** tokens start with digits, `a-f`, `A-F`, `.`, `:`, `%`; after `%`, IPv6 zones may also contain `g-z`, `G-Z`, `_`, and `-`.

## Release Configuration

When changing `.goreleaser.yaml` or `.github/workflows/release.yml`, verify every publisher's runtime requirements before concluding:

- GoReleaser publishers may need external CLIs on the runner, e.g. Snapcraft needs `snapcraft`, Nix/NUR needs `nix-hash`, Docker needs Buildx, and Chocolatey needs `choco`.
- In `.github/workflows/release.yml`, GitHub Actions step `if:` expressions must not reference `secrets.*` directly. Map secrets to `env` first and test `env.*`.
- Personal access tokens for tap/bucket/NUR/WinGet publishing should be exposed as `GORELEASER_PUBLISH_TOKEN`, not a secret or env var starting with `GITHUB_`.
- After release config edits, run `goreleaser check`; run `goreleaser healthcheck` when publishers or workflow setup change.

## Library and CLI Boundaries

Keep the root `ipstream` library dependency-free. CLI code belongs under `cmd/*`; do not add third-party dependencies to the root package for CLI concerns. If a CLI needs extra dependencies, keep them isolated from the library API and build surface.

## Performance Work

This project has hot-path parser code. Performance changes must be benchmark-driven:

- Run relevant benchmarks before and after changing `Write`, token scanning, `tryParse`, `parseIPv4Fast`, char tables, overflow handling, or zone handling.
- Preserve correctness tests while optimizing; do not accept a faster path that changes the reconstruction invariant.
- Do not rewrite `netip.ParseAddr` or add a custom IPv6 parser unless the project maintainer explicitly requests it in a repository issue, PR comment, or email.
- Treat benchmark regressions as blockers unless the user explicitly accepts the tradeoff.

## Test Coverage Conventions

Parser changes need tests for streaming boundaries, not just single-buffer input:

- valid and invalid IPs split across multiple `Write` calls
- inputs ending without a delimiter, flushed by `Close`
- IPv4, IPv6, IPv4-mapped IPv6, and IPv6 zone IDs
- oversized tokens and tokens immediately following overflow
- reconstruction of the original input by concatenating emitted `raw` slices

Avoid test data that always ends with a delimiter; include no-trailing-delimiter cases.

## Comments and Documentation

Prefer short, precise comments only where the code has non-obvious constraints: heuristics, performance tricks, `unsafe`, overflow behavior, zone rules, and Go compiler/bounds-check details. Do not add comments that merely restate simple code.
