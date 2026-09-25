# Configuration

The user configuration file is `$XDG_CONFIG_HOME/masume/config.toml`. The default path is `~/.config/masume/config.toml`, and `%APPDATA%\masume\config.toml` on Windows. A repository can provide profiles and queries in a [project file](#project-file), `.masume.toml`.

[`config.example.toml`](../config.example.toml) lists the settings and sample profiles. Not every example value is a default. The tables below list the defaults. [Usage](usage.md) covers the client workflows.

If no file exists, the client, `masume --detect` and `masume --mcp` create a starter file. `masume run` does not. The connection form saves profiles in the user configuration file. A save leaves text outside the edited profile block unchanged, but a rewritten assignment can lose its inline comment.

## Sections

| Section | Contains |
| --- | --- |
| [`[masume]`](#file-version) | Config file version |
| [`[profile.NAME]`](#profiles) | One connection profile |
| [`[secret.NAME]`](#secret-stores) | One password store, shared by profiles |
| [`[ui]`](#interface) | Icons, theme and colours |
| [`[keys]`](#keys) | Key preset and bindings per scope |
| [`[ai]`](#ai) | AI chat availability and provider settings |
| [`[mcp]`](#mcp) | Profiles an agent can use, and the access level |
| [`[notebooks]`](#notebooks) | Extra notebook directories |

A committed [`.masume.toml`](#project-file) accepts only `[profile.NAME]` and `[query.NAME]`.

Every key is optional unless its table lists it as `required`. Invalid TOML stops the whole file from loading. masume reports the error and uses the default settings, with no profiles from that file. The user file and project file load separately.

An invalid value in valid TOML causes masume to skip the profile or use the default. Some invalid values are reported, others fall back to the default silently. For example, a non-boolean `autocommit` uses `true`, and a non-positive AI timeout uses `30000`.

Reports are shown in these places:

| Report | Shown |
| --- | --- |
| A file, profile or secret store | On stderr at startup. The client also lists reports under **Config problems** in the palette, `Ctrl+K` |
| `[ui]`, `[keys]`, `[ai]` or a theme | Under **Config problems** in the client palette. Headless commands do not report problems in these settings |

Most unknown keys are ignored without a report. The exceptions include unknown actions, icon kinds, providers, and sections in a project file.

## File version

`[masume] version` is the config file version.

```toml
[masume]
version = 1
```

When the file version is below the current version, masume adds the tables that the newer versions introduced and changes nothing else. Before the first change, masume copies the file to `config.toml.bak.N`, where `N` is the old version. A file at or above the current version is left unchanged.

| Version | Adds |
| --- | --- |
| 1 | the `[ai.providers.*]` and `[ai.agents.*]` tables |

## Profiles

One profile is one connection. The name after `profile.` is the name shown in the picker.

```toml
[profile.shop]
engine   = "postgres"
host     = "127.0.0.1"
port     = 5432
database = "shop"
user     = "ada"
auth     = "prompt"
env      = "dev"
mode     = "write"
```

| Key | Default | Meaning |
| --- | --- | --- |
| `engine` | `postgres` | Database engine. [engines.md](engines.md) lists the engines |
| `host` | required | The connection form default is `127.0.0.1`. A path or `socket` means a unix socket; see [Unix socket](#unix-socket). Ignored for SQLite |
| `port` | per engine | Server port |
| `database` | required, except on MySQL-protocol engines | Database name, SQLite file path, or Redis database number |
| `user` | required if the engine needs one | Ignored for SQLite. Optional for Redis and MongoDB |
| `auth` | `secret` if `secret` is set; otherwise `command` if `password_command` is set; otherwise `password` | Password source: `prompt`, `keyring`, `command`, `secret` or `password`. See [Passwords](#passwords) |
| `password_env` | | Environment variable with the password. Used when `auth` is `password` |
| `password_command` | | Shell command; the first line of stdout is the password. Used when `auth` is `command` |
| `secret` | | `[secret]` store name. Used when `auth` is `secret` |
| `secret_ref` | | Reference in the store, passed to the store command as one quoted argument |
| `env` | `dev` | `dev`, `test` or `prod`. The theme sets the colour of each environment |
| `mode` | `write` | `write` or `read-only`. See [read-only access](engines.md#read-only-access) |
| `confirm_writes` | `off` on dev, `delete` on test, `write` on prod | `off`, `delete`, `write` or `agent`. See [MCP confirmation](mcp.md#clients-without-elicitation) for clients without dialogs |
| `write_plan` | `off` on dev, `count` on test, `undo` on prod | `off`, `count` or `undo`. See [Write plans](#write-plans) |
| `undo_rows` | `1000` | Maximum rows captured for an undo. `0` means a ceiling of 1048576 rows |
| `sslmode` | per engine | TLS mode. See [TLS](#tls) |
| `sslrootcert` | the system trust store | Certificate authority bundle in PEM form. See [Certificate files](#certificate-files) |
| `sslcert` | | Client certificate in PEM form, sent when the server asks for one |
| `sslkey` | | Private key for `sslcert`. Required with `sslcert` |
| `statement_timeout_ms` | `0` | Time limit for one statement in milliseconds. `0` means the server default |
| `keepalive_s` | `30` | Seconds between connection checks. `0` disables the keepalive |
| `page_size` | `200` | Rows per page, in the grid and in `masume run`. Must be above zero |
| `autocommit` | `true` | `false` opens a transaction when a statement runs in the TUI, and keeps it open until commit or rollback |
| `ssh_host` | | SSH server host. Without it masume connects to `host` directly. See [SSH tunnel](#ssh-tunnel) |
| `ssh_port` | `22` | SSH server port |
| `ssh_user` | required with `ssh_host` | SSH server user |
| `ssh_key` | | Private key path. Without it the tunnel uses the SSH agent |
| `ssh_key_passphrase_env` | | Environment variable with the `ssh_key` passphrase |
| `ssh_password_env` | | Environment variable with the SSH password |
| `ssh_known_hosts` | `~/.ssh/known_hosts` | Known hosts file with the SSH host key |
| `command` | | Shell command started before the connection opens and stopped when it closes, for example an SSH tunnel. See [Connection command](#connection-command) |
| `wait_for_port` | | TCP port on `host` checked before connecting. Without it, masume connects right after starting `command` |
| `command_timeout` | `10` | Seconds to wait for `wait_for_port`. Must be above zero |
| `mcp` | the `[mcp]` level | MCP access limit for the profile: `off`, `read-only`, `read-write` or `full`. The global limit and the profile `mode` still apply. See [mcp.md](mcp.md) |
| `description` | | Free text, edited in the connection form. Not shown in the picker |
| `ai_instructions` | | Database context sent to the AI model with each chat request on this connection |

A profile with a missing required key is skipped and reported. The other profiles still load.

A leading `~` in `database` expands to the home directory. Relative SQLite paths in `.masume.toml` use the project file directory; relative paths in the user file and on the command line use the startup directory. `:memory:` opens an in-memory database.

The per-environment defaults of `confirm_writes` and `write_plan` apply when the profile does not set them. Changing `env` in the connection form does not change the confirmation setting. A new form starts with confirmation `off`. [Usage](usage.md) covers connection forms and transaction commands.

Headless commands use `statement_timeout_ms`, `mode`, `page_size`, and `command`. They do not use `autocommit`, `confirm_writes`, `write_plan`, or undo. They cannot prompt for a password. See [headless.md](headless.md).

### TLS

Redshift, Neon, Supabase, PlanetScale, Turso, Azure SQL Database, and Amazon DocumentDB default to `require`. Other PostgreSQL-family and MySQL-family engines default to TLS with an unencrypted fallback. MongoDB defaults to no TLS. SQLite does not use TLS.

| Mode | Behaviour |
| --- | --- |
| `disable` | No TLS |
| `allow`, `prefer` | TLS, no certificate check. PostgreSQL and MySQL can fall back to an unencrypted connection. MongoDB has no fallback |
| `require` | TLS, no certificate check |
| `verify-ca` | TLS, verify the CA without the host name |
| `verify-full` | TLS, verify the CA and the host name |

A profile with an unknown non-empty `sslmode` is skipped and reported. In the connection form, the `sslmode` field cycles through the modes, and `default` means the engine default.

### Certificate files

`sslrootcert` is the CA bundle for server verification. `verify-ca` and `verify-full` check the chain against it. Without it, the check uses the system trust store, which has no private CAs. A managed database with its own CA needs the bundle: Amazon RDS, Azure Database, Aliyun, and any internal PKI.

`sslcert` and `sslkey` are the client certificate and its private key, for a server that asks for one. Set both or neither; a profile with only one is skipped. masume sends the pair in every mode that uses TLS.

```toml
[profile.shop]
engine      = "postgres"
host        = "shop.eu-central-1.rds.amazonaws.com"
database    = "shop"
user        = "reader"
sslmode     = "verify-full"
sslrootcert = "~/.certs/rds-ca-rsa2048-g1.pem"
sslcert     = "~/.certs/client.pem"
sslkey      = "~/.certs/client.key"
```

A leading `~` expands to the home directory. If masume cannot read a file, the connection fails and the error shows the path. The same happens for a bundle with no PEM certificate.

PostgreSQL-family, MySQL-family, SQL Server, ClickHouse, MongoDB, Redis, Cassandra, and ScyllaDB engines read the three keys. Turso connects over `wss://` with the system trust store and ignores all three. SQLite uses no TLS. With `disable` or a unix socket there is no TLS, and the files are not used.

The connection form has a `tls files` toggle. Set it to `on`, and the form shows the three fields. Set it to `off`, and saving removes all three keys from the profile.

On any of the three fields, `Enter` or a click opens a file picker. The picker starts in the directory of the current path, or in the startup directory if the field is empty. `Enter` puts the selected file in the field, and `Escape` leaves the field unchanged. A path under the home directory is written with `~`. The same picker is used for `ssh key`, `ssh known hosts`, and the SQLite database file.

The command line accepts the same names as URL parameters and as connection keywords.

```
masume "postgres://reader@db.internal/shop?sslmode=verify-full&sslrootcert=~/.certs/ca.pem"
masume "host=db.internal dbname=shop user=reader sslmode=verify-ca sslrootcert=/certs/ca.pem"
```

## Project file

A repository can contain `.masume.toml` with shared profiles and queries.

masume looks for `.masume.toml` in the startup directory, then in each parent directory. The nearest one is the project file. The client, `masume run`, and `masume --mcp` read the project file.

```toml
# .masume.toml, committed with the code

[profile.dev]
engine   = "postgres"
host     = "127.0.0.1"
port     = 5432
database = "shop"
user     = "shop"
env      = "dev"

[profile.staging]
engine   = "postgres"
host     = "staging.internal"
database = "shop"
user     = "reader"
auth     = "prompt"
env      = "test"
mode     = "read-only"

[query.recent-orders]
sql         = "select * from orders order by created_at desc limit 50"
description = "the newest 50 orders"

[query.stuck-jobs]
sql      = "select * from jobs where state = 'running' and started_at < now() - interval '1 hour'"
profiles = ["staging"]
```

### Project profiles

A project profile accepts the same connection keys as a user profile, except the keys below. A profile that sets one of them is skipped, even with an empty value:

| Key | Meaning |
| --- | --- |
| `password_command` | A shell command run on connect |
| `command` | A shell command run on connect |
| `password_env` | An environment variable |
| `secret`, `secret_ref` | A secret store |

`auth = "prompt"` and `auth = "keyring"` are allowed. Other password sources need a user profile. Project profiles accept `env`, `mode`, `confirm_writes`, and `write_plan`.

Other sections are reported and ignored, including `[secret]`, `[ui]`, `[keys]`, `[ai]`, and `[mcp]`.

### Project queries

`[query.NAME]` is a shared query, listed under `Ctrl+Q`:

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `sql` | string | required | SQL statement. A query with a missing or empty statement is skipped and reported |
| `description` | string | | Shown instead of the SQL in the saved query list |
| `profiles` | list of strings | every profile | Profiles that can use the query. An empty list means every profile |

### Overrides

A user profile replaces the project profile with the same name. A user query replaces the project query with the same name.

The connection picker marks project profiles with `project` and shows the project path. `d` cannot remove a project profile. `e` opens a project profile in the connection form. `Ctrl+S` saves it as a user override. The override keeps every setting of the profile, including settings the form does not show.

`Ctrl+Q` lists project and user queries, sorted by name. Project queries have a `project` label. `Enter` loads the selected query into the editor. `Ctrl+D` cannot remove a project query. To remove a project profile or query, edit the project file.

## Write plans

With `write_plan`, masume checks a single write before it runs, if the statement is supported. PostgreSQL-family, MySQL-family, and SQLite engines support write plans; MongoDB does not. The plan uses the target table and the predicate from the statement. Unsupported statements, such as joins, target aliases and batches, go through the normal confirmation.

Staged grid changes get a plan when `Ctrl+Y` applies them. Changes of one kind share one plan. A mix of inserts, updates and deletes, or a change the plan cannot read, is applied without a plan.

The server counts the matching rows with the write predicate. The count can change before the write runs. An update plan also lists assigned columns. `cascades` lists trigger names and foreign key effects. The plan does not inspect trigger bodies or predict their effects. `blocked` lists foreign keys that can reject the delete. In the client, these keys put a headline at the top of the plan, and Enter opens the referencing rows.

`write_plan = "undo"` prepares an undo for updates, deletes, and truncates. An update undo restores assigned columns by primary key. A delete or truncate undo inserts captured target rows. `Alt+U` (`global.undo-write`) asks for confirmation, then runs the undo statements together. The connection keeps only the latest write result, in memory.

Undo capture reads target rows inside the write transaction. Row locking follows the engine's transaction rules. If a transaction is open, the write joins it and does not commit; otherwise, masume starts and commits a transaction. Engine DDL rules still apply; for example, MySQL-family `TRUNCATE` commits implicitly.

Undo covers only the captured target rows. It does not cover cascaded rows or trigger effects. Undo statements can run triggers again. A later undo can overwrite newer values in the restored columns. Restoring deleted rows can fail on key or constraint conflicts.

Inserts, tables without primary keys, updates that assign primary keys, plans with zero matching rows, failed counts, failed metadata reads, and counts above `undo_rows` get no undo. The plan shows the reason. The write can still run after confirmation. Imports do not keep an undo.

The default `undo_rows` is `1000`; `0` removes the configured limit and uses a capture ceiling of 1048576 rows. If the plan includes an undo, the capture must succeed before the write runs. A capture error or a truncated capture stops the write. If planning fails and leaves no undo, the write can still run after confirmation.

On a connection without transactions, writes run without undo and the reason is reported. The AI chat and MCP use the same planning code. MCP write responses include the captured undo SQL, if any. [Usage](usage.md) covers write confirmation and undo commands.

## Passwords

`auth` is the password source:

| `auth` | Password source |
| --- | --- |
| `prompt` | Password dialog at connect. The password stays in memory unless saved to the keyring |
| `keyring` | Password stored earlier in the operating system keyring |
| `command` | Shell command in `password_command` |
| `secret` | `[secret]` store in `secret`, at the reference in `secret_ref` |
| `password` | Environment variable in `password_env` |

```toml
auth = "prompt"
```

```toml
auth = "keyring"
```

```toml
auth             = "command"
password_command = "pass db/shop"
```

```toml
auth         = "password"
password_env = "PGPASSWORD"
```

A password command runs through `sh -c` with no stdin, and through `%COMSPEC% /d /s /c` on Windows. The command must exit with status 0 within 30 seconds and print a non-empty first line. Later output is ignored.

masume ignores passwords written in configuration files. A non-empty `password` value produces this warning:

```
profile "shop": passwords in files are ignored; enter the password and
select "remember in the keyring", or set password_env, password_command
or a [secret] store
```

`auth` stays as set: the configured environment variable, command, secret store, or keyring still applies. The client prompts only when the password source and the engine require it. SQLite needs no password. MongoDB without a user does not prompt unless `auth = "prompt"`. Redis takes a password without a user, for the server setting `requirepass`. A connection test from the form prompts through the same dialog and does not keep the password.

`masume run` and `masume --mcp` cannot prompt. A password must come from a non-interactive source. Saving a profile removes an existing `password` assignment from that profile block.

### Keyring

On Linux, masume uses the Secret Service API over D-Bus, which GNOME Keyring and KWallet provide. macOS uses Keychain. Windows uses Credential Manager. Keyring entries use the service name `masume` and the profile name.

On a machine with a keyring, the password dialog has a checkbox:

```
╭─ password ──────────────────────── PRODUCTION ─╮
│ connecting to shop-prod · prod                 │
│ reader@db.internal:5432/shop                   │
│                                                │
│ ••••••••••••                                   │
│                                                │
│ [x] remember in the keyring                    │
│                                                │
│   ↵ connect      Esc cancel                    │
╰────────────────────────────────────────────────╯
```

`Tab` moves the focus between the password field and the checkbox. `Space` toggles the checkbox. If the box is checked, a successful connection stores the password in the keyring. The saved profile then uses `auth = "keyring"`. A project profile needs a user override; masume does not edit the project file.

If an `auth = "keyring"` profile has no stored password, the dialog opens with the checkbox checked. A successful connection can then store the missing entry.

Passwords from the command line or from detection stay in memory at first. Saving the temporary profile can store the password in the keyring, even with `auth = "prompt"` selected. Without a keyring, the saved profile uses `auth = "prompt"` and stores no password.

Removing a saved connection with `d` also removes its keyring entry.

Without a keyring, the TUI hides the checkbox and prompts when needed. Headless commands cannot prompt.

### Secret stores

A secret store is a reusable password command under `[secret.NAME]`. `{{ref}}` is the placeholder for the profile's `secret_ref`.

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `command` | string | required | Shell command; the first line of stdout is the secret. Needs at least one unquoted, standalone `{{ref}}` argument |

```toml
[secret.work]
command = "op read {{ref}}"

[secret.infra]
command = "vault kv get -field=password {{ref}}"

[secret.local]
command = "sops -d --extract '[\"db\"][\"password\"]' {{ref}}"

[profile.shop-prod]
auth       = "secret"
secret     = "work"
secret_ref = "op://eng/shop-prod/password"

[profile.warehouse]
auth       = "secret"
secret     = "infra"
secret_ref = "secret/data/warehouse"
```

A profile with `secret` and no `auth` uses `auth = "secret"`. The profile still needs `secret_ref` and the normal connection fields.

Each `{{ref}}` becomes one shell-quoted argument. The placeholder must be unquoted and separate from other text; any other position is rejected. A reference can contain spaces, quotes, and semicolons, and the shell does not interpret them. For more than one input, call a script.

Templates accept literal arguments, quoted flags, and pipelines. The client rejects shell expansion, escapes outside single quotes, redirects, command lists, and multiline templates. For anything more complex, write a script and pass `{{ref}}` to it as an argument.

The called program still interprets its arguments. Never pass a reference as script text to `sh -c`, `cmd /c`, `eval`, or similar commands.

A failed store command reports the exit code and the first stderr line when present. The password command rules also apply: successful exit within 30 seconds, a non-empty first output line, and no stdin.

A profile that refers to a missing store is skipped and reported. A store with no command or an invalid placeholder is skipped and reported.

Secret stores belong in the user configuration file. Project files cannot declare or reference secret stores.

MongoDB credentials need a user. A server without authentication can reject supplied credentials.

## Unix socket

PostgreSQL and MySQL also listen on a unix socket. A `host` that starts with `/` or `~/` is a socket path, as in libpq and psql. `host = "socket"` uses the default socket path of a local server.

```toml
[profile.local]
engine   = "postgres"
host     = "socket"       # or "/var/run/postgresql"
port     = 5432
database = "shop"
user     = "turan"
```

`socket` resolves to the first of these paths that exists:

| Engine | Paths |
| --- | --- |
| PostgreSQL | `/var/run/postgresql`, `/run/postgresql`, `/tmp`, each with `.s.PGSQL.<port>` |
| MySQL | `/var/run/mysqld/mysqld.sock`, `/run/mysqld/mysqld.sock`, `/tmp/mysql.sock`, `/var/lib/mysql/mysql.sock` |

PostgreSQL takes the socket directory, and `port` selects the file in it. `/var/run/postgresql` and `/var/run/postgresql/.s.PGSQL.5432` resolve to the same connection. MySQL takes the socket file; a directory resolves to `mysqld.sock` or `mysql.sock` in it. If no socket is found, the connection fails and the error lists the checked paths.

Windows has no unix sockets, and a socket host fails there. A socket connection has no TLS, and `sslmode` is ignored. A socket host with `ssh_host` is rejected. Only the PostgreSQL- and MySQL-protocol engines accept a socket. SQLite opens a file and needs no host.

## SSH tunnel

A profile can connect through an SSH tunnel. masume opens the tunnel itself and does not run the `ssh` binary. The SSH server connects to `host` and `port`.

```toml
[profile.prod]
engine     = "postgres"
host       = "db.internal"
port       = 5432
database   = "shop"
user       = "reader"
auth       = "prompt"
ssh_host   = "ssh.example.com"
ssh_user   = "ada"
ssh_key    = "~/.ssh/id_ed25519"
```

masume opens a local forward: a listener on an ephemeral port of `127.0.0.1`, forwarded to `host:port` over the SSH connection. The listener and the SSH client close with the database connection. The picker and the title bar show `host:port`, not the local endpoint.

Authentication order: `ssh_key`, then `ssh_password_env`, then the agent on `$SSH_AUTH_SOCK`. An encrypted `ssh_key` needs `ssh_key_passphrase_env` unless the key is loaded in the agent.

Host key verification is strict. An unknown host key or a missing known hosts file fails the connection, and the error shows the path. Use `ssh-keyscan` to add a host key.

TLS certificates are verified against `host`, so `sslmode = "verify-full"` works through the tunnel. A tunneled MongoDB connection is a direct connection; masume does not use other replica set members.

The connection form has an `ssh tunnel` toggle. Set it to `on`, and the form shows the SSH fields and writes them to the profile. `Ctrl+T` tests the connection through the tunnel. Set it to `off`, and saving removes every `ssh_` key from the profile.

Project files cannot set `ssh_password_env` or `ssh_key_passphrase_env`.

## Connection command

A profile can start a shell command before connecting, for example an SSH tunnel. The command must stay in the foreground. masume stops the process group when the connection closes or when setup fails. A detached or daemonized process may keep running.

```toml
[profile.shop-tunnel]
engine          = "postgres"
host            = "127.0.0.1"
port            = 15432
database        = "shop"
user            = "reader"
auth            = "prompt"
command         = "ssh -N -L 15432:db.internal:5432 jump.example.com"
command_timeout = 10
wait_for_port   = 15432
```

masume tries a TCP connection to `host:wait_for_port` until it succeeds or the timeout passes. `wait_for_port` is only a readiness check; `port` stays the database port. Without `wait_for_port`, masume connects right after starting the command. `command_timeout` is the readiness timeout, not the command lifetime. Without `command`, there is no readiness check.

## Interface

```toml
[ui]
icons               = "plain"
theme               = "tokyonight"
hide_system_schemas = true
key_hints           = "full"
timezone            = "server"
```

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `icons` | `plain`, `ascii` or `nerd` | `plain` | The base glyph set. See [Icons](#icons) |
| `theme` | string | `ayu-dark` | A built-in theme name, a file name in `themes/` without `.toml`, or `system` for the terminal colours. See [themes.md](themes.md) |
| `hide_system_schemas` | boolean | `true` | `false` displays system schemas, including `pg_catalog` and `information_schema`. `h` in the tree toggles them for the session |
| `key_hints` | `full`, `main` or `off` | `full` | Key hint level for the status bar, title bar, tab row, pane strips and pane borders. An unknown mode is reported and falls back to `full`. See [Key hint modes](#key-hint-modes) |
| `timezone` | `server`, `utc` or `local` | `server` | Display zone for timestamps with a time zone in the grid. `server` is the zone the server returns, `local` the zone of this computer. An unknown zone is reported and falls back to `server`. See [Time zones](#time-zones) |

### Time zones

`timezone` applies to PostgreSQL `timestamptz`, SQL Server `datetimeoffset`, ClickHouse `DateTime`, Cassandra `timestamp`, and MongoDB dates. The grid shows these values in the selected zone with the offset, as in `2026-09-23 14:30:00.000 +02:00`. A timestamp without a time zone shows its stored value with no offset. MySQL returns timestamps as text in the session time zone, so `timezone` does not change them.

| Engine | Zone in `server` mode |
| --- | --- |
| PostgreSQL | Session `TimeZone`, including changes by `SET TIME ZONE` |
| SQL Server | Offset stored in the `datetimeoffset` value |
| ClickHouse | Column time zone, or the server time zone |
| Cassandra, MongoDB | UTC |

Cell viewers and row viewers show the same text as the grid. Copies, exports, and filters use UTC.

### Icons

`icons = "plain"` is the default set. `icons = "ascii"` is ASCII-only. `icons = "nerd"` needs a Nerd Font; without one, the terminal draws empty boxes. An unknown set is reported and falls back to `plain`.

`[ui.icon_glyphs]` overrides single glyphs in any of the three sets. An empty string hides that kind. An unknown kind is reported.

```toml
[ui]
icons = "plain"

[ui.icon_glyphs]
schema            = ""
table             = ""
view              = ""
materialized-view = ""
function          = ""
sequence          = ""
type              = ""
trigger           = ""
column            = ""
index             = "▤"
primary-key       = ""
foreign-key       = ""
role              = ""
roles             = ""
favourites        = ""
recent            = ""
query             = "≡"
folder            = "▸"
plan              = "⊳"
note              = ""
problem           = "✗"
ai                = "✦"
fold-closed       = "▸"
fold-open         = "▾"
field             = "▸"
close             = "×"
dot               = "●"
sort-up           = "↑"
sort-down         = "↓"
prompt            = "❯"
step-back         = "‹"
step-on           = "›"
banner            = "⚑"
new-tab           = "+"
```

A terminal without a Nerd Font draws the `nerd` glyphs, and the example above, as empty boxes.

| Kind | Drawn for | `plain` | `ascii` | `nerd` |
| --- | --- | --- | --- | --- |
| `schema` | A schema in the object tree | `◇` | `~` | `` |
| `table` | A table | `▦` | `T` | `` |
| `view` | A view | `◈` | `V` | `` |
| `materialized-view` | A materialized view | `◆` | `M` | `` |
| `function` | A function | `ƒ` | `f` | `` |
| `sequence` | A sequence | `№` | `S` | `` |
| `type` | A type | `⊞` | `Y` | `` |
| `trigger` | A trigger | `⚑` | `!` | `` |
| `column` | A column | `⋮` | `c` | `` |
| `index` | An index | `▤` | `#` | `▤` |
| `primary-key` | A primary key | `◆` | `*` | `` |
| `foreign-key` | A foreign key | `→` | `>` | `` |
| `role` | A role | `●` | `o` | `` |
| `roles` | The roles folder | `●` | `o` | `` |
| `favourites` | The favourites folder | `★` | `*` | `` |
| `recent` | The recent folder | `↻` | `@` | `` |
| `query` | A saved query | `≡` | `=` | `≡` |
| `folder` | A folder | `▸` | `>` | `▸` |
| `plan` | A query plan | `⊳` | `>` | `⊳` |
| `note` | A notice | `⚠` | `!` | `` |
| `problem` | A problem | `✗` | `x` | `✗` |
| `ai` | The AI chat | `✦` | `*` | `✦` |
| `fold-closed` | A closed fold | `▸` | `>` | `▸` |
| `fold-open` | An open fold | `▾` | `v` | `▾` |
| `field` | A form field marker | `▸` | `>` | `▸` |
| `close` | A close control | `×` | `x` | `×` |
| `dot` | A status dot | `●` | `o` | `●` |
| `sort-up` | Ascending sort | `↑` | `^` | `↑` |
| `sort-down` | Descending sort | `↓` | `v` | `↓` |
| `prompt` | A prompt marker | `❯` | `>` | `❯` |
| `step-back` | A step back | `‹` | `<` | `‹` |
| `step-on` | A step forward | `›` | `>` | `›` |
| `banner` | A banner | `⚑` | `!` | `⚑` |
| `new-tab` | A new tab | `+` | `+` | `+` |

`[ui.palette]`, `[ui.colors]`, and `[ui.syntax]` overlay the selected theme. See [themes.md](themes.md) for colour names, token kinds, and inheritance.

### Key hint modes

| Mode | Hints |
| --- | --- |
| `full` | Every key hint on the status bar, title bar, tab row, pane strips, pane borders and cards |
| `main` | Primary key hints only. See the list below |
| `off` | No key hints. Bars, strips, borders and cards still show their status: statement count, caret position, error count, result rows, card purpose |

`main` shows the primary keys: the main key of each pane (open a tree row, run the statement, run all, run a notebook cell), the key that opens the menu for the row under the cursor, keys for the current state (cancel a running read, retry a failed one, fetch more rows, count the rows, edit a table as a query), and keys for actions with no other key (show a hidden tree, step through the connections). On a card, `main` shows the answer keys and the close key, with no extras. The chat card shows `ask`, `to editor`, and `close`, and the notebook card shows `open` and `close`. The title bar, pane borders, and plan strip keep their keys; the tab row and the step keys of the result strips show none.

`full` and `main` show the AI keys: `ask AI` on the title bar, the AI key on the editor border, and `ask AI` on the plan strip. `off` hides all three.

Every mode shows the chords in menu rows, the palette, and the help card. Every mode also shows the answer chips of a question, and the key in a report that has one, such as the key that undoes a write. A hidden key still works. In every mode, the palette (`^K`) and the help card (`?`) list every action.

## Keys

```toml
[keys]
preset = "default"

[keys.global]
refresh-objects = []
run-at-cursor = ["ctrl+r", "f5"]

[keys.grid]
copy-menu = "alt+y"
```

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `preset` | string | `default` | Base key set. `default` is the only preset |

Each table under `[keys]` is a scope. An entry binds an action to one chord or to a list of chords. An empty list removes the binding. Actions not listed keep the preset bindings.

| Scope | Applies to |
| --- | --- |
| `global` | The whole client. Bindings of the focused pane and open dialogs come first |
| `tree` | The object tree on the left |
| `grid` | The result grid |
| `editor` | The query editor |
| `plan` | The query plan tree |
| `notebook` | The cell list of a notebook tab |
| `document` | Document tree of result rows |
| `list` | Any list inside a card: the history, the saved queries, the palette |
| `dialog` | A question card, and the connection picker |

`alt`, `meta`, and `option` are names for the same modifier. Unknown actions and invalid chords are reported, and the preset binding stays. [Keys](keys.md) lists the actions and defaults. [Usage](usage.md) covers the corresponding workflows.

## AI

```toml
[ai]
enabled              = true
default_provider     = "anthropic"
statement_timeout_ms = 30000

[ai.providers.anthropic]
model       = "claude-opus-5"
api_key_env = "ANTHROPIC_API_KEY"
```

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `enabled` | boolean | `true` | `false` turns off the AI chat and hides its UI. MCP is not affected |
| `default_provider` | `anthropic`, `openai`, `grok` or `openai_compatible` | `anthropic` | Initial chat provider. The palette switches the provider for the session. An unknown name is reported |
| `default_agent` | string | empty | Agent name from `[ai.agents]`. When set, the chat uses this agent instead of a provider. A name with no table is reported |
| `statement_timeout_ms` | integer above zero | `30000` | Timeout for the chat `run_query` tool, in milliseconds. Does not apply to other tools or to undo capture |

Each provider has one table: `[ai.providers.anthropic]`, `[ai.providers.openai]`, `[ai.providers.grok]` and `[ai.providers.openai_compatible]`. A provider without a model is reported when the chat uses it.

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `model` | string | empty | Model ID. Required for every provider |
| `api_key` | string | empty | API key, stored in the file. A non-empty value takes priority over `api_key_env`. Optional for `openai_compatible` |
| `api_key_env` | string | empty | Environment variable with the API key. No default variable name |
| `base_url` | string | empty | Provider or proxy address. A non-empty value takes priority over `base_url_env`. Required for `openai_compatible` |
| `base_url_env` | string | empty | Environment variable with the provider or proxy address. No default variable name |
| `max_tool_steps` | integer above zero | `25` | Maximum provider rounds per question |

Without a configured address, Anthropic uses `https://api.anthropic.com/v1`, OpenAI uses `https://api.openai.com/v1` and Grok uses `https://api.x.ai/v1`. masume removes trailing slashes and appends `/v1` unless the configured address already ends with `/v1`. The client then uses `/messages` for Anthropic, `/responses` for OpenAI and `/chat/completions` for Grok and for `openai_compatible`.

`openai_compatible` has no default address, so `base_url` or `base_url_env` is required. Requests send an `Authorization` header only when a key is set.

Unknown provider tables are reported and ignored. See [ai.md](ai.md) for provider data and credential sources.

The [settings screen](usage.md#settings) writes `[ai]` and the table of the provider or agent it saves. It never writes `api_key`, so an existing key in the file stays. Its other pages write `[ui]`, `[keys]`, `[mcp]` and `[notebooks]`.

### AI agents

Each agent has one table, `[ai.agents.NAME]`. `NAME` is the agent name in the palette and the settings screen.

The config file has tables for `claude`, `codex`, `gemini` and `opencode`. A table with any other name adds an agent. An agent without `command` is reported and left out.

```toml
[ai.agents.claude]
command = "npx"
args    = ["@zed-industries/claude-code-acp"]

[ai.agents.opencode]
command = "opencode"
args    = ["acp"]
```

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `command` | string | required, except for built-in agents | Program that serves the Agent Client Protocol on stdin and stdout. An agent without one is reported and skipped |
| `args` | list of strings | empty | Arguments for `command` |
| `model` | string | empty | Agent model, from the agent's model list. Empty means the model set in the agent's own configuration |
| `env` | list of strings | empty | Extra environment variables for the child process, as `NAME=VALUE`, added to the masume environment. An entry with an empty value replaces the inherited value |

masume runs the command as a child process and serves it the chat tools over the loopback address. `[mcp]` does not apply to the chat. See [ai.md](ai.md#agents).

## MCP

```toml
[mcp]
profiles   = ["shop-dev"]
access     = "read-only"
row_limit  = 500
timeout_ms = 30000
```

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `profiles` | list of strings | empty | Profiles an agent can use. An empty list allows none |
| `access` | `off`, `read-only`, `read-write` or `full` | `read-only` | Global access limit. The profile `mcp` key can lower it. `mode = "read-only"` limits access to reads |
| `row_limit` | integer above zero | `500` | Maximum rows returned by `run_query`. Does not apply to catalog results, undo rows or changed rows |
| `timeout_ms` | integer above zero | `30000` | Timeout for the MCP `run_query` tool, in milliseconds. Does not apply to other tools or to undo capture |

An unknown `access` level, or a `row_limit` or `timeout_ms` of zero or less, falls back to the default without a report.

See [mcp.md](mcp.md) for tools and write confirmation.

## Notebooks

`[notebooks]` adds directories to the notebook list. The list always includes `<project root>/.masume/notebooks` and `$XDG_STATE_HOME/masume/notebooks`.

```toml
[notebooks]
paths = ["~/notes/sql"]
```

See the [notebook guide](notebooks.md) for the file format and the run policy.

## Paths

| Path | Contains |
| --- | --- |
| `$XDG_CONFIG_HOME/masume/themes/` | Custom themes, one file each |
| `$XDG_STATE_HOME/masume/history.sqlite` | Query history, saved queries, marks, tabs, chats, editor contexts and catalog cache |
| `history.sqlite-wal`, `history.sqlite-shm` | SQLite sidecar files beside the history file |
| `$XDG_STATE_HOME/masume/mcp.log`, `mcp.log.1` | Partial MCP diagnostics and one rotated backup |
| `$XDG_STATE_HOME/masume/ai-chat.log`, `ai-chat.log.1` | Partial chat diagnostics and one rotated backup |

`XDG_CONFIG_HOME` is the configuration base directory, and `XDG_STATE_HOME` is the state base directory, normally `~/.local/state`. Windows reads neither: the configuration base directory is `%APPDATA%` and the state base directory is `%LOCALAPPDATA%`. Keyring entries are not stored in these files. See [security](../SECURITY.md#stored-data) for permissions, retention, and diagnostic limits.