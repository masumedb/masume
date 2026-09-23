# User guide

This guide covers the interactive terminal client. The [key reference](keys.md) lists every default binding and its focus scope.

## First connection

`masume` opens the connection picker. Up and Down select a profile, and Enter connects.

The filter field is above the list, and `/` focuses it. The filter matches the name, environment, engine, and target. Up, Down, and Enter still work while the field has focus. Esc leaves the field and keeps the filter.

`n` opens a new connection form. `e` edits the selected profile. In the form, `Ctrl+T` tests the connection and `Ctrl+S` saves the profile. The `ssh tunnel` toggle shows the SSH fields. See [SSH tunnel](configuration.md#ssh-tunnel). The `tls files` toggle shows the certificate fields, and `Enter` on a file path field opens a file picker. The line under the fields shows what the selected `auth`, `sslmode` or `confirm` value does. See [Certificate files](configuration.md#certificate-files).

An explicit target or `$DATABASE_URL` opens a connection directly. See [connection targets](#connection-targets), [container detection](#databases-in-a-container), and [passwords](configuration.md#passwords).

A connection without restored tabs starts with an empty query tab, and focus moves to the object tree. `Tab` and `Shift+Tab` move between visible panes. `Alt+P s` focuses the tree, `Alt+P e` the editor, and `Alt+P r` the result pane.

The tree shows every schema on the server, including empty schemas. A MySQL-family or ClickHouse profile with a `database` shows only that database. A MySQL-family profile without one shows every database on the server. See [profiles](configuration.md#profiles).

In the tree, the arrow keys move, expand, and collapse nodes. `Enter` on a table opens its rows, or switches to its open tab. `o` opens another tab for that table. `i` opens the Columns view. `Enter` on a column inserts its qualified name into a query editor.

`/` filters the tree. Enter keeps the filter. Esc clears the active filter entry. A filter started inside a schema searches only that schema. `f` toggles a favourite. `h` toggles system schemas. `F5` refreshes the catalog.

`m` opens the object menu, with SQL templates, table changes, imports, and an ER diagram. A SQL template goes into the editor and does not run. The entries depend on the object and the engine.

## Connection targets

A URL, a keyword connection string, or an existing SQLite file opens a connection without a saved profile. `--profile NAME` opens a user or project profile instead. A connection argument and `--profile` cannot be combined. With neither, masume opens `$DATABASE_URL`.

```sh
masume 'postgres://reader@db.internal:5432/shop?sslmode=verify-full'
masume "host=db.internal port=5432 dbname=shop user=reader sslmode=require"
masume ./notes.db
masume --profile shop-prod
```

| Form | Accepted values |
| --- | --- |
| A URL | Supported schemes: `postgres`, `postgresql`, `mysql`, `mariadb`, `cockroachdb`, `yugabytedb`, `redshift`, `sqlserver`, `mssql`, `clickhouse`, `cassandra`, `scylla`, `libsql`, `redis`, `rediss`, `mongodb` |
| A connection string | `key=value` pairs: `engine`, `host`, `hostaddr`, `port`, `dbname`, `database`, `user`, `password`, `sslmode`, `sslrootcert`, `sslcert`, `sslkey`. The default engine is `postgres` |
| A file path | A SQLite path ending in `.db`, `.db3`, `.sqlite` or `.sqlite3`. A file with another extension must have a SQLite header. `:memory:` is also accepted |

A URL without a database uses the user name on PostgreSQL-family engines, database `0` on Redis, and `admin` on MongoDB. A MySQL-family URL without a database connects to the server with no database selected. Every other engine needs the database in the URL. A URL without a host uses `127.0.0.1`.

Connection strings accept single-quoted values and backslash escapes inside quotes. `engine` takes the profile engine names. An unknown key is an error.

A URL can carry one host, credentials, a port, a database, `sslmode` (also spelled `ssl-mode` or `sslMode`), and the certificate files `sslrootcert`, `sslcert`, and `sslkey`. Other URL options, such as `authSource`, `replicaSet`, and `connect_timeout`, are ignored. `mongodb+srv` URLs are rejected.

`rediss://` connects with TLS and verifies the certificate. Other settings take the defaults of a new connection, such as `env = "dev"`, `mode = "write"`, and `page_size = 200`.

The client prompts for a missing password when the engine and the user need one. SQLite, and MongoDB without a user, need no password. The connection stays in memory until it is saved. The picker lists it under the database name or file name, with a numeric suffix for a duplicate name.

## Databases in a container

```sh
masume --detect
```

`--detect` lists running containers with `docker`, or with `podman` if Docker is not installed. Detected databases appear before saved profiles in the connection picker. Detection does not write the configuration file.

A container appears when both are true:

- The image name contains a supported database name: `postgres`, `postgis`, `pgvector`, `timescale`, `supabase`, `cockroach`, `mysql`, `percona`, `mariadb`, `tidb`, `redis`, `valkey`, or `mongo`. Detection checks the image name only, not the parent image. `supabase/postgres` is Supabase.
- The container publishes the database port.

Detection reads the user, database, and password from container environment variables:

| Engine | Variables |
| --- | --- |
| PostgreSQL | `POSTGRES_USER`, `POSTGRES_DB`, `POSTGRES_PASSWORD` |
| MySQL | `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_DATABASE`, `MYSQL_ROOT_PASSWORD` |
| MariaDB | The MySQL names with a `MARIADB_` prefix, with MySQL variables as fallbacks |
| MongoDB | `MONGO_INITDB_ROOT_USERNAME`, `MONGO_INITDB_ROOT_PASSWORD`, `MONGO_INITDB_DATABASE` |
| Redis | `REDIS_PASSWORD` |
| CockroachDB | `COCKROACH_USER`, `COCKROACH_PASSWORD`, `COCKROACH_DATABASE` |

A missing variable takes its default. A PostgreSQL container with only `POSTGRES_DB` set uses the `postgres` user.

Detected connections use `env = "dev"` and `mode = "write"`. Hosted-service images such as `supabase/postgres` use `sslmode = "prefer"` instead of the engine default `require`.

`--detect` exits with code 1 if neither tool is installed, if detection fails, or if no supported database is found. `--detect` cannot be combined with a target or `--profile`. An explicit target or profile takes priority over `$DATABASE_URL`.

## Temporary connections

A command-line or detected connection is temporary. On exit, masume asks whether to save each temporary connection that was opened:

```
┌─ save connection ─────────────────────────────┐
│ Write "shop" to the config file?              │
│                                               │
│ shop  postgres@db.internal:5432/shop          │
│                                               │
│ The password goes into the keyring, not the   │
│ file.                                         │
│                                               │
│ y save and quit · n quit without saving       │
└───────────────────────────────────────────────┘
```

`y` saves the connections and exits. `n` exits without saving. `Esc` returns to the client. masume does not ask about saved profiles, project profiles, or connections that were not opened.

The password line appears only when the password is in memory. Saving stores that password in the keyring and sets `auth = "keyring"`. Without a keyring, masume saves `auth = "prompt"` and does not store the password.

`Ctrl+N`, then `e`, opens the selected connection in the form, and `Ctrl+S` saves it as a profile. The exit prompt then skips it. Setting `auth = "prompt"` does not clear a password already in memory, and saving can still store that password in the keyring.

On a save error, the client stays open and shows the reason.

## Editing SQL

`Alt+N` opens a new query tab with editor focus. On a table tab, `Alt+E` opens the SQL of the table read in a new query tab. In a query tab, `Alt+E` writes the grid sort and server filters into the SQL.

`Ctrl+V` and the terminal paste command both paste the system clipboard.

Completion opens while typing, and `Ctrl+Space` opens it for the word under the caret. Up and Down select a candidate, and Tab accepts it. The accepted candidate replaces the whole word under the caret, including the part after the caret. Enter always inserts a newline, even with completion open. Esc closes completion. Without completion, Tab changes panes.

Completion lists the columns of the statement at the caret, the columns of the result on screen, and the catalog relations, routines and schemas. A term matches a candidate at its start or anywhere inside it. A term of three or more letters with the same first letter also matches by its letters in order: `plat` matches `placed_at`.

Shift with a movement key extends the selection. `Ctrl+A` selects all editor text. `Ctrl+C` copies and keeps the selection. A second `Ctrl+C` quits, after a confirmation for staged changes and open transactions. `Ctrl+D` formats SQL and keeps the caret on the same token. `Alt+C` toggles line comments. `Alt+[` or `Alt+]` changes indentation.

`Alt+F` opens Find. Enter applies the search. The row under the field shows the match count as the term is typed. `Alt+W` matches whole words only. `F3` and `Shift+F3` select the next or previous match. The search matches plain substrings, including matches inside longer names.

To replace, type the search term in Find and press `Ctrl+R`. Then type the replacement and press Enter. Enter replaces every match, and one editor undo reverses the whole replacement. `F4` replaces one match and selects the next one.

Search and replacement cover the whole editor buffer, including other statements, comments, and strings. A lowercase term matches either case. A term with a capital letter matches that case only. If lowercasing changes the byte length of the text, matching uses the original case.

`F8` moves to the next reported SQL problem. Editor undo is `Ctrl+Z`. Redo is `Ctrl+Shift+Z` or `Alt+Z`.

## Running SQL

`Ctrl+R` runs the selected text. With no selection, it runs the statement at the caret. A selection can contain several statements. `Alt+R` runs the entire buffer. On a table tab, either key reads the table again.

Statements run in order, each with its own result. The first failure stops the batch, and later statements do not run. A batch is not atomic. Without a transaction, writes before the failure stay.

After a run from the editor, the result pane has focus. Outside text entry, `;` and `'` select the previous and next statement result. A click on the numbered result strip also selects a result.

Named parameters such as `:customer_id` open a JSON value form before the run. `Ctrl+R` submits the values and Esc cancels. Each statement with parameters has its own form, and no statement runs until every form is complete.

A read-only profile rejects writes. Other profiles can confirm writes and show a write plan. Check the SQL and the affected rows before accepting. See [write guards](configuration.md#profiles) and [write plans](configuration.md#write-plans).

`Ctrl+X` cancels the running query and stops a running export. Cancellation depends on the engine. Amazon DocumentDB, Azure SQL Database, Cassandra, CockroachDB, MongoDB, PlanetScale, Redis, SQL Server, SQLite, and Turso cannot cancel a statement. On these engines the key is hidden, and the run spinner has the note `this engine cannot stop a running statement`. `Ctrl+C` copies or quits.

## Transactions

Transaction commands apply to the active connection, in all of its tabs. They need an engine with transaction support.

| Command | Key |
| --- | --- |
| Begin a transaction | `Ctrl+B` |
| Commit the transaction | `Ctrl+L` |
| Roll back the transaction | `Ctrl+U` |
| Toggle autocommit for this connection | `Ctrl+O` |

Begin a transaction before statements that must stay uncommitted. With autocommit off, masume begins a transaction before a run when none is active, and leaves it open until an explicit commit or rollback. The profile setting `autocommit = false` starts the connection in this mode.

Autocommit off applies to editor queries, table reads, grid writes, undo, whole-query export, and AI chat `run_query`. Imports manage their own transactions and reject an open transaction. Catalog reads and other AI chat tools do not begin a transaction. Headless and MCP runs handle transactions separately.

Changing autocommit neither commits nor rolls back an open transaction. Engine restrictions still apply, including statements that commit implicitly.

## Result views

The result strip lists the views. Outside text entry, `1` through `9` select a view by position, and comma and period select the previous and next view. View positions depend on the tab, result, and engine.

Table tabs show Data, Columns, Indexes, Constraints, DDL, and Plan where supported. Query results show Data, Fields, and Plan where supported. A statement without result columns shows Statistics, including affected rows and execution time.

Tree appears when a result contains documents or structured values. Left and Right collapse and expand nodes. Enter opens a node. `y` copies the value. `Shift+Y` copies its path. `/`, `u`, and `c` search and reset as in the grid.

In Data, the arrow keys move between cells. `v` opens the full cell value. Enter opens the full row. `g` follows a foreign key. `a` finds a column by name. `z` freezes or unfreezes the current column at the left.

A binary value longer than 32 bytes shows its first 32 bytes in hex and its size, as in `\x89504e47… (4.2MB)`. The cell viewer, the row viewer, copies, and exports use the whole value.

Foreign-key navigation filters by the selected column only. For a composite foreign key, add filters for the other key columns.

Masking hides the values in columns with sensitive names. `M` toggles masking, in the grid only. Cell viewers, row viewers, document trees, structured copies, and exports show the original values.

## Sorting and filters

A server sort or filter runs the read again. A screen filter only hides loaded rows. Neither changes the editor text. `Alt+E` writes the server sort and filters into the SQL.

| Key | Operation |
| --- | --- |
| `s` | Sort by the current column; repeat to change direction |
| `S` | Add or change a column in the multi-column sort |
| `f` | Add a server filter equal to the current cell |
| `x` | Add a server filter excluding the current cell |
| `w` | Enter a raw server predicate, replacing the previous one |
| `u` | Remove the last server filter and rerun |
| `F` | Pick the current-column values to keep on screen |
| `/` | Search loaded rows on screen |
| `c` | Clear the sort, all server filters, and all screen filters, then rerun |

`f` and `x` add filters to a stack. `w` keeps those cell filters and replaces the raw predicate. An empty `w` entry removes the raw predicate. `u` removes only the last server filter and keeps the sort. Its rerun also clears screen filters. With no server filters, `u` does nothing. `c` does not remove clauses written in the SQL.

A server filter applies to the whole statement, not only to the fetched rows. A `LIMIT` in the SQL applies after the filter, so the grid can show rows that the unfiltered statement does not return.

In the `F` card, Space toggles a value. `o` keeps only the selected value, and `a` keeps all values. Enter applies the selection. Counts cover loaded rows before screen filtering. An empty `/` entry clears the screen search.

Sorting, server filtering, and rerunning discard staged grid edits, after a confirmation. Answering No keeps the edits and the rows. The status bar shows the discarded changes. These operations also clear screen filters.

## Loading rows

`page_size` is the rows per page. The default is 200. Moving or scrolling near the last loaded row fetches more rows. The footer shows the loaded rows, the total rows when known, and the rows shown after screen filtering.

An active screen filter turns off automatic paging in both Data and Tree. `Ctrl+F` fetches another page, also under a screen filter. `t` gets a row count from the server where supported. Counting does not load the rows.

## Editing rows

Grid writes need a writable profile, a single identifiable table, loaded column metadata, and a primary key. Updates and deletes need every primary-key column in the result. Joins and other results without a single table cannot be edited. Views, materialized views, and generated columns are not editable through the grid.

| Key | Staged operation |
| --- | --- |
| `e` | Edit the current cell |
| `d` | Mark or unmark the current row for deletion |
| `D` | Duplicate the row without primary-key or generated columns |
| `n` | Enter a new row as JSON |
| `p` | Review staged SQL |
| `X` | Discard all staged changes, after a confirmation |

The cell editor shows choices for known enum and boolean columns. `Ctrl+S` stages the value. `Ctrl+L` stages NULL. `Ctrl+E` stages an empty value. `Ctrl+D` stages DEFAULT. `Ctrl+F` formats JSON in a JSON cell editor. In the card, these keys do not run their global actions.

Staging does not write to the database. The right side of the status bar shows the staged count and the review key: `● 3 staged · p to review`. In the review card, `Ctrl+Y` applies the changes, `x` discards them, and Esc returns without applying. A failed apply keeps the staged changes. On an engine without atomic staged writes, masume asks before it applies several changes one by one. A failure can leave earlier changes written.

Staged changes belong to one statement result. Return to that result before applying, or discard the staged changes.

### Undo

| Operation | Effect |
| --- | --- |
| Editor `Ctrl+Z` | Reverses text edits only; redo with `Ctrl+Shift+Z` or `Alt+Z` |
| Grid `Ctrl+Z` | Reverses staged changes only; redo with `Ctrl+Shift+Z` or `Z` |
| Global `Alt+U` | Runs the reverse SQL of a write plan, after a confirmation |

Neither editor undo nor grid undo reverses a database write that ran. `Alt+U` needs a write-plan undo. Imports and staged grid writes do not create one. Transaction rollback is separate from all three. See [write plans](configuration.md#write-plans).

## Copy and export

`y` opens the grid copy menu. The formats are cell, row JSON, result CSV, result JSON, result Markdown, result INSERT statements, and column IN clause. `C c`, `C j`, `C m`, and `C i` copy the whole loaded result directly. These sequences start with an uppercase C.

Result and column copies include every loaded row, including rows hidden by screen filters. They use the original values, ignore masks, and do not apply staged edits. Copies never fetch more rows. INSERT copies need a table target.

`Ctrl+S` opens CSV file export. `Ctrl+G` opens JSON file export. Up and Down move between fields, and Left and Right change choices. Set the path and options, then press `Ctrl+S` to write. Overwriting an existing file needs confirmation.

| Row scope | Behavior |
| --- | --- |
| `loaded so far` | Writes the loaded rows without another server read; the default |
| `every row` | Runs the read again and streams every returned row |

`every row` keeps the executed read, its server filters, sorting, and SQL limits. The second read can return newer data. A statement classified as a write allows `loaded so far` only. Screen filters and masks affect neither scope.

CSV options are delimiter, header, quoting, line endings, NULL text, and formula guarding. The defaults are comma, header on, quoting as needed, LF, empty NULL text, and formula guarding on. CSV clipboard copies use these defaults.

Formula guarding adds an apostrophe before text that starts with `=`, `+`, `-`, `@`, tab, or carriage return. Plain numbers stay unchanged.

Default CSV output cannot tell NULL from empty text. A custom `null as` value changes the NULL output, but import reads empty CSV fields as NULL even with a custom marker. See [Importing files](#importing-files).

Result JSON exports and copies keep JSON nulls and native numbers and booleans. Cell and row-menu copies use display text, and row-menu JSON is not the typed result JSON export.

## Importing files

In the object tree, select a table, press `m`, and choose Import a file. The schema menu imports into a new table. Imports support PostgreSQL-family, MySQL-family, and SQLite engines. MongoDB has no import, and read-only connections reject imports.

![The import form](../vhs/shots/18-import-form.png)

Select a file. Adjust its format and column mapping, then press `Ctrl+S` for review. `Ctrl+S` or Enter in the review starts the import. Enter on the file row opens the file picker again. Esc returns from the review to the form. During the import, the review shows a progress bar of rows written against rows in the file.

Supported extensions are `.csv`, `.tsv`, `.txt`, `.json`, `.jsonl`, and `.ndjson`.

| Setting | Meaning |
| --- | --- |
| `file` | File path. A leading `~` expands to the home directory |
| `table` | Target table, optionally qualified with a schema |
| `format` | `csv` or `json`. The default comes from the file extension |
| `delimiter` | One CSV delimiter character. The default is a comma, or a tab for `.tsv` |
| `header` | `yes` uses the first CSV row as column names. `no` creates names such as `column_1` |
| `null as` | CSV text for `NULL`, initially `\N`. Empty fields, including quoted empty fields, always become `NULL` |

File columns map to table columns with the same name, ignoring case. Other columns start as `(skip)`. A generated column cannot be a target. A new table starts with every sampled column.

Type inference reads the first 200 rows. The types are `integer`, `number`, `boolean`, `timestamp`, and `text`. Numeric text with leading zeroes stays text. Mixed values use a common type, with text as the fallback.

An import into an existing table casts each value to the type category of the target column. New PostgreSQL tables use `bigint`, `numeric`, `boolean`, `timestamptz`, and `text`. Other engines use their own dialect types.

The review checks the mappings, required columns, field counts, type conversions, and nullability. The row check runs locally. Only the column definitions of an existing table are read from the server. The review counts every rejected row and lists the first 20 errors.

The import skips rows that fail local validation. Accepted rows can still fail on server constraints, permissions, type limits, or triggers.

Imports reject an open transaction. Each import starts a transaction and tries to commit the accepted rows together. A batch has at most 1000 rows, and some engines use smaller batches. A write failure rolls back the transaction, and a rollback error appears on the card.

Rollback depends on engine and table support. MySQL-family DDL can commit implicitly. Nontransactional tables cannot roll back writes. An import that creates a table is not always all-or-nothing.

Imports keep no undo and do not use `write_plan`. The input must be UTF-8: CSV, a JSON array of objects, or newline-delimited JSON objects.

## Dump and restore

In the object tree, select a schema, press `m`, and choose Dump the schema. The form has the file, the content, and the drop option. Up and Down move between fields, and Left and Right change choices. Enter writes the file. Overwriting an existing file needs confirmation.

| Content | Meaning |
| --- | --- |
| `schema and rows` | Writes the definition of each table and one INSERT per row; the default |
| `schema only` | Writes the definitions and reads no rows |
| `rows only` | Writes the INSERT statements only |

`drop first` writes a `DROP … IF EXISTS` for every object in the dump, in reverse write order.

During a dump, the card shows a progress bar of the tables written, the table being read, and the rows written so far. During a restore, the card counts the statements that ran. Esc stops either one.

A dump contains the types, sequences, and functions of the schema, then its tables and their rows, the views on them, and its triggers. Each table comes after the tables that its foreign keys reference. Roles, grants, and owners are not written. Rows are read in batches, and a large table is never held in memory whole. The file is written next to the target, moved over the target at the end, and readable only by its owner.

The table menu dumps one table on its own, without the other objects and the views of the schema.

To run a `.sql` file, select a schema, press `m`, and choose Restore a dump. Pick the file, then press Enter. The statements run on the open connection, one at a time and in file order, like editor statements. With autocommit off, masume opens one transaction and leaves it open for an explicit commit. A failure stops the run, and the card shows the failed statement. With autocommit on, the writes of the earlier statements stay. Restore rejects an open transaction and a read-only connection. Restored statements are not added to the query history.

Dump and restore need an engine that reports object definitions. See [engine support](engines.md). `masume dump` and `masume restore` do the same in [headless mode](headless.md#dump-and-restore).

## Query plans

`Ctrl+E` shows the estimated plan. `Ctrl+Y` shows the analyzed plan where supported. Analysis runs the read to measure it. For a statement classified as a write, masume shows the estimate instead and does not run the write.

In Plan, `r` toggles the raw server plan. `y` copies the raw plan. `i` asks AI to analyze the plan when AI is enabled. See [engine support](engines.md) and [AI chat](ai.md).

## Tabs and history

`Alt+Up` and `Alt+Down` switch tabs. `Alt+1` through `Alt+9` select a tab directly. `Alt+T` names a query tab with a first-line comment, and sets the title of a notebook tab. Table and object tabs keep their object names.

`Alt+W` closes a tab. `Alt+Shift+W` reopens the last closed tab of the session. Closing a tab with staged edits prompts to apply, discard, or cancel. The last tab stays open. `Ctrl+W` closes the connection, after a confirmation when there are several tabs or staged changes.

`Ctrl+N` returns to the connection picker. `Alt+Left` and `Alt+Right` switch open connections. `Alt+S` toggles the sidebar. `Alt+D` toggles the result pane.

`Ctrl+P` saves the query under a name. On a notebook tab, `Ctrl+P` writes the notebook file. `Ctrl+Q` opens saved queries. `Ctrl+T` opens query history. Typing filters either list. Enter replaces the current query text, and `Alt+Enter` loads the query into a new tab. On a table or object tab, both keys open a query tab. Loading does not run the SQL.

`Ctrl+D` removes the selected personal saved query. Project queries are edited in the [project file](configuration.md#project-file).

On the next connect, masume restores each tab with its query text or notebook text, caret, sort, and server filters, and the active tab. Results, staged edits, transaction state, screen filters, column widths, and frozen columns are not restored. Restored query tabs do not run. Restored table and object tabs read their data when first shown.

## Notebooks

`Alt+B` opens a notebook. `Alt+O n` lists the notebooks of the project and of the user. Opening a notebook runs no cell. See the [notebook guide](notebooks.md) for cell kinds, run policy, the file format, and `masume nb run`.

## Query builder

`Alt+J` opens a query builder tab. The tab draws the tables as boxes, the joins as lines between them, and the SQL below. The builder writes SQL but does not read SQL back.

`t` adds a table from the catalog. Every table after the first is joined, and masume fills in the join condition from the foreign keys of both sides. The join card has the kind (`inner`, `left`, `right`, `full`) and the condition. Two tables with no foreign key between them open the card with an empty condition. A join left without a condition is written as a `cross join`.

Left and Right move between tables. Up and Down move through the columns and then the filters. `Space` adds the column under the cursor to the select list. `Enter` opens the column card, with the aggregate, the result column name, and the sort. A picked column without an aggregate goes into `group by`.

`w` adds a `where` condition, prefilled with the column under the cursor. `Enter` on a filter row edits the filter. `x` removes the table or filter under the cursor. Removing a table also removes every table joined through it.

`Ctrl+R` runs the statement and shows the result in the pane below. `e` copies the statement to a query tab. Later edits in that tab do not change the builder.

The mouse works everywhere the keys do. A click on a column picks it, and a click on the name of a box selects that table. A click on a join, a field, or a filter row opens its card. In a card, a click selects a row, and the `‹ ›` marks step its value.

A builder tab is saved with the other tabs. The next connect restores its tables, joins, filters, and picked columns. The columns of each table are read from the server again.

A diagram wider than the pane scrolls sideways to follow the cursor. A table taller than the pane scrolls vertically with the cursor. The wheel scrolls the pane vertically. `Shift` with the wheel scrolls the diagram sideways, and so do a trackpad and a tilting wheel. The next cursor key scrolls the pane back to the cursor.

The builder writes one flat select. For a subquery, a union, a window function, or a CTE, start in the builder and finish in the editor. MongoDB has no builder.

## Server activity

`Alt+O a` opens Server activity. The dashboard refreshes about every two seconds. It has panels for sessions, load, blocking, and slow statements.

Panels depend on engine support and server permissions. MySQL load values need access to `performance_schema`. MySQL has no blocking or slow-statement panel. The PostgreSQL slow-statement panel needs a working `pg_stat_statements` extension, checked at connection time. Slow statements are ordered by mean execution time, highest first, within the current database. Unsupported and failed optional panels are hidden.

Up and Down select a session. Enter replaces the current query text with the SQL of that session. `Alt+Enter` opens the SQL in a new query tab. Neither key runs the SQL. On a non-query tab, Enter also opens a query tab.

`x` cancels the statement of the selected session. `Ctrl+D` terminates the session and its connection. Both ask for confirmation first. Left collapses the optional panels, and Right expands them.

SQL Server cannot cancel one statement of another session, and `x` reports the action as unsupported. `Ctrl+D` runs `KILL`, which ends the session and its transaction.

ClickHouse lists running statements instead of sessions. Both stop actions run `KILL QUERY`, which ends the statement and keeps the connection. The row number is the position in the list, and ClickHouse identifies each statement by a text query ID.

MongoDB lists operations instead of SQL sessions. Both stop actions use `killOp`. Neither action terminates the connection.

## Palette operations

`Ctrl+K` opens the command palette. Type to search, select a command, and press Enter. A command needs its target, such as a result to copy.

Palette-only operations are Reload the theme files, Settings, AI provider selection, AI agent selection, Ask AI: explain this query, Ask AI: optimize this query, and Ask AI: build a notebook. Config problems appears when the configuration has reports. None of these has a default key.

`Alt+O t` opens the theme picker. Moving the selection previews a theme. Enter saves the selection. Esc cancels. See [themes](themes.md).

`Ctrl+I` opens AI chat. `Alt+I` fills the chat input with the editor buffer without sending. `Ctrl+H` asks AI for help with an editor error or a failed check at once. Some terminals capture these keys, and the palette has the same commands. See the [AI guide](ai.md) for chat controls, providers, write confirmation, and shared data.

## Settings

The Settings palette command opens the settings screen. No key opens it.

The sections are on the left, and the rows of the selected section are on the right. Each change is written to the config file at once. The status bar shows `Saved`, or the reason a value was rejected.

| Section | Contents |
| --- | --- |
| AI | chat, chat source, provider pages, agent pages |
| Appearance | theme, icons, key hints, hide system schemas |
| Keys | key preset, one page of keys per scope |
| MCP | access level, server limits, connections page |
| Notebooks | notebook directories page |

A row marked `›` opens its own page.

| Key | Context | Action |
| --- | --- | --- |
| `↑` `↓` `Home` `End` | either pane | move between rows |
| `→` | the sections | move to the rows |
| `Tab` | either pane | move to the other pane |
| `Enter` `→` | a row marked `›` | open its page |
| `←` `→` | a row with a list of values | change the value |
| `Enter` `Space` | an on/off row | toggle the value |
| `Enter` | a text row | edit and save the value |
| `Enter` | an action row | run the action |
| `Esc` `←` | a page, a pane, the screen | go back one step |
| `Ctrl+T` | an agent page | load the models of the agent |

A key row takes a chord in config-file syntax, such as `ctrl+alt+f`. A comma separates alternative chords, a space separates the presses of a key sequence, and `none` removes the binding. The row opens empty, and an empty row restores the preset chord. A chord that no terminal can send is rejected. A chord bound to two actions in one scope is saved and reported.

A change rewrites only the table of its row. Every line outside that table stays unchanged, including an `api_key`, which the screen never writes. A change applies at once. Providers are built in and cannot be removed.

## Mouse controls

- Scroll the cell list of a notebook with the wheel or the scrollbar at its right. Click a row to move to that cell. Click it again to open the cell.
- Click a pane to focus it. Click a tab, a connection, a statement result, or a view label to select it.
- Click a tree row to select it. Double-click to open it. Click the fold marker to expand or collapse.
- Click a grid cell to select it. Double-click a row to open the row details.
- Click a column header to sort. Shift-click adds the column to the sort. Sorting discards staged edits.
- Drag a column edge to resize the column. Double-click the edge to reset the width.
- Drag the divider between the editor and the result to resize both panes. Either edge works: the bottom of the editor or the top of the result. Click the bottom of the editor without dragging to show or hide the result pane.
- Drag the right border of the object tree to set its width. The tree is at least 16 columns wide, and the pane beside it at least 32.
- Drag a scrollbar or turn the wheel to scroll. The wheel does not move the cursor. The next cursor key scrolls back to the cursor.
- A trackpad or a tilting wheel scrolls the grid sideways by columns, and the editor sideways by cells. `Shift` with a plain wheel does the same.
- In the editor, click to place the caret. Double-click selects a word, and triple-click selects a line. Drag to select text.
- Drag over other text on screen to select it. `Ctrl+C` copies the selected text.
- Right-click an object, a cell, a header, a tab, a connection, or the editor for a context menu.
- Middle-click a tab or click its close mark to close the tab. The usual close confirmation applies.
- Click a key hint or a menu entry to run its action.

## Troubleshooting

- A key types text: check the focus and the [scope rules](keys.md#scopes). A card can bind a different action to the same key.
- A key with a modifier does nothing: check terminal and multiplexer support. Use a listed alternative or the palette, or [rebind the action](keys.md#rebinding).
- Copy and paste do nothing: masume reads and writes the system clipboard with `wl-copy`, `xclip`, `xsel`, `pbcopy` or `clip.exe`. Without any of these, masume falls back to the OSC 52 escape sequence, which needs terminal support. `Ctrl+V` then pastes only text copied inside masume.
- Rows are missing: check screen filters, server filters, SQL limits, and the loaded-row count. `Ctrl+F` fetches another page.
- The client rejects grid editing: check the status message, the primary key, the selected columns, the relation kind, and the profile access mode.
- A connection or the configuration fails: read the error and Config problems in the palette. See [configuration](configuration.md) and [engines](engines.md).
- Symbols or colors are unreadable: set ASCII icons or another [theme](themes.md) in the [interface configuration](configuration.md#interface).
