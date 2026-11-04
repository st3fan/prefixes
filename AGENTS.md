# AGENTS.md

Single-package Go CLI that prints RIPEstat-originated prefixes for AS numbers as CSV. All logic lives in `main.go` (package `main`); zero external dependencies. Module and repo: `github.com/st3fan/prefixes`.

## Pull requests

- All changes land via pull requests against `main`; never push directly to `main`.
- Create a feature branch, push it, then `gh pr create`.

## Commands

- `go build -o prefixes .`
- `go test ./...` — fully offline; tests mock RIPEstat with `httptest`. Never add live-network tests.

## Architecture

- There is no library code. `run(ctx, args, output io.Writer, client apiClient)` is the entire pipeline and the unit-test seam.
- `apiClient.baseURL` is the test injection point (default `https://stat.ripe.net`). The endpoint paths `/data/as-overview/data.json` and `/data/ris-prefixes/data.json` are asserted in tests and fixed.
- `main()` only wires `os.Args`/`os.Stdout` to `run`; keep new CLI behavior inside `run`.

## Behavior contracts (covered by tests — don't silently change)

- All arguments are validated before any network I/O or output. Invalid input produces no partial CSV.
- ANS are normalized to uppercase `ASn` (e.g. `as015348` → `AS15348`); the raw arg is rejected if it can't parse.
- Flags `-6` (include IPv6) and `-q` (suppress logging) may appear anywhere in args, order-independent; every non-flag argument must parse as an ASN.
- Per-AS and final-total summaries are logged to stderr via the standard `log` package; CSV output goes to stdout. `-q` silences only those summaries — errors still print to stderr.
- Prefixes are deduplicated and sorted by address, then prefix length.
- CSV output is written only after all fetches succeed; errors go to stderr with exit 1. An AS with no visible prefixes gets one row with an empty `Prefix` column.

## Environment

- `go.mod` declares `go 1.27.1` — a recent toolchain is required (Go auto-downloads it via `GOTOOLCHAIN`).
- Running the binary itself requires internet access to stat.ripe.net; tests do not.