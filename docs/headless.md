# Headless mode

`masume run`, `masume nb run`, `masume dump`, and `masume restore` open a connection, do the work, and write to stdout. They use profiles, connection commands, timeouts, and read-only checks. They have no write confirmation, write plan, or undo.

```sh
masume run -p shop-prod -f json 'select count(*) from orders'
```

A target, `--profile`, and `$DATABASE_URL` follow the same rules as in the client. See [connection targets](usage.md#connection-targets).

## Arguments

```text
masume run [TARGET] STATEMENT
masume run [TARGET] -e FILE
```

| Argument | Meaning |
| --- | --- |
| `TARGET` | Connection URL, keyword connection string, or existing SQLite file. Without a target, use `--profile` or `$DATABASE_URL` |
| `-p`, `--profile NAME` | User or project profile |
| `-e`, `--execute FILE` | Statement file. `-` reads stdin |
| `-f`, `--format FORMAT` | `table` by default, or `csv`, `json`, or `markdown` |
| `-l`, `--limit ROWS` | Maximum output rows per statement, above zero. Also applies to a statement with its own limit |
| `--param NAME=VALUE` | String value for `:NAME`. Repeat for each parameter |
| `--explain` | JSON plan. Reads are measured where the engine supports it |

When both the target and the statement are positional, the target comes first. A target and `--profile` cannot be used together.

`-h` prints the help and ignores the positional arguments. An unknown option wins over `-h` and exits with code 2.

With several `-e` options, only the last file is used. SQL that starts with `--` must come from a file or stdin; the argument parser has no `--` separator.

## Writes and access

The profile `autocommit` setting does not apply. A transaction needs an explicit `BEGIN`, `COMMIT`, or `ROLLBACK`.

A read-only profile rejects recognized writes before execution. A TiDB profile with `mode = "read-only"` fails to open and exits with `2`. See [read-only access](engines.md#read-only-access).

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success, also when `--limit` or the default read cap cut the output |
| `1` | Statement, parameter, plan, or output failure; an empty batch; or incomplete write results without `--limit` |
| `2` | Argument, input file, password, connection command, or connection failure |
| `3` | Write rejected by the read-only profile |

A write can succeed and still exit with `1`: when output fails after the write, or when the returned rows exceed the default cap. With an explicit `--limit`, truncated output exits with `0`, write results included.

Diagnostics and truncation notices go to stderr.

## Formats

`table` separates columns with spaces and pads each column to its widest cell. `markdown` writes a pipe table without padding. Both formats replace newlines inside cells with spaces.

```text
id  total_cents  status
--  -----------  ---------
1   4990         paid
2   1200         paid
3   99           cancelled
```

CSV uses commas, a header, LF line endings, and quoting where needed. Null and the empty string both become an empty field. Newlines stay inside quoted fields.

CSV formula guarding is on: a non-numeric field that starts with `=`, `+`, `-`, `@`, tab, or carriage return gets a leading apostrophe. Plain numbers are unchanged. Headless runs have no flags for these CSV settings.

JSON output is an array of records. A result without rows is an empty array. CSV writes the column header even when there are no rows. An empty MongoDB result has no discovered columns.

| JSON value | Form |
| --- | --- |
| Top-level record keys | Sorted by byte value, so uppercase names come first. Repeated column names get suffixes such as `_2` |
| Native numbers and booleans | JSON numbers and booleans |
| Driver-specific decimals | Text, with the driver's decimal precision |
| Null | `null` |
| Arrays and document values | Embedded JSON when the value has a recognized structure |
| Newline inside text | The escape `\n` |
| Other driver values | Formatted text |

MongoDB columns come from the document fields. Object IDs become hexadecimal strings. Decimal values become text.

A statement without a result set writes nothing to stdout. Its command and affected row count go to stderr, for example `UPDATE 1`.

## Parameters

Parameter names are case-insensitive. Every CLI parameter value is a string: `42`, `true`, and `null` stay strings. The database can convert them as the statement needs.

A repeated parameter takes the last value.

SQL runs use driver parameters. MongoDB statements get the values inline, as quoted literals. `--explain` also inlines them, quoted for the engine dialect.

```sh
masume run -p shop -f csv \
  --param day=2026-09-02 --param status=paid \
  'select id from orders where created_at::date = :day and status = :status'
```

Write typed MongoDB values in the statement, for example `ObjectId("507f1f77bcf86cd799439011")` or `{quantity: 42}`.

## Plans

`--explain` always writes JSON, whatever `--format` is. When the engine supports measured plans, a read runs to be measured. That run can take locks and call functions with side effects.

A recognized write gets an estimated plan only, if the engine supports one for that statement. The read-only check still runs before planning.

```sh
masume run -p shop --explain --param s=paid \
  'select * from orders where status = :s' \
  | jq '[.nodes[].selfMs // 0] | add'
```

The top-level fields are `analyzed`, `summary`, and `nodes`. `analyzed` is `true` for a measured plan.

Each node contains `depth`, `label`, `detail`, `estimatedRows`, `actualRows`, `selfMs`, `shareOfTotal`, `slowest`, and `misestimated`. Missing estimates or measurements are `null`.

Explain one statement per run. Several statements produce consecutive JSON objects with no enclosing array. `--format json` rejects the batch first.

## Passwords

A `password` value in a TOML profile is ignored, with a warning. A password in a connection target still works.

| Authentication | Headless source |
| --- | --- |
| `auth = "password"` | `password_env`, or a password in the connection target |
| `auth = "command"` | First stdout line of `password_command` |
| `auth = "secret"` | First stdout line of the `[secret.NAME]` command |
| `auth = "keyring"` | Existing keyring entry for the profile name |
| `auth = "prompt"` | Unsupported when the connection needs a prompt |

Commands have no stdin and a 30-second timeout. A missing password, a missing keyring entry, or a password error exits with `2`. SQLite needs no password. MongoDB can omit the user when authentication is disabled.

## Several statements

A statement argument or file can contain several statements. Headless runs execute them in order on one session and stop at the first failure; later statements do not run.

A batch is not atomic by itself. Earlier writes can stay committed after a later failure. On SQL engines that support transactions, the batch can contain its own transaction statements. Each invocation opens its own session.

```sql
BEGIN;
UPDATE orders SET status = 'paid' WHERE id = 42;
INSERT INTO order_events (order_id, event) VALUES (42, 'paid');
COMMIT;
```

```sh
masume run -p shop -e payment.sql
```

Engine restrictions on DDL and on nontransactional tables still apply.

`--format json` rejects several statements before running any, and exits with `1`. Other formats write the results one after another, with a separate header for each tabular result. Empty input exits with `1`, and so does SQL that contains only comments.

## Row limits

Without `--limit`, a read with its own recognized limit returns every row up to that limit. Other reads return at most `page_size` rows, 200 by default.

Recognized limits are SQL `LIMIT`, `FETCH FIRST`, MongoDB `.limit(n)`, and `findOne()`. A `$limit` stage alone in a MongoDB pipeline does not select the streaming path.

```sh
masume run -p shop 'select * from orders limit 250'
masume run -p shop 'select * from orders'
masume run -p shop --limit 100 'select * from orders limit 250'
```

The first command returns up to 250 rows. The second returns one profile page. The third returns at most 100 rows, despite the SQL limit.

`--limit` applies to each statement separately and turns off batch streaming. When more rows exist, both the default read cap and an explicit cap exit with `0` and write a notice.

### Memory and streaming

Batch streaming applies only to a recognized read with its own limit and no `--limit` flag. The batch size is `page_size`. CSV and JSON write each batch as it arrives.

`table` and `markdown` output buffer the whole result before writing. Every other read, and every write, fetches its rows in one capped read before output.

When MongoDB output streams, the columns are the fields of the first batch. A field that first appears in a later batch is left out of the output. The driver reports that field as an error after the cursor is read.

That MongoDB run exits with `1`. By then CSV output can contain incomplete records, JSON can lack its closing bracket, and buffered `table` and `markdown` output is never written. A cursor error or an output error can also leave partial output.

### Write results

A write runs once and never streams. A write such as `UPDATE ... RETURNING` can succeed and return more rows than the cap.

Without `--limit`, truncated write results exit with `1`. With an explicit `--limit`, the same truncation exits with `0`. A `--limit` large enough for all returned rows gets the complete result in one read. The output cap does not limit affected rows.

### Report profile

`page_size` is the default cap for the interface and for headless runs:

```toml
[profile.shop-report]
engine       = "postgres"
host         = "db.internal"
database     = "shop"
user         = "reader"
auth         = "password"
password_env = "SHOP_REPORT_PASSWORD"
mode         = "read-only"
page_size    = 50000
```

## Notebooks

`masume nb run FILE` runs a notebook file. Profiles, timeouts, and the read-only check work as in `masume run`. A write cell needs `--allow-writes`.

```sh
masume nb run reports/revenue-review.masume.md -p shop --param day=2026-09-01 -f markdown
```

`--only CELL` runs one cell by id. `--explain` writes a JSON plan for every statement and runs none of them. `-f markdown` writes the whole notebook with the rows of each cell.

| Code | Meaning |
| --- | --- |
| `0` | Success |
| `1` | A cell failed, or a write cell ran without `--allow-writes` |
| `2` | Argument, input file, password, or connection failure |
| `3` | A cell writes and the profile is read-only |

See the [notebook guide](notebooks.md#headless-mode).

## Dump and restore

`masume dump` writes schema and data to a SQL file, and `masume restore` runs that file. Both use the profiles, connection commands, and timeouts above.

```sh
masume dump -p shop --schema public --drop shop.sql
masume dump -p shop - | gzip > shop.sql.gz
masume restore -p shop-staging shop.sql
```

```text
masume dump [TARGET] FILE
masume restore [TARGET] FILE
```

| Argument | Meaning |
| --- | --- |
| `FILE` | Dump file. `-` is stdout for a dump and stdin for a restore |
| `-p`, `--profile NAME` | User or project profile |
| `-s`, `--schema NAME` | Schema to dump. Default: the default schema of the connection |
| `-t`, `--table NAME` | One table, as `name` or `schema.name`. Repeat for more tables. With `-t`, the dump has no types, sequences, functions, views, or triggers |
| `-c`, `--content WHAT` | `schema and rows` by default, or `schema only` or `rows only` |
| `--drop` | Add a `DROP … IF EXISTS` for each object the dump creates |

A dump contains the types, sequences, and functions of the schema, then its tables with their rows, the views over them, and its triggers. Each table comes after the tables its foreign keys reference. Roles, grants, and owners are left out. A dump only reads, so a read-only profile can dump.

Neither command works on an engine that reports no definitions, such as MongoDB.

A restore runs each statement on its own, in file order, with no wrapping transaction. At the first failure it stops, reports the failed statement and the number of statements run before it, and exits with code `1`. A read-only profile exits with code `3` before any statement is sent.

| Code | `dump` | `restore` |
| --- | --- | --- |
| `0` | Dump written | Every statement ran |
| `1` | A read, a definition, or the file failed | A statement failed; earlier statements stay applied |
| `2` | Argument, password, or connection failure | Argument, input file, password, or connection failure |
| `3` | | Read-only profile |

See [dump and restore](usage.md#dump-and-restore) for the file layout.

## Config and history

`masume run` reads the config and project files but writes neither. A run without a config file does not create the starter file; the terminal client, `masume --detect` and `masume --mcp` create it.

Headless runs do not record query history in `history.sqlite`.
