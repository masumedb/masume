# Headless mode

`masume run`, `masume nb run`, `masume dump`, and `masume restore` open a connection. They do the work. They write to stdout. They use profiles, connection commands, timeouts, and read-only checks. They have no write confirmation. They have no write plan. They have no undo.

```sh
masume run -p shop-prod -f json 'select count(*) from orders'
```

A target, `--profile`, and `$DATABASE_URL` follow the same rules as the client. See [connection targets](usage.md#connection-targets).

## Arguments

```text
masume run [TARGET] STATEMENT
masume run [TARGET] -e FILE
```

| Argument | Meaning |
| --- | --- |
| `TARGET` | A connection URL, keyword connection string, or existing SQLite file. Without a target, use `--profile` or `$DATABASE_URL` |
| `-p`, `--profile NAME` | A user or project profile |
| `-e`, `--execute FILE` | The statement file. A single `-` reads stdin |
| `-f`, `--format FORMAT` | `table` by default, or `csv`, `json`, or `markdown` |
| `-l`, `--limit ROWS` | A positive output cap per statement, including statements with their own limit |
| `--param NAME=VALUE` | A string value for `:NAME`. Repeat for each parameter |
| `--explain` | A JSON plan, with execution measurements for reads it can measure |

The target precedes the statement when both are positional. A target and `--profile` cannot appear together.

`-h` takes priority over the positional arguments. It prints the help. An unknown option takes priority over `-h`. It exits with code 2.

Only one `-e` file is used. Repeated `-e` options use the last file. SQL that starts with `--` must come from a file or stdin. The argument parser has no `--` separator.

## Writes and access

The profile `autocommit` setting does not start a transaction. SQL transactions need explicit `BEGIN`, `COMMIT`, or `ROLLBACK`.

A read-only profile rejects recognized writes before execution. A TiDB profile with `mode = "read-only"` fails to open. It exits with `2`. See [read-only access](engines.md#read-only-access).

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | The run completed, possibly with an intentional cap or a default read cap |
| `1` | Statement, parameter, plan, or output failure; an empty batch; or incomplete write results without `--limit` |
| `2` | Argument, input file, password, connection command, or connection failure |
| `3` | The profile's read-only check rejected a write |

Exit `1` can follow a successful write. Output can fail after the write. Returned rows can exceed the default cap. An explicit `--limit` makes truncation return `0`. This includes truncation of write results.

Diagnostics and truncation notices go to stderr.

## Formats

`table` uses spaces between columns. It measures each column's widest cell. `markdown` writes a pipe-separated table. It does not measure column widths. Both formats replace newlines inside cells with spaces.

```text
id  total_cents  status
--  -----------  ---------
1   4990         paid
2   1200         paid
3   99           cancelled
```

CSV uses commas, a header, LF endings, and quoting as needed. Null and empty string values both produce empty fields. Newlines remain inside quoted fields.

CSV formula guarding is on. Non-numeric fields receive a leading apostrophe. This applies when they start with `=`, `+`, `-`, `@`, tab, or carriage return. Plain numbers stay unchanged. Headless runs have no flags for these CSV settings.

JSON output is an array of records. A result without rows produces an empty array. CSV writes the column header even when there are no rows. An empty MongoDB result has no discovered columns.

| JSON value | Form |
| --- | --- |
| Top-level record keys | Sorted by byte value, so uppercase names come first; repeated column names receive suffixes such as `_2` |
| Native numbers and booleans | JSON numbers and booleans |
| Driver-specific decimals | Text, with the driver's decimal precision |
| Null | `null` |
| Arrays and document values | Embedded JSON when the value has a recognized structure |
| Newline inside text | The escape `\n` |
| Other driver values | Formatted text |

MongoDB output contains columns from documents. Object IDs become hexadecimal strings. Decimal values become text.

A statement without a result set writes its command to stderr. It writes the affected count. An example is `UPDATE 1`. Such statements write nothing to stdout.

## Parameters

Parameter names are case-insensitive. Every CLI parameter value is a string. `42`, `true`, and `null` stay strings. The database can convert them as the statement needs.

Repeated parameter names use the last supplied value.

SQL execution uses driver parameters. MongoDB parameters become quoted inline values. Explain parameters also become quoted inline values through the engine dialect.

```sh
masume run -p shop -f csv \
  --param day=2026-09-02 --param status=paid \
  'select id from orders where created_at::date = :day and status = :status'
```

Typed MongoDB values belong in the statement. Examples are `ObjectId("507f1f77bcf86cd799439011")` and `{quantity: 42}`.

## Plans

`--explain` always writes JSON. `--format` does not change this. A read executes for measurement when the engine supports measured plans. Reads can take locks. They can call functions with side effects.

Recognized writes receive an estimated plan only. This happens when the engine supports that statement. Read-only checks still apply before planning.

```sh
masume run -p shop --explain --param s=paid \
  'select * from orders where status = :s' \
  | jq '[.nodes[].selfMs // 0] | add'
```

The top-level fields are `analyzed`, `summary`, and `nodes`. `analyzed` is `true` for a measured plan.

Each node contains `depth`, `label`, `detail`, `estimatedRows`, `actualRows`, `selfMs`, `shareOfTotal`, `slowest`, and `misestimated`. Missing estimates or measurements are `null`.

Use one statement per explain run. Several statements produce consecutive JSON objects. There is no enclosing array. `--format json` rejects the batch first.

## Passwords

A `password` value in a TOML profile is ignored. It produces a warning. Passwords supplied in connection targets remain supported.

| Authentication | Headless source |
| --- | --- |
| `auth = "password"` | `password_env`, or a password supplied in the connection target |
| `auth = "command"` | The first stdout line from `password_command` |
| `auth = "secret"` | The first stdout line from the configured `[secret.NAME]` command |
| `auth = "keyring"` | The existing keyring entry for the profile name |
| `auth = "prompt"` | Unsupported when the connection needs a prompt |

Commands have no stdin. They have a 30-second timeout. Missing passwords return `2`. Missing keyring entries return `2`. Password resolution errors return `2`. SQLite needs no password. MongoDB can omit the user when authentication is disabled.

## Several statements

A statement argument or file can contain several statements. Headless runs execute them in order. They run on one session. They stop at the first failure. Later statements do not run.

A batch is not automatically atomic. Earlier writes can remain committed after a later failure. Supported SQL engines accept explicit transaction statements within the batch. Separate invocations share no session.

```sql
BEGIN;
UPDATE orders SET status = 'paid' WHERE id = 42;
INSERT INTO order_events (order_id, event) VALUES (42, 'paid');
COMMIT;
```

```sh
masume run -p shop -e payment.sql
```

Engine DDL restrictions still apply. Nontransactional table restrictions still apply.

`--format json` rejects several statements. It exits with `1`. It rejects before executing any statement. Other formats write consecutive results. Each tabular result has a separate header. Empty input returns `1`. SQL containing only comments returns `1`.

## Row limits

Without `--limit`, a read with a recognized limit returns all rows within that limit. Other reads return at most `page_size` rows. The default is 200.

Recognized limits include SQL `LIMIT`. They include `FETCH FIRST`. They include MongoDB `.limit(n)`. They include `findOne()`. A MongoDB pipeline's `$limit` stage alone does not select the streaming path.

```sh
masume run -p shop 'select * from orders limit 250'
masume run -p shop 'select * from orders'
masume run -p shop --limit 100 'select * from orders limit 250'
```

The first command returns up to 250 rows. The second returns one profile page. The third returns at most 100 rows, despite the SQL limit.

`--limit` applies separately to each statement. It disables batch streaming. A default read cap returns `0` and a notice when more rows exist. An explicit cap also returns `0` and a notice.

### Memory and streaming

Batch streaming applies only to recognized reads. Such reads have their own limit. They have no CLI `--limit`. The batch size is `page_size`. CSV writes each batch as the batch arrives. JSON writes each batch as it arrives.

Table and Markdown output buffer the whole returned result before writing. Other reads use one capped read before output. All writes use one capped read before output.

MongoDB streaming columns are the fields in the first batch. Fields first encountered later are omitted from the output. The driver reports those fields as an error after reading the cursor.

Such a MongoDB run returns `1`. CSV can already contain incomplete records. JSON can lack its closing bracket. Buffered table and Markdown output remains unwritten. Cursor errors can leave partial output. Output errors can also leave it.

### Write results

A write executes once. It never uses the streaming path. A write such as `UPDATE ... RETURNING` can succeed. It can return more rows than the cap.

Without `--limit`, truncated write results return `1`. An explicit `--limit` makes the same truncation return `0`. A larger `--limit` can hold the complete returned result in one read. The output cap does not limit affected rows.

### Report profile

`page_size` is the default cap for the interface. It is the default cap for headless runs:

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

`masume nb run FILE` runs a notebook file. The profile is the same as `masume run`. The timeouts are the same. The read-only check is the same. A write cell needs `--allow-writes`.

```sh
masume nb run reports/revenue-review.masume.md -p shop --param day=2026-09-01 -f markdown
```

`--only CELL` runs one cell by id. `--explain` writes a JSON plan of every statement. It runs none of them. `markdown` writes the whole notebook with the rows of every cell.

| Code | Meaning |
| --- | --- |
| `0` | The run completed |
| `1` | A cell failed, or a write cell ran without `--allow-writes` |
| `2` | Argument, input file, password, or connection failure |
| `3` | The profile is read-only and a cell writes |

See the [notebook guide](notebooks.md#headless-mode).

## Dump and restore

`masume dump` dumps schema and data to a SQL file. `masume restore` restores that dump. Both use the profiles above. Both use the connection commands and timeouts above.

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
| `FILE` | The dump file. A single `-` writes stdout, and a restore reads stdin |
| `-p`, `--profile NAME` | A user or project profile |
| `-s`, `--schema NAME` | The schema to dump. Without it, the default schema of the connection |
| `-t`, `--table NAME` | One table, as `name` or `schema.name`. Repeat for more, and the objects are left out |
| `-c`, `--content WHAT` | `schema and rows` by default, or `schema only` or `rows only` |
| `--drop` | Write a `DROP … IF EXISTS` for everything the dump makes |

A dump holds the types, sequences, and functions of the schema. Then it holds its tables and their rows. Then it holds the views over them. Then it holds its triggers. Every table stands after the tables its foreign keys name. Roles, grants, and owners are omitted. A dump only reads. A read-only profile can dump.

Neither command runs on an engine that reports no definitions. MongoDB is an example.

A restore runs each statement on its own. It runs in file order. There is no wrapping transaction. It stops at the first failure. It reports the statement that failed. It reports how many ran before it. It exits with code `1`. A read-only profile exits with code `3`. It sends nothing.

| Code | `dump` | `restore` |
| --- | --- | --- |
| `0` | The dump was written | Every statement ran |
| `1` | A read, a definition, or the file failed | A statement failed; the statements before it stand |
| `2` | Argument, password, or connection failure | Argument, input file, password, or connection failure |
| `3` | | The profile is read-only |

See [dump and restore](usage.md#dump-and-restore) for the file layout.

## Config and history

`masume run` reads the config and project files. It writes neither file. A run without a config file does not create the starter file. The terminal client can create that file. `masume --detect` can create it.

Headless runs do not record query history in `history.sqlite`.