# as-to-cidr

Print the currently originated IPv4 prefixes for one or more AS numbers as CSV.
Add `-6` to include IPv6 prefixes.

```sh
go build -o as-to-cidr .
./as-to-cidr AS15348 AS32491 > prefixes.csv
./as-to-cidr -6 AS15348 AS32491 > prefixes-with-ipv6.csv
```

The output has `AS,Name,Prefix` columns and one row per prefix. An AS with no
visible originating prefixes gets one row with an empty `Prefix`. Rows follow
the order of the command-line AS numbers; prefixes within each AS are sorted.

The command uses the [RIPEstat AS Overview](https://stat.ripe.net/docs/data-api/api-endpoints/as-overview)
and [RIS Prefixes](https://stat.ripe.net/docs/data-api/api-endpoints/ris-prefixes)
APIs. The name is the registered AS holder, and prefixes come from the latest
available RIS routing snapshot. An internet connection is required.
