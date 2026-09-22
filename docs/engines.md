# Engines

A bug report for an engine should include the service, the server version, the statement, and the error.

## Protocols

Engines in one protocol family share a driver. Catalogs, SQL features, permissions, plans, and hosted restrictions can still differ.

| Protocol | Engines |
| --- | --- |
| PostgreSQL | PostgreSQL, CockroachDB, TimescaleDB, Redshift, Neon, Supabase, Aurora PostgreSQL, YugabyteDB |
| MySQL | MySQL, MariaDB, TiDB, PlanetScale, Aurora MySQL |
| SQLite | SQLite |
| libSQL | Turso |
| RESP | Redis |
| CQL | Cassandra, ScyllaDB |
| TDS | SQL Server, Azure SQL Database |
| ClickHouse native | ClickHouse |
| MongoDB wire | MongoDB, Amazon DocumentDB |

## Capabilities

Most capabilities are static defaults. The interface shows an action only when its flag is set. A flag does not guarantee server support or permission, and a shown action can still fail.

| Engine | Plans | Measures | Transactions | Cancels | Activity | Locks | Load | Sorts | Truncates | DDL |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| aurora-mysql | yes | yes | yes | yes | yes | no | yes | yes | yes | yes |
| aurora-postgres | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| cassandra | no | no | no | no | no | no | no | no | yes | yes |
| azure-sql | yes | yes | yes | no | yes | yes | no | yes | yes | yes |
| clickhouse | yes | no | no | yes | yes | no | yes | yes | yes | yes |
| cockroach | yes | yes | yes | no | no | no | no | yes | yes | yes |
| documentdb | yes | yes | yes | no | yes | no | no | yes | no | no |
| mariadb | yes | yes | yes | yes | yes | no | yes | yes | yes | yes |
| mongodb | yes | yes | yes | no | yes | no | no | yes | no | no |
| mysql | yes | yes | yes | yes | yes | no | yes | yes | yes | yes |
| neon | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| planetscale | yes | yes | yes | no | no | no | no | yes | yes | yes |
| postgres | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| redis | no | no | no | no | yes | no | no | no | no | no |
| redshift | yes | no | yes | yes | yes | no | no | yes | yes | yes |
| scylladb | no | no | no | no | no | no | no | no | yes | yes |
| sqlite | yes | no | yes | no | no | no | no | yes | no | yes |
| sqlserver | yes | yes | yes | no | yes | yes | yes | yes | yes | yes |
| supabase | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| tidb | yes | yes | yes | yes | yes | no | no | yes | yes | yes |
| timescale | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| turso | yes | no | yes | no | no | no | no | yes | no | yes |
| yugabyte | yes | yes | yes | yes | yes | yes | no | yes | yes | yes |

| Flag | Meaning |
| --- | --- |
| Plans | Query plans for supported statements |
| Measures | Execution measurements in supported plans |
| Transactions | Explicit begin, commit, and rollback |
| Cancels | Cancel of the running query. Without it, the cancel key is hidden and the run spinner shows `this engine cannot stop a running statement` |
| Activity | Session or operation list. Stopping another session also needs a server permission |
| Locks | Blocking relationships between sessions |
| Load | Connection counts, limits, and server start time. Other metrics vary by engine |
| Sorts | Server-side sorting for supported reads |
| Truncates | `TRUNCATE` in the object menu |
| DDL | Object definitions, read from the server or generated |

On MongoDB, the transaction and atomic staged-change flags come from the deployment's `hello` response. Replica sets and sharded clusters support transactions; standalone servers do not. A standalone server applies staged changes one at a time, so earlier changes stay after a failure.

| Flag | Default |
| --- | --- |
| Plans every statement | CockroachDB only |
| Write previews | Every SQL engine except ClickHouse and Cassandra. Not MongoDB |
| Read-only mode | Every engine except TiDB. Client-only on MongoDB, Amazon DocumentDB, SQL Server, Azure SQL Database, Redis, Cassandra, ScyllaDB and Turso |
| Atomic staged changes | Every engine except ClickHouse, which has no transactions. Set after connection on MongoDB |
| Statement statistics | Off on every engine until connection. Set after an extension check on the PostgreSQL family, a permission check on SQL Server, and a query log check on ClickHouse |

## Dashboard metrics

The dashboard hides unsupported panels. Activity, lock relationships, server load, and statement statistics are separate capabilities.

| Engines | Metrics | Requires |
| --- | --- | --- |
| PostgreSQL, TimescaleDB, Neon, Supabase, Aurora PostgreSQL | Activity, locks, connections, connection limit, start time, transaction count, WAL bytes, temporary files, cache hits, replication lag | PostgreSQL statistics views, functions, and sufficient permissions |
| MySQL, MariaDB, Aurora MySQL | Activity, connections, connection limit, start time | `information_schema.processlist`, `performance_schema.global_status`, and `@@max_connections` |
| ClickHouse | Running statements, connections, connection limit, start time, statement statistics | `system.processes`, `system.metrics`, `system.server_settings`, and `system.query_log` |
| SQL Server | Activity, locks, connections, connection limit, start time, statement statistics | `sys.dm_exec_sessions`, `sys.dm_exec_requests`, `sys.dm_tran_locks`, `sys.dm_os_sys_info`, and the VIEW SERVER STATE permission |
| Azure SQL Database | Activity, locks, statement statistics | The same views at database scope, and the VIEW DATABASE STATE permission |
| YugabyteDB | Activity, locks, statement statistics | `pg_stat_activity`, `pg_locks`, and `pg_stat_statements`, which the server loads by default |
| Redshift, TiDB | Activity only | The adapter's activity query and sufficient permissions |
| MongoDB, Amazon DocumentDB | Current operations | `currentOp` and sufficient permissions |
| Redis | Connected clients | `CLIENT LIST`, and `CLIENT KILL` to stop a client |
| CockroachDB, PlanetScale, SQLite, Turso, Cassandra, ScyllaDB | No dashboard metrics | None |

YugabyteDB has no PostgreSQL write-ahead log. `pg_current_wal_lsn()` returns `not yet supported` and fails the whole load query, so the server load panel is hidden. Activity, lock waits and statement statistics work.

PostgreSQL metrics use `pg_stat_activity`, `pg_locks`, `pg_stat_database`, WAL functions, and replication statistics. Replication lag is shown only when the query returns a value. The cache hit rate needs recorded block reads or hits. Every rate needs at least two counter samples.

PostgreSQL-family statement statistics need the `pg_stat_statements` extension. masume checks the extension catalog when the session opens, on engines that have one. The server must load the extension and allow access to its statistics. The panel shows call counts, mean and total execution time, and returned rows.

ClickHouse statement statistics come from `system.query_log`, which the server must be configured to write. masume reads the table once when the session opens and shows the panel if the read succeeds. Each row is one statement shape, grouped by normalized query hash.

SQL Server statement statistics come from `sys.dm_exec_query_stats` and need the VIEW SERVER STATE permission. masume reads the view once when the session opens and shows the panel if the read succeeds. SQL Server load metrics do not include the PostgreSQL counters, cache hit rate, or replication lag.

MySQL-family load metrics do not include PostgreSQL counters, cache hit rate, replication lag, or statement statistics. MariaDB's `performance_schema.global_status` table needs version 10.5.2 or later and the Performance Schema. The adapter does not read MySQL lock relationships.

The static flags do not check every statistics view, extension setting, or permission. A missing dependency can cause a dashboard error.

## Read-only access

On a read-only profile, the client rejects every statement it recognizes as a write. PostgreSQL-family sessions also set read-only mode on the server. MySQL and MariaDB use `SET SESSION TRANSACTION READ ONLY`. SQLite opens existing files with `mode=ro`. ClickHouse uses `SET readonly = 2`, which rejects writes but still accepts the settings the driver sends. MongoDB, Amazon DocumentDB, SQL Server, Azure SQL Database, Redis, Cassandra and ScyllaDB have client-only checks.

The libSQL server does not support `mode=ro`, so a read-only Turso profile has a client-only check.

TiDB does not enforce a read-only session. A TiDB profile set to `mode = "read-only"` fails to connect.

MCP read-only access opens a read-only connection, whatever the profile mode. The connection fails on a server without read-only sessions, so a TiDB profile with read-only MCP access cannot connect.

When an agent sends a statement that the client classifies as a read, the statement runs in a read-only transaction: `BEGIN READ ONLY` on the PostgreSQL family, `START TRANSACTION READ ONLY` on MySQL and MariaDB. A routine that the statement calls cannot write either. On other servers, and on a connection already in a transaction, the statement runs as is.

Database permissions apply separately from the client checks.

## Default port and TLS

| Engine | Port | Default `sslmode` |
| --- | --- | --- |
| aurora-mysql | 3306 | `prefer` |
| aurora-postgres | 5432 | `prefer` |
| cassandra | 9042 | unset; no TLS |
| clickhouse | 9000 | unset; no TLS |
| cockroach | 26257 | `prefer` |
| documentdb | 27017 | `require` |
| mariadb | 3306 | `prefer` |
| mongodb | 27017 | unset; no TLS |
| mysql | 3306 | `prefer` |
| neon | 5432 | `require` |
| planetscale | 3306 | `require` |
| postgres | 5432 | `prefer` |
| redis | 6379 | unset; no TLS |
| redshift | 5439 | `require` |
| scylladb | 9042 | unset; no TLS |
| sqlite | none | none |
| azure-sql | 1433 | `require` |
| sqlserver | 1433 | unset; encrypts the login only |
| supabase | 5432 | `require` |
| tidb | 4000 | `prefer` |
| timescale | 5432 | `prefer` |
| turso | 443 | `require` |
| yugabyte | 5433 | `prefer` |

Most PostgreSQL-family and MySQL-family engines with an unset `sslmode` behave as `prefer`.

For PostgreSQL-family and MySQL-family engines, `allow` and `prefer` permit unencrypted fallback. `require` uses TLS without certificate verification. `verify-ca` checks the certificate chain. `verify-full` also checks the host name. Verification uses `sslrootcert` if set, otherwise the system trust roots.

ClickHouse is different: the native protocol does not negotiate TLS. Unset, `allow`, and `prefer` connect without encryption. `require` encrypts without certificate verification. `verify-ca` and `verify-full` verify the certificate. Encrypted ClickHouse listens on a separate port, 9440 by default.

SQL Server is different. Azure SQL Database accepts only encrypted sessions, and its default is `require`. On SQL Server, unset, `allow`, and `prefer` encrypt the login only, and the rest of the session is unencrypted. `disable` encrypts nothing. `require` encrypts the whole session without certificate verification. `verify-ca` and `verify-full` verify the certificate.

For Cassandra and ScyllaDB, unset or `disable` uses no TLS. Every other mode uses TLS, and `verify-ca` and `verify-full` check the certificate.

For Redis, unset or `disable` uses no TLS. Every other mode uses TLS. A `rediss://` target sets `verify-full`.

Amazon DocumentDB accepts only encrypted connections, and its default is `require`.

For MongoDB, unset or `disable` uses no TLS. Explicit `allow`, `prefer`, and `require` use TLS without certificate verification and without unencrypted fallback. MongoDB also supports `verify-ca` and `verify-full`.

For Turso, `disable` opens `ws://`, and every other mode opens `wss://`. A hosted database accepts only TLS. Turso connections verify against the system trust store and ignore the certificate files.

Every other engine reads `sslrootcert`, `sslcert`, and `sslkey`. `sslrootcert` replaces the system trust roots for `verify-ca` and `verify-full`. The client key pair is sent in every mode that encrypts. See [Certificate files](configuration.md#certificate-files).

See [profiles](configuration.md#profiles) for the other profile keys. Connection targets do not forward native URL options. See [connection targets](usage.md#connection-targets).

## MongoDB

A query tab accepts a subset of MongoDB shell calls.

```js
db.orders.find({status: "new"}).sort({total: -1})
```

The parser accepts extended JSON, unquoted document keys, single-quoted strings, comments, trailing commas, and regular expressions such as `/pattern/i`.

Supported value helpers are `ObjectId`, `ISODate`, `Date`, `NumberLong`, `NumberInt`, `NumberDouble`, `NumberDecimal`, and `UUID`. General JavaScript variables, loops, and function evaluation are unsupported.

`db.getSiblingDB("name")` selects a database for a statement. `db.getCollection("name")` selects a collection by its quoted name. Statements end at a top-level semicolon or newline. A statement continues across lines inside an open document and on lines that start with `.`.

### Supported calls

Supported database calls:

| Call | Supported arguments |
| --- | --- |
| `runCommand`, `adminCommand` | One command document. `adminCommand` uses the admin database |
| `getCollectionNames` | No filter or options |
| `createCollection` | Collection name only |
| `dropDatabase` | No options |

Supported collection calls:

| Call | Supported arguments and behavior |
| --- | --- |
| `find`, `findOne` | Filter, optional projection, and the supported chains below |
| `aggregate` | Pipeline array only |
| `countDocuments`, `count` | Filter only. Both use `CountDocuments` |
| `estimatedDocumentCount` | No options |
| `distinct` | Field name and optional filter |
| `getIndexes` | No options |
| `insertOne`, `insertMany`, `insert` | Document or document array. A document inserts one, an array inserts many |
| `updateOne`, `updateMany`, `replaceOne` | Filter and update or replacement only |
| `deleteOne`, `deleteMany` | Filter only |
| `remove` | Filter and optional boolean or `{justOne: true}` |
| `findOneAndUpdate`, `findOneAndReplace` | Filter and update or replacement. Returns the original document |
| `findOneAndDelete` | Filter. Returns the removed document |
| `createIndex` | Key document and optional `name`, `unique`, and `sparse` settings |
| `dropIndex` | Index name |
| `drop` | No options |

Find chains apply `sort`, `projection`, `limit`, and `skip`. The parser accepts `pretty`, `toArray`, `batchSize`, `hint`, `allowDiskUse`, and `collation`, but ignores them at execution. Other find chains fail.

Most methods ignore extra arguments and chains without an error, and do not forward unsupported options. Update options such as `upsert`, find-and-update options such as `returnDocument`, and aggregate options such as `allowDiskUse` have no effect. Index options other than `name`, `unique`, and `sparse` are ignored.

`runCommand` and `adminCommand` send the command document to the server as is, and the result is the raw reply document. Cursor replies are not iterated. Client access checks still apply.

Completion and syntax highlighting include more methods than execution supports. Calls such as `bulkWrite`, `update`, `save`, and `createIndexes` are not implemented as shell methods.

Plans support `find`, `aggregate`, `count`, `countDocuments`, and `distinct`. Shell `.explain()` chaining is unsupported. Use the plan action or headless `--explain`.

### Documents and access

Collection metadata comes from a sample of up to 100 documents. Result columns come from the returned documents. When streaming, columns come from the first batch. A column with different non-null types is `mixed`. Staged edits use the row's `_id`.

Streaming drops a field that first appears after the first batch and reports an error. By then, headless output can already be incomplete. See [headless streaming](headless.md#memory-and-streaming).

Amazon DocumentDB, the `documentdb` engine, uses the same wire protocol and calls. The cluster accepts only encrypted connections, so its default `sslmode` is `require`. A cluster in a VPC with no public route needs an SSH tunnel or a bastion. Transactions need engine version 4.0 or later. As on MongoDB, the session checks the deployment at connect.

Set `user` only for authenticated MongoDB connections. Without a user, no credentials are sent. The profile fields are the only authentication settings, and native URL query options cannot add others.

## SQL Server

masume connects to SQL Server 2016 and later, and to Azure SQL Database as the `azure-sql` engine. Both use TDS. The connection opens one database, and its relations are listed under their schemas. `dbo` is the default schema for most logins.

Pages after the first use `OFFSET` and `FETCH NEXT`, which the server accepts only after `ORDER BY`. A read without its own sort gets `ORDER BY (SELECT NULL)`. The rows then come in server order, which can change between pages. Add a sort when pages must line up. The first page has no `OFFSET`, and the client limits the rows as it reads them. A read that the server cannot sort, such as `select next value for`, still runs. The `SELECT` that the object menu generates uses `TOP` instead.

The statement separator is the semicolon. `GO` is a batch separator of the command-line tools, not of the server, and a buffer that contains `GO` fails. The server accepts `CREATE VIEW`, `CREATE FUNCTION`, `CREATE PROCEDURE`, and `CREATE TRIGGER` only as the first statement in a batch. Put each one in its own tab or cell.

Identity and computed columns reject client values, and the row form leaves them out. The object menu renames with `sp_rename`.

A write with an `OUTPUT` clause returns rows, and masume shows them instead of a count. The server rejects `OUTPUT` without `INTO` on a table with an enabled trigger. Use `OUTPUT INTO` or disable the trigger.

Statement diagnostics use `sys.dm_exec_describe_first_result_set`, which compiles the statement and returns the error as a row. A statement with a `:name` parameter is checked after the parameters have values.

An estimated plan comes from `SET SHOWPLAN_ALL ON`. A measured plan comes from `SET STATISTICS PROFILE ON`. Both show estimated and actual rows per step, but no time per step.

The dashboard stops another session with `KILL`, which ends the session and its transaction. T-SQL cannot cancel one statement in another session. The activity list has no cancel action, and a running statement on this connection has no cancel either; `Ctrl+X` is hidden on SQL Server. Set `statement_timeout_ms` to limit statement time. On a timeout, the driver stops the statement and the client reconnects. The old connection is unusable, an open transaction is lost with it, and the client reports this.

SQL Server has no read-only session, so only the client enforces a read-only profile. SQL Server also has no materialized views; an indexed view is shown as a view.

Azure SQL Database allows one database per connection, and `USE` cannot switch to another. Its dynamic management views are scoped to that database and need the VIEW DATABASE STATE permission. The server load panel is hidden.

## Cassandra

masume connects to Cassandra 4 and 5 over the native protocol; the default port is 9042. A keyspace is the schema, and the connection opens one of them. The tree shows every keyspace in the cluster.

A query tab takes CQL. The statement separator is the semicolon, and a buffer that contains several statements runs them one at a time.

The catalog comes from `system_schema`. A table lists the partition key first, then the clustering columns, then the other columns by name. A column outside the key is nullable. The table definition is built from the catalog, with the clustering order and the secondary indexes.

CQL has no `EXPLAIN`, no join, no user transaction, and no `OFFSET`. Each page after the first reads past the rows before it, so a deep page reads every row it skips. There is no row count. Staged changes run in one logged batch, which the server applies in full or not at all.

The grid stores an edited cell as text, and the driver marshals a value by its Go type. Each value is therefore converted to its column type before binding, with separate forms for decimal, uuid, timestamp, and integer columns.

Cassandra has no read-only session, so only the client enforces a read-only profile. The dashboard has no panels.

ScyllaDB, the `scylladb` engine with the `scylla://` scheme, uses the same protocol and CQL. The tree also hides `system_replicated_keys` and `audit`, which ScyllaDB reserves in addition to the Cassandra system keyspaces.

## Redis

Redis has no SQL. A query tab takes Redis commands, one per line:

```
SET user:1 "a name"
GET user:1
```

A command ends at the line break. A key can contain a semicolon. masume knows the Redis command set and marks unknown commands. Each command is classed as a read, a write, a delete, a whole-database operation, or a script. A script gets the highest risk class.

The tree comes from a scan of the key space and shows the keys of the last scan. A new key appears after the next scan. Each key name prefix is shown as a relation with four columns: key, type, TTL, and value.

The connection opens one numbered database, set by `database` in the profile. A password without a user works with the `requirepass` server setting, which has no user.

## Turso

masume connects to Turso and to any libSQL server over the websocket protocol. The host name is the database, and the auth token is the password. In a `libsql://` target, the token is the `authToken` parameter.

The SQL dialect is SQLite, and the catalog comes from the same `pragma` functions.

The server sends no column types. A result column gets the type of its first value: `text`, `integer`, `real`, or `blob`. A local libSQL file opens as a SQLite profile.

## ClickHouse

masume connects to ClickHouse over its native protocol; the default port is 9000. Each ClickHouse database is a schema, and the connected database is the default. The object tree shows only that database.

The protocol accepts one statement per call. A buffer of several statements runs them one at a time and returns the result of the last one. Such a buffer cannot bind values; `:name` parameters work only in a buffer with one statement.

The server has no user transactions and rejects begin, commit, and rollback. Staged changes are applied one at a time. If a change fails, the earlier changes stay, and the report shows the failed change.

A staged row edit is written as `ALTER TABLE … UPDATE`, which is a table mutation. The session sets `mutations_sync = 1`, so the server finishes the mutation before it replies, and the grid then reads the row back. A staged delete is written as `DELETE FROM`. A write reports no row count, because the server does not expose the row count of a mutation.

The server has no foreign keys and stores table constraints only in the `CREATE TABLE` statement. The relation panes show neither, and the diagram shows no relationships. The table sorting key is shown as its primary index, with the data-skipping indexes next to it. A materialized view is shown as a materialized view, and its storage table is hidden.

A new table needs a table engine. The object menu writes `ENGINE = MergeTree` with an `ORDER BY` clause. An import writes the same and orders by the first column of the file. ClickHouse has no auto-increment column, so a new table gets a plain `UInt64` key.

`EXPLAIN` returns the plan of a read, one step per line. Plans have no run measurements, so the pane shows only the estimate. Statement diagnostics use `EXPLAIN PLAN`, which resolves every name in a read without running it. Statements other than reads are not checked.

`KILL QUERY` stops one statement, and both stop actions in the activity list use it. A ClickHouse statement has no session to close. The server identifies a statement by a text query ID, so the activity list numbers its rows instead. A stop applies to the statement in the listed row.