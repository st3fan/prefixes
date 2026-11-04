# prefixes

Print the currently originated IPv4 prefixes for one or more AS numbers.
Add `-6` to include IPv6 prefixes. Add `-q` to suppress the progress summaries
that are logged to stderr — each log line starts with a bracketed UTC
timestamp. Results always go to stdout.

## Install

Requires Go 1.27 or newer (the module declares `go 1.27.1`).

```sh
go install github.com/st3fan/prefixes@latest
```

This downloads, builds, and installs the `prefixes` binary into
`$(go env GOPATH)/bin` (usually `~/go/bin`). Make sure that directory is on
your `PATH`, for example:

```sh
export PATH="$(go env GOPATH)/bin:$PATH"
```

To upgrade an existing installation, run the same command again. To pin a
specific release instead of `@latest`, use its tag name.

## Usage

```sh
prefixes AS15348 AS32491 > prefixes.txt   # minimal, the default format
prefixes -o csv AS15348 AS32491 > prefixes.csv
prefixes -o json AS15348 AS32491 > prefixes.json
prefixes -o csv -6 AS15348 AS32491 > prefixes-with-ipv6.csv
```

## Output formats

- `minimal` (default): just the prefixes, one per line. An AS with no visible
  prefixes contributes nothing.
- `csv`: `AS,Name,Prefix` columns, one row per prefix. An AS with no visible
  originating prefixes gets one row with an empty `Prefix`. Rows follow the
  order of the command-line AS numbers; prefixes within each AS are sorted.
- `json`: an array with one object per AS number: `as`, `name`, and `prefixes`.
- `jsonl`: one JSON object per line — `as`, `name`, and `prefix` — mirroring the
  CSV columns. An AS with no visible prefixes gets one object with an empty
  `prefix`.

The command uses the [RIPEstat AS Overview](https://stat.ripe.net/docs/data-api/api-endpoints/as-overview)
and [RIS Prefixes](https://stat.ripe.net/docs/data-api/api-endpoints/ris-prefixes)
APIs. The name is the registered AS holder, and prefixes come from the latest
available RIS routing snapshot. An internet connection is required.