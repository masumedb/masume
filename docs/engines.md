# Engines

Engine problem reports should include the service, the server version, the statement, and the error.

## Protocols

Engines in one protocol family share a driver, but catalogs, SQL features, permissions, plans, and hosted restrictions can differ.

| Protocol | Engines |
| --- | --- |
| PostgreSQL | PostgreSQL, CockroachDB, TimescaleDB, Redshift, Neon, Supabase, Aurora PostgreSQL |
| MySQL | MySQL, MariaDB, TiDB, PlanetScale, Aurora MySQL |
| SQLite | SQLite |
| libSQL | Turso |
| RESP | Redis |
| TDS | SQL Server, Azure SQL Database |
| ClickHouse native | ClickHouse |
| MongoDB wire | MongoDB |

## Capabilities

Most capabilities are static defaults. The interface uses these flags to decide which actions to show. A flag does not guarantee server support or permission; a shown action can still fail.

| Engine | Plans | Measures | Transactions | Cancels | Activity | Locks | Load | Sorts | Truncates | DDL |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| aurora-mysql | yes | yes | yes | yes | yes | no | yes | yes | yes | yes |
| aurora-postgres | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| azure-sql | yes | yes | yes | no | yes | yes | no | yes | yes | yes |
| clickhouse | yes | no | no | yes | yes | no | yes | yes | yes | yes |
| cockroach | yes | yes | yes | no | no | no | no | yes | yes | yes |
| mariadb | yes | yes | yes | yes | yes | no | yes | yes | yes | yes |
| mongodb | yes | yes | yes | no | yes | no | no | yes | no | no |
| mysql | yes | yes | yes | yes | yes | no | yes | yes | yes | yes |
| neon | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| planetscale | yes | yes | yes | no | no | no | no | yes | yes | yes |
| postgres | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| redis | no | no | no | no | yes | no | no | no | no | no |
| redshift | yes | no | yes | yes | yes | no | no | yes | yes | yes |
| sqlite | yes | no | yes | no | no | no | no | yes | no | yes |
| sqlserver | yes | yes | yes | no | yes | yes | yes | yes | yes | yes |
| supabase | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| tidb | yes | yes | yes | yes | yes | no | no | yes | yes | yes |
| timescale | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| turso | yes | no | yes | no | no | no | no | yes | no | yes |

| Flag | Meaning |
| --- | --- |
| Plans | Query plans for supported statements |
| Measures | Execution measurements in supported plans |
| Transactions | Explicit begin, commit, and rollback |
| Cancels | Dedicated cancellation of the current query. An engine without it hides the cancel key, and the run wheel shows `this engine cannot stop a running statement` |
| Activity | Session or operation listing. Stopping another session also needs server permission |
| Locks | Blocking relationships between sessions |
| Load | Connection counts, limits, and server start time. Additional metrics depend on the engine |
| Sorts | Server-side sorting for supported reads |
| Truncates | `TRUNCATE` in the object menu |
| DDL | Object definition retrieval or generation |

MongoDB transaction and atomic staged-write flags depend on the deployment's `hello` response. Replica sets and sharded clusters support transactions; standalone servers do not. A standalone server applies staged changes separately, so earlier changes can remain after a failure.

| Flag | Default |
| --- | --- |
| Plans every statement | CockroachDB only |
| Write previews | Every SQL engine except ClickHouse. No MongoDB |
| Read-only mode | Every engine except TiDB and Turso. MongoDB, SQL Server and Turso enforcement is client-only |
| Atomic staged changes | Every engine except ClickHouse, which holds no transaction. MongoDB adjusts this after connection |
| Statement statistics | No engine before connection. PostgreSQL-family sessions enable this after an extension check, SQL Server sessions after a permission check, and ClickHouse sessions after a check of its query log |

## Dashboard metrics

The dashboard omits unsupported panels. Activity, lock relationships, server load, and statement statistics are separate capabilities.

| Engines | Implemented metrics | Dependencies |
| --- | --- | --- |
| PostgreSQL, TimescaleDB, Neon, Supabase, Aurora PostgreSQL | Activity, locks, connections, connection limit, start time, transaction count, WAL bytes, temporary files, cache hits, replication lag | PostgreSQL statistics views, functions, and sufficient permissions |
| MySQL, MariaDB, Aurora MySQL | Activity, connections, connection limit, start time | `information_schema.processlist`, `performance_schema.global_status`, and `@@max_connections` |
| ClickHouse | Running statements, connections, connection limit, start time, statement statistics | `system.processes`, `system.metrics`, `system.server_settings`, and `system.query_log` |
| SQL Server | Activity, locks, connections, connection limit, start time, statement statistics | `sys.dm_exec_sessions`, `sys.dm_exec_requests`, `sys.dm_tran_locks`, `sys.dm_os_sys_info`, and the VIEW SERVER STATE permission |
| Azure SQL Database | Activity, locks, statement statistics | The same views at database scope, and the VIEW DATABASE STATE permission |
| Redshift, TiDB | Activity only | The adapter's activity query and sufficient permissions |
| MongoDB | Current operations | `currentOp` and sufficient permissions |
| Redis | Connected clients | `CLIENT LIST`, and `CLIENT KILL` to stop one |
| CockroachDB, PlanetScale, SQLite, Turso | No dashboard metrics | None |

PostgreSQL metrics use `pg_stat_activity`, `pg_locks`, `pg_stat_database`, WAL functions, and replication statistics. Replication lag appears only when the query returns a value. Cache hit rate needs recorded block reads or hits, and rates need successive counter samples.

PostgreSQL-family statement statistics need the `pg_stat_statements` extension. masume checks the extension catalog when the session opens, except for engine variants without that catalog. The server must load the extension and permit access to its statistics. The panel contains call counts, mean execution time, total execution time, and returned rows.

ClickHouse statement statistics come from `system.query_log`, and the server must be configured to write it. masume reads that table once when the session opens and shows the panel where the read succeeds. One row of the panel is one shape of statement, grouped by the hash the server normalizes it to.

SQL Server statement statistics come from `sys.dm_exec_query_stats`. They need the VIEW SERVER STATE permission. masume reads that view once when the session opens and shows the panel where the read succeeds. SQL Server load metrics do not include the PostgreSQL counters, cache hit rate, or replication lag.

MySQL-family load metrics do not include PostgreSQL counters, cache hit rate, replication lag, or statement statistics. MariaDB's `performance_schema.global_status` table needs version 10.5.2 or later and the Performance Schema. The adapter does not read MySQL lock relationships.

Static capability flags do not check every statistics view, extension setting, or permission; missing dependencies can produce dashboard errors.

## Read-only access

The client rejects recognized writes for read-only profiles. PostgreSQL-family sessions also ask for server read-only mode. MySQL and MariaDB use `SET SESSION TRANSACTION READ ONLY`. SQLite opens existing files with `mode=ro`. ClickHouse uses `SET readonly = 2`, which rejects a write but still takes the settings the driver sends. MongoDB, SQL Server, Azure SQL Database and Redis have client-only checks.

Turso takes no read-only connection. A read-only Turso profile has a client-only check.

TiDB does not enforce the session read-only statement. An explicit TiDB profile with `mode = "read-only"` fails during connection.

MCP read-only access is separate from profile mode. For TiDB, MCP keeps the profile mode and applies its client access policy. A writable TiDB profile can open with MCP read-only access, but an explicitly read-only TiDB profile still fails.

A classified read can still have side effects. Database permissions remain separate from client access checks.

## Default port and TLS

| Engine | Port | Default `sslmode` |
| --- | --- | --- |
| aurora-mysql | 3306 | `prefer` |
| aurora-postgres | 5432 | `prefer` |
| clickhouse | 9000 | unset; no TLS |
| cockroach | 26257 | `prefer` |
| mariadb | 3306 | `prefer` |
| mongodb | 27017 | unset; no TLS |
| mysql | 3306 | `prefer` |
| neon | 5432 | `require` |
| planetscale | 3306 | `require` |
| postgres | 5432 | `prefer` |
| redis | 6379 | unset; no TLS |
| redshift | 5439 | `require` |
| sqlite | none | none |
| azure-sql | 1433 | `require` |
| sqlserver | 1433 | unset; the login only |
| supabase | 5432 | `require` |
| tidb | 4000 | `prefer` |
| timescale | 5432 | `prefer` |
| turso | 443 | `require` |

Most PostgreSQL-family and MySQL-family engines with an unset `sslmode` behave as `prefer`.

For PostgreSQL-family and MySQL-family engines, `allow` and `prefer` permit unencrypted fallback. `require` needs TLS without certificate verification. `verify-ca` checks the certificate chain. `verify-full` also checks the host name. Verification uses the system trust roots.

ClickHouse differs. The native protocol does not negotiate. Unset, `allow`, and `prefer` connect without encryption. `require` encrypts without certificate verification. `verify-ca` and `verify-full` verify it. An encrypted ClickHouse listens on a port of its own. The default is 9440.

SQL Server differs. Azure SQL Database takes an encrypted session only, and its default is `require`. On SQL Server, unset, `allow`, and `prefer` encrypt the login. The rest of the session goes unencrypted. `disable` encrypts nothing. `require` encrypts the whole session without certificate verification. `verify-ca` and `verify-full` verify it.

Redis differs. Unset or `disable` uses no TLS. Every other mode uses TLS. A `rediss://` target sets `verify-full`.

MongoDB differs. Unset or `disable` uses no TLS. Explicit `allow`, `prefer`, and `require` need TLS. They use no certificate verification. They have no unencrypted fallback. MongoDB also supports `verify-ca` and `verify-full`.

Turso differs. `disable` opens `ws://`, and every other mode opens `wss://`. A hosted database accepts TLS only.

See [profiles](configuration.md#profiles) for the other profile keys. Connection targets do not forward native URL options. See [connection targets](usage.md#connection-targets).

## MongoDB

A query tab accepts a subset of MongoDB shell calls.

```js
db.orders.find({status: "new"}).sort({total: -1})
```

The parser accepts extended JSON, unquoted document keys, single-quoted strings, comments, trailing commas, and regular expressions such as `/pattern/i`.

Supported value helpers are `ObjectId`, `ISODate`, `Date`, `NumberLong`, `NumberInt`, `NumberDouble`, `NumberDecimal`, and `UUID`. General JavaScript variables, loops, and function evaluation are unsupported.

`db.getSiblingDB("name")` selects a database for a statement. `db.getCollection("name")` selects a collection with a quoted name. Statements end at a top-level semicolon or newline. Open documents and continuation lines starting with `.` can span lines.

### Supported calls

The adapter executes these database calls:

| Call | Supported arguments |
| --- | --- |
| `runCommand`, `adminCommand` | One command document. `adminCommand` uses the admin database |
| `getCollectionNames` | No filter or options |
| `createCollection` | Collection name only |
| `dropDatabase` | No options |

The adapter executes these collection calls:

| Call | Supported arguments and behavior |
| --- | --- |
| `find`, `findOne` | Filter, optional projection, and the supported chains below |
| `aggregate` | Pipeline array only |
| `countDocuments`, `count` | Filter only. Both use `CountDocuments` |
| `estimatedDocumentCount` | No options |
| `distinct` | Field name and optional filter |
| `getIndexes` | No options |
| `insertOne`, `insertMany`, `insert` | Document or document array. The value shape selects single or multiple insertion |
| `updateOne`, `updateMany`, `replaceOne` | Filter and update or replacement only |
| `deleteOne`, `deleteMany` | Filter only |
| `remove` | Filter and optional boolean or `{justOne: true}` |
| `findOneAndUpdate`, `findOneAndReplace` | Filter and update or replacement. Returns the original document |
| `findOneAndDelete` | Filter. Returns the removed document |
| `createIndex` | Key document and optional `name`, `unique`, and `sparse` settings |
| `dropIndex` | Index name |
| `drop` | No options |

Find chains apply `sort`, `projection`, `limit`, and `skip`. The parser accepts `pretty`, `toArray`, `batchSize`, `hint`, `allowDiskUse`, and `collation`, but execution ignores those chains. Other find chains fail.

Most methods ignore extra arguments and chains instead of rejecting them; unsupported options are not forwarded. Update options such as `upsert`, find-and-update options such as `returnDocument`, and aggregate options such as `allowDiskUse` have no effect. Index options other than `name`, `unique`, and `sparse` are ignored.

Command documents passed to `runCommand` or `adminCommand` reach the server as documents, and their replies remain command documents. Cursor replies are not automatically exhausted. Client access checks still apply.

Completion and syntax highlighting include more methods than execution supports. Calls such as `bulkWrite`, `update`, `save`, and `createIndexes` are not implemented as shell methods.

Plans support `find`, `aggregate`, `count`, `countDocuments`, and `distinct`. Shell `.explain()` chaining is unsupported. Use the interface plan action or headless `--explain` instead.

### Documents and access

Collection metadata samples up to 100 documents. Result columns come from the returned documents. Streaming columns come from the first batch. A column with different non-null types is `mixed`. Staged edits use the row's `_id`.

Streaming omits fields first encountered after the first batch and reports an error. Headless output can already be incomplete at that point. See [headless streaming](headless.md#memory-and-streaming).

Set a user only for authenticated MongoDB connections. With a user, the adapter supplies credentials; without one, it supplies none. The client has no authentication settings beyond the profile fields. Native URL query options do not provide them.

## SQL Server

masume connects to SQL Server 2016 and later, and to Azure SQL Database as the `azure-sql` engine. Both use TDS. The connection opens one database, and the relations of that database appear under their schemas. `dbo` is the default schema of most logins.

A page after the first uses `OFFSET` and `FETCH NEXT`, which the server reads after a sort only. A page of a read with no sort of its own gets `ORDER BY (SELECT NULL)`, which keeps the rows in the order the server returns them; that order is not guaranteed between pages. Sort a read whose pages must line up. The first page takes no window, and the client caps the rows as it reads them. A read the server will not sort, such as `select next value for`, still runs. The generated `SELECT` of the object menu caps its rows with `TOP` instead.

The statement separator is the semicolon. `GO` is a separator of the command-line tools, not of the server, and a buffer that holds `GO` fails. The server also takes `CREATE VIEW`, `CREATE FUNCTION`, `CREATE PROCEDURE`, and `CREATE TRIGGER` as the first statement of a batch only. Each one needs a tab or a cell of its own.

An identity column and a computed column reject a value from the client, and the row form leaves them out. A rename goes through `sp_rename`. The object menu writes it.

A write with an `OUTPUT` clause returns rows, and masume shows them in place of a count. The server rejects `OUTPUT` without `INTO` on a table that has an enabled trigger; such a write needs `OUTPUT INTO` or a disabled trigger.

Statement diagnostics use `sys.dm_exec_describe_first_result_set`, which compiles the statement and returns the fault as a row. A statement with a `:name` parameter is checked once the parameters have values.

An estimated plan comes from `SET SHOWPLAN_ALL ON`. A measured plan comes from `SET STATISTICS PROFILE ON`. Both carry estimated and counted rows per step, but neither carries a time per step.

The dashboard stops another session with `KILL`, which ends the session and its transaction. T-SQL has no statement that stops one statement of another session. The activity list has no cancel, and the interface also has no cancel for a statement of this connection; `Ctrl+X` is not shown on a SQL Server connection. Set `statement_timeout_ms` to bound a statement instead. The client stops such a statement through the driver and opens the connection again afterwards. A stopped statement leaves the connection unusable, a transaction is lost with that connection, and the client says so.

The server has no read-only session, and a read-only profile is enforced by this client alone. It also has no materialized view; an indexed view appears as a view.

Azure SQL Database holds one database per connection, and `USE` does not reach another one. Its dynamic management views are scoped to that database and need the VIEW DATABASE STATE permission. The server load panel is hidden there: `@@max_connections` is a value of a whole instance, which the service does not hold.

## Redis

Redis has no SQL. A query tab takes Redis commands, one per line:

```
SET user:1 "a name"
GET user:1
```

A command ends at the line break, because a key can hold a semicolon. masume holds the Redis command set, marks a command it does not know, and rates each one as a read, a write, a delete, a sweep of the whole database, or a script. A script is rated at the highest risk, because its body can call any command.

The tree is built from a scan of the key space, so it shows the keys of the last scan. A key written after that scan appears after the next one. A prefix of the key names stands for a relation, and the four columns of every prefix are the key, its type, its time to live, and its value.

The connection opens one numbered database, and the profile holds that number. A password without a user is accepted: the server setting is `requirepass`, which has no user.

## Turso

masume connects to Turso and to any libSQL server over the websocket protocol of the server. The database is the host name, and the auth token is the password. A `libsql://` target carries the token as its `authToken` parameter.

The SQL is SQLite, and the catalog is read through the same `pragma` functions. The HTTP protocol of the same server runs each statement on a connection of its own, which ends a transaction before the next statement arrives, so masume opens the websocket protocol instead.

The server sends no column type. A result column takes the type of its first value: `text`, `integer`, `real`, or `blob`. A local file of libSQL opens as a SQLite profile.

## ClickHouse

masume connects to ClickHouse over its native protocol; the default port is 9000. A ClickHouse database is a schema, and the connected one is the default. The object tree draws that database alone.

The protocol takes one statement per call. A buffer of several statements runs one at a time and answers with the result of the last one. Such a buffer binds no values. A `:name` parameter belongs to a buffer that holds one statement.

The server holds no user transaction. The server rejects begin, commit, and rollback. Staged changes are applied one after another. A change that fails leaves the changes before it in place, and the report names the change that failed.

A staged edit of a row is written as `ALTER TABLE … UPDATE`. This is a mutation of the table. The session sets `mutations_sync = 1`, the server finishes the mutation before it answers, and the grid reads the row back as it now stands. A staged delete is written as `DELETE FROM`. A write reports no row count. The server counts the rows of a mutation nowhere the client can read.

The server has no foreign key and keeps the constraints of a table in the statement that made it. The relation panes show neither, and the diagram draws no relationship. The sorting key of a table appears as its primary index, and the data-skipping indexes appear beside it. A materialized view appears as one, and the table that keeps its rows is hidden.

A new table needs an engine. The object menu writes `ENGINE = MergeTree` and uses the order the table keeps its rows in. An import writes the same and orders by the first column of the file. The server numbers no column of its own, so a new table carries a plain `UInt64` key.

`EXPLAIN` returns the plan of a read, one step per line. No plan carries a measurement of a run, so the pane shows the estimate alone. Statement diagnostics use `EXPLAIN PLAN`, which reads every name of a read without running it. A statement that is not a read is not checked.

`KILL QUERY` stops one statement, and both stop actions of the activity list use it. A statement of this server belongs to no session that can be closed. The server names a statement with a text of its own, so the list of this client counts its rows instead. A stop acts on the row that was listed.