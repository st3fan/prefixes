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

- All arguments are validated before any network I/O or output. Invalid input produces no partial output.
- ANS are normalized to uppercase `ASn` (e.g. `as015348` → `AS15348`); the raw arg is rejected if it can't parse.
- Flags `-6` (include IPv6), `-q` (suppress logging), and `-o` (output format; consumes the next argument: `json`, `csv`, `jsonl`, or `minimal`) may appear anywhere in args, order-independent; every non-flag argument must parse as an ASN.
- Per-AS and final-total summaries are logged to stderr via the standard `log` package; every log line must start with a bracketed RFC 3339 UTC timestamp (e.g. `[2026-11-04T16:30:00Z]`). Emit log lines only through the `logf` helper. Output of every format goes to stdout. `-q` silences only those summaries — timestamped errors still print to stderr.
- Prefixes are deduplicated and sorted by address, then prefix length.
- Default format is `minimal`: raw prefixes one per line, no header or metadata; a prefix-less AS adds nothing. `csv` keeps the `AS,Name,Prefix` rows (empty `Prefix` column for a prefix-less AS); `jsonl` mirrors those rows as one `{"as","name","prefix"}` object per line, including an empty-`prefix` line for a prefix-less AS; `json` emits one object per AS with `as`, `name`, and a never-null `prefixes` array.
- All fetches complete before any output is written, regardless of format — a mid-run error produces no partial output and exits 1.

## Environment

- `go.mod` declares `go 1.27.1` — a recent toolchain is required (Go auto-downloads it via `GOTOOLCHAIN`).
- Running the binary itself requires internet access to stat.ripe.net; tests do not.