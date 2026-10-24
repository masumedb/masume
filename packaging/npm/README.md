# masume

A database client for the terminal, with an SQL editor, notebooks, an AI chat and an MCP server. PostgreSQL, MySQL, MariaDB, SQL Server, ClickHouse, SQLite, Cassandra, ScyllaDB, Redis, MongoDB, and hosted services. See the [engine list](https://github.com/masumedb/masume/blob/master/docs/engines.md#protocols).

See the [project README](https://github.com/masumedb/masume#readme) for the full description, the screenshots and the configuration.

## Install

```sh
npm install -g masume
masume
```

Or run it without an install:

```sh
npx masume
```

macOS, Linux and Windows, on x64 and arm64. Node 18 or later.

The package holds no binary. On install it downloads the release archive for the current platform from [GitHub](https://github.com/masumedb/masume/releases) and checks it against the `checksums.txt` copy inside the package. With `--ignore-scripts` the download runs on the first `masume` command instead.

## License

Apache-2.0
