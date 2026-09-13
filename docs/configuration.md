# Configuration

The user configuration file is `$XDG_CONFIG_HOME/masume/config.toml`. The default path is `~/.config/masume/config.toml`. A repository can provide profiles and queries in a [project file](#project-file), `.masume.toml`.

[`config.example.toml`](../config.example.toml) lists the settings and sample profiles. The example values are not all defaults. The tables below give the defaults. [Usage](usage.md) covers the client workflows.

On the first run, masume creates a starter file if none exists. The client, `masume --detect` and `masume --mcp` create the file; `masume run` does not. The connection form saves profiles in the user configuration file. Text outside the edited profile block stays unchanged. Rewritten assignments can lose inline comments.

## Sections

| Section | Holds |
| --- | --- |
| [`[profile.NAME]`](#profiles) | One connection profile |
| [`[secret.NAME]`](#secret-stores) | One password store, shared by profiles |
| [`[ui]`](#interface) | Icons, theme and colours |
| [`[keys]`](#keys) | The key preset and bindings for each scope |
| [`[ai]`](#ai) | AI chat availability and provider settings |
| [`[mcp]`](#mcp) | The profiles an agent reaches, and its access level |
| [`[notebooks]`](#notebooks) | Extra notebook directories |

A committed [`.masume.toml`](#project-file) accepts only `[profile.NAME]` and `[query.NAME]`.

Every key is optional unless the table marks the key `required`. Invalid TOML prevents the whole file from loading. masume reports the file error and uses default settings without the profiles from that file. The user file and project file load separately.

An invalid value in valid TOML can skip a profile or use a default. Some invalid values produce reports; others silently use defaults. For example, a non-boolean `autocommit` uses `true`, and a non-positive AI timeout uses `30000`.

Reports appear here:

| Report | Where it appears |
| --- | --- |
| A file, profile or secret store | On stderr at startup. The client also lists reports under **Config problems** in the palette, `Ctrl+K` |
| `[ui]`, `[keys]`, `[ai]` or a theme | Under **Config problems** in the client palette. Headless commands do not report these settings |

Unknown keys are generally ignored without a report. Exceptions include unknown actions, icon kinds, providers, and sections in a project file.

## Profiles

One profile is one connection. The name after `profile.` is the picker name.

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
| `engine` | `postgres` | See [engines.md](engines.md) for the list |
| `host` | required | The form defaults to `127.0.0.1`. A path or `socket` connects over a unix socket; see [Unix socket](#unix-socket). Ignored for SQLite |
| `port` | per engine | The server port |
| `database` | required, except on MySQL-protocol engines | The database name, or the SQLite file path |
| `user` | required if the engine needs one | Ignored for SQLite. Optional for MongoDB |
| `auth` | `secret` if `secret` is set; otherwise `command` if `password_command` is set; otherwise `password` | The password source: `prompt`, `keyring`, `command`, `secret` or `password`. See [Passwords](#passwords) |
| `password_env` | | The environment variable that holds the password. Read when `auth` is `password` |
| `password_command` | | A shell command that prints the password on its first line. Read when `auth` is `command` |
| `secret` | | The `[secret]` store that holds the password. Read when `auth` is `secret` |
| `secret_ref` | | The reference inside that store, passed to its command as one quoted argument |
| `env` | `dev` | `dev`, `test` or `prod`. The theme provides the environment colour |
| `mode` | `write` | `write` or `read-only`. See [read-only access](engines.md#read-only-access) |
| `confirm_writes` | `off` on dev, `delete` on test, `write` on prod | `off`, `delete`, `write` or `agent`. See [MCP confirmation](mcp.md#clients-without-elicitation) for clients without dialogs |
| `write_plan` | `off` on dev, `count` on test, `undo` on prod | `off`, `count` or `undo`. See [Write plans](#write-plans) |
| `undo_rows` | `1000` | Maximum captured rows for an undo. `0` uses a ceiling of 1048576 rows |
| `sslmode` | per engine | The TLS mode. See [TLS](#tls) |
| `statement_timeout_ms` | `0` | Time limit for one statement in milliseconds. `0` uses the server default |
| `keepalive_s` | `30` | Seconds between connection checks. `0` disables the keepalive |
| `page_size` | `200` | Rows the grid loads per page, and rows one page of `masume run` holds. Must be above zero |
| `autocommit` | `true` | `false` starts a transaction on TUI statement execution and keeps the transaction open until commit or rollback |
| `ssh_host` | | SSH server host. Without it masume connects to `host` directly. See [SSH tunnel](#ssh-tunnel) |
| `ssh_port` | `22` | SSH server port |
| `ssh_user` | required with `ssh_host` | SSH server user |
| `ssh_key` | | Private key path. Without it the tunnel uses the SSH agent |
| `ssh_key_passphrase_env` | | Environment variable with the `ssh_key` passphrase |
| `ssh_password_env` | | Environment variable with the SSH password |
| `ssh_known_hosts` | `~/.ssh/known_hosts` | Known hosts path with the SSH host key |
| `command` | | A shell command started before connect and stopped when the connection closes, for example an SSH tunnel. See [Connection command](#connection-command) |
| `wait_for_port` | | The TCP port checked on `host` before connection. Without this key, masume connects immediately after starting `command` |
| `command_timeout` | `10` | Seconds to wait for `wait_for_port`. Must be above zero |
| `mcp` | the `[mcp]` level | The profile access limit: `off`, `read-only`, `read-write` or `full`. The global limit and profile mode still apply. See [mcp.md](mcp.md) |
| `description` | | Profile text stored and edited in the connection form. The picker does not display this text |
| `ai_instructions` | | Database context sent to the AI model with AI chat requests on this connection |

A missing required key skips the profile and produces a report. Other valid profiles still load.

A leading `~` in `database` expands to the home directory. Relative SQLite paths in `.masume.toml` use the project file directory. Relative paths in the user file or command line use the startup directory. `:memory:` opens an in-memory database.

Environment defaults apply when `confirm_writes` or `write_plan` is absent from a profile file. Changing `env` in the connection form does not change the confirmation setting. A new form starts with confirmation `off`. [Usage](usage.md) covers connection forms and transaction commands.

Headless commands use `statement_timeout_ms`, `mode`, `page_size` and `command`. They do not use `autocommit`, `confirm_writes`, `write_plan` or undo, and they cannot prompt for a password. See [headless.md](headless.md).

### TLS

Redshift, Neon, Supabase and PlanetScale default to `require`. Other PostgreSQL-family and MySQL-family engines default to TLS with an unencrypted fallback. MongoDB defaults to no TLS. SQLite does not use TLS.

| Mode | Behaviour |
| --- | --- |
| `disable` | No TLS |
| `allow`, `prefer` | TLS without certificate checks. PostgreSQL and MySQL can fall back to an unencrypted connection. MongoDB has no fallback |
| `require` | TLS without certificate checks |
| `verify-ca` | TLS with certificate authority checks |
| `verify-full` | TLS with certificate authority and host name checks |

An unknown non-empty `sslmode` string skips the profile and produces a report.

## Project file

A repository can contain `.masume.toml` with shared profiles and queries.

masume searches the startup directory and then each parent directory. The nearest `.masume.toml` is the project file. The client, `masume run` and `masume --mcp` read the project file.

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

A project profile accepts the connection keys of a user profile, except the keys below. A profile that sets one of these is skipped, even when the value is empty:

| Key | Meaning |
| --- | --- |
| `password_command` | A shell command run on connect |
| `command` | A shell command run on connect |
| `password_env` | An environment variable |
| `secret`, `secret_ref` | A secret store |

`auth = "prompt"` and `auth = "keyring"` are allowed. Other password sources require a user profile. Project profiles accept `env`, `mode`, `confirm_writes` and `write_plan`.

Other sections, including `[secret]`, `[ui]`, `[keys]`, `[ai]` and `[mcp]`, are reported and ignored.

### Project queries

`[query.NAME]` is a shared query, listed under `Ctrl+Q`:

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `sql` | string | required | The statement. A missing or empty statement skips the query and produces a report |
| `description` | string | | The query description, displayed instead of the SQL in the saved query list |
| `profiles` | list of strings | every profile | The profiles with access to the query. An empty list includes every profile |

### Overrides

A user profile replaces the project profile with the same name. A user query replaces the project query with the same name.

The connection picker marks project profiles with `project` and displays the project path. `d` cannot remove a project profile. `e` opens a project profile in the connection form; `Ctrl+S` saves a user override. The override retains the profile settings, including settings absent from the form.

`Ctrl+Q` lists project and user queries, sorted by name. Project queries have a `project` label. `Enter` loads the selected query into the editor. `Ctrl+D` cannot remove a project query. Removal of a project profile or query requires a project file edit.

## Write plans

`write_plan` measures an eligible single write before execution. PostgreSQL-family engines, MySQL-family engines and SQLite support write plans. MongoDB does not. The plan uses the target table and predicate from the statement. Unsupported statements, including joins, target aliases and batches, use the normal confirmation path.

The server counts matching rows with the write predicate. Counts can change before execution. An update plan also lists assigned columns. `cascades` lists trigger names and foreign key effects. The plan does not inspect trigger bodies or predict their effects. `blocked` lists foreign keys that can reject the delete.

`write_plan = "undo"` prepares an undo for eligible updates, deletes and truncates. An update undo restores assigned columns by primary key. A delete or truncate undo inserts captured target rows. `Alt+U`, the `global.undo-write` action, asks for confirmation and applies the undo statements together. The connection retains only the latest recorded write outcome in memory.

Undo capture reads target rows inside the write transaction. Row locking follows the engine's transaction rules. The write joins an existing transaction without committing that transaction. Otherwise, masume starts and commits the transaction. Engine DDL rules still apply, including implicit commits for MySQL-family `TRUNCATE`.

Undo covers only captured target rows. Cascaded rows and trigger effects are excluded. Undo statements can run triggers again. A later undo can overwrite newer values in the restored columns. Restoring deleted rows can fail on key or constraint conflicts.

Inserts, tables without primary keys, and updates that assign primary keys receive no undo. Plans with zero matching rows, failed counts, failed metadata reads, or counts above `undo_rows` also receive no undo. The plan gives the reason, and the write can still run after confirmation. Grid changes and imports do not retain an undo.

The default `undo_rows` is `1000`. `0` removes the configured limit but uses a capture ceiling of 1048576 rows. If a plan promises an undo, capture must succeed before execution. A capture error or truncated capture stops the write. A planning failure that offers a write without undo still allows the write after confirmation.

A connection without transaction support runs without undo and reports the reason. The AI chat and MCP use the same planning code. MCP write responses include captured undo SQL when available. [Usage](usage.md) covers write confirmation and undo commands.

## Passwords

`auth` is the password source:

| `auth` | Password source |
| --- | --- |
| `prompt` | The password dialog at connection time. The password stays in memory unless saved to the keyring |
| `keyring` | The keyring of the operating system, where masume stored the password earlier |
| `command` | The shell command in `password_command` |
| `secret` | The `[secret]` store in `secret`, at the reference in `secret_ref` |
| `password` | The environment variable in `password_env` |

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

A password command runs through `sh -c` with no stdin. The command must exit successfully within 30 seconds and print a non-empty first line. Later output is ignored.

masume ignores profile passwords in configuration files. A non-empty `password` string produces this warning:

```
profile "shop": passwords in files are ignored; enter the password and
select "remember in the keyring", or set password_env, password_command
or a [secret] store
```

Ignoring `password` does not change `auth`. The configured environment variable, command, secret store or keyring still applies. The client prompts only when the selected source and engine require a prompt. SQLite needs no password. MongoDB without a user does not prompt unless `auth = "prompt"`. A test from the connection form asks for the password through the same dialog, and keeps none of it.

`masume run` and `masume --mcp` cannot prompt. A required password must come from a non-interactive source. Saving a profile removes an existing `password` assignment from that profile block.

### Keyring

Linux keyring access uses Secret Service over D-Bus, supported by GNOME Keyring and KWallet. macOS uses Keychain. Keyring entries use service `masume` and the profile name.

On a machine with a keyring, the password dialog includes a checkbox:

```
╭─ password ────────────────────────────────────╮
│ connecting to shop-prod · prod                │
│ reader@db.internal:5432/shop                  │
│                                               │
│ ••••••••••••                                  │
│                                               │
│ [x] remember in the keyring                   │
│                                               │
│ Enter connect · Esc cancel · Tab keyring      │
╰───────────────────────────────────────────────╯
```

`Tab` toggles the checkbox. After a successful connection, a checked box stores the password in the keyring. Saved profiles then use `auth = "keyring"`. A project profile requires a user override; masume does not edit the project file.

An `auth = "keyring"` profile with a missing password opens the dialog with the checkbox checked. A successful connection can replace the missing entry.

Command-line and detected passwords initially stay in memory. Saving a temporary profile can store its retained password in the keyring, even after selecting `auth = "prompt"`. Without a keyring, the saved profile uses `auth = "prompt"` and stores no password.

Removing a saved connection with `d` also removes its keyring entry.

Without a keyring, the TUI hides the checkbox and prompts when necessary. Headless commands cannot use this prompt fallback.

### Secret stores

A secret store is a reusable password command under `[secret.NAME]`. `{{ref}}` is the placeholder for the profile's `secret_ref`.

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `command` | string | required | The shell command that prints the secret on its first line. At least one unquoted, standalone `{{ref}}` argument is required |

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

A profile with `secret` and no `auth` uses `auth = "secret"`. The profile still requires `secret_ref` and normal connection fields.

Each `{{ref}}` becomes one shell-quoted argument. The template must leave the placeholder unquoted and separate from other text. Invalid placeholder positions are refused. A reference can contain spaces, quotes and semicolons without becoming shell syntax. Commands that need multiple inputs can call a script.

Templates accept literal arguments, quoted flags and pipelines. Shell expansion, escapes outside single quotes, redirects, command lists and multiline templates are refused. Complex commands require a script, with `{{ref}}` passed as an argument.

The invoked program still interprets its arguments. References must not be script text for `sh -c`, `eval`, or similar commands.

A failed store command reports the exit code and first stderr line when available. Password command requirements also apply: successful exit within 30 seconds, non-empty first output line, and no stdin.

A missing store skips the profile and produces a report. A missing command or invalid placeholder skips the store and produces a report.

Secret stores belong in the user configuration file. Project files cannot declare or reference secret stores.

MongoDB credentials require a user. A server without authentication can reject supplied credentials.

## Unix socket

PostgreSQL and MySQL also listen on a unix socket. A `host` that starts with `/` or `~/` is a socket path, the same convention as libpq and psql. `host = "socket"` resolves the default path of a local server.

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
| PostgreSQL | `/var/run/postgresql`, `/run/postgresql`, `/tmp`, each holding `.s.PGSQL.<port>` |
| MySQL | `/var/run/mysqld/mysqld.sock`, `/run/mysqld/mysqld.sock`, `/tmp/mysql.sock`, `/var/lib/mysql/mysql.sock` |

A PostgreSQL client dials the socket directory and `port` selects the file in it, so `/var/run/postgresql` and `/var/run/postgresql/.s.PGSQL.5432` resolve to the same connection. A MySQL client dials the file, and a directory resolves to `mysqld.sock` or `mysql.sock` in it. A path that resolves to no socket fails the connection, and the error lists the checked paths.

A socket carries no TLS, so `sslmode` is ignored. A socket with `ssh_host` is refused: an SSH tunnel forwards TCP only. The PostgreSQL-protocol and MySQL-protocol engines take a socket; the rest do not. SQLite opens a file and needs no host.

## SSH tunnel

A profile can reach its database server through an SSH tunnel. masume opens the tunnel in process and runs no `ssh` binary. The SSH server connects to `host` and `port`.

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

masume opens a local forward: a listener on `127.0.0.1` with an ephemeral port, forwarded to `host:port` over the SSH connection. The listener and the SSH client close with the database connection. The picker and the title bar show `host:port`, not the local endpoint.

Authentication order: `ssh_key`, then `ssh_password_env`, then the agent on `$SSH_AUTH_SOCK`. An encrypted `ssh_key` needs `ssh_key_passphrase_env`, or the key loaded in the agent.

Host key verification is strict. An unknown host key or a missing known hosts file fails the connection with an error naming the path. `ssh-keyscan` appends a host key.

TLS certificates are verified against `host`, so `sslmode = "verify-full"` works through the tunnel. A tunneled MongoDB connection is direct, so masume uses no other replica set member.

The connection form has an `ssh tunnel` toggle. Set it to `on` and the form shows the SSH fields, writes them to the profile, and `Ctrl+T` tests the connection through the tunnel. Set it to `off` and saving removes every `ssh_` key from the profile.

Project files cannot set `ssh_password_env` or `ssh_key_passphrase_env`.

## Connection command

A profile can start a shell command before connection, such as an SSH tunnel. The command must stay in the foreground. masume stops the running process group when the connection closes or connection setup fails. Detached or daemonized processes are not reliably cleaned up.

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

masume checks `host:wait_for_port` for a TCP connection until the timeout. `wait_for_port` is a readiness check; `port` remains the database connection port. Without `wait_for_port`, masume connects immediately after starting the command. `command_timeout` is the readiness timeout, not the command lifetime. A profile without `command` performs no readiness check.

## Interface

```toml
[ui]
icons               = "plain"
theme               = "tokyonight"
hide_system_schemas = true
key_hints           = "full"
```

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `icons` | `plain` or `ascii` | `plain` | The base glyph set. See [Icons](#icons) |
| `theme` | string | `ayu-dark` | A built-in theme name, a file name in `themes/` without `.toml`, or `system` for the colours of the terminal. See [themes.md](themes.md) |
| `hide_system_schemas` | boolean | `true` | `false` displays system schemas, including `pg_catalog` and `information_schema`. `h` in the tree toggles visibility for the session |
| `key_hints` | `full`, `main` or `off` | `full` | How many key hints the status bar, the title bar, the tab row, the pane strips and the pane borders draw. An unknown mode produces a report and uses `full`. See [Key hint modes](#key-hint-modes) |

### Icons

`icons = "plain"` is the default set. `icons = "ascii"` is ASCII-only. An unknown set produces a report and uses `plain`.

`[ui.icon_glyphs]` overlays individual glyphs on that set. An empty string hides that kind. An unknown kind produces a report. A Nerd Font glyph belongs here; the shipped sets stay one column wide without that font.

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

A terminal without a Nerd Font draws those example glyphs as empty boxes.

| Kind | Drawn for | `plain` | `ascii` |
| --- | --- | --- | --- |
| `schema` | A schema in the object tree | `◇` | `~` |
| `table` | A table | `▦` | `T` |
| `view` | A view | `◈` | `V` |
| `materialized-view` | A materialized view | `◆` | `M` |
| `function` | A function | `ƒ` | `f` |
| `sequence` | A sequence | `№` | `S` |
| `type` | A type | `⊞` | `Y` |
| `trigger` | A trigger | `⚑` | `!` |
| `column` | A column | `·` | `.` |
| `index` | An index | `▤` | `#` |
| `primary-key` | A primary key | `◆` | `*` |
| `foreign-key` | A foreign key | `→` | `>` |
| `role` | A role | `●` | `o` |
| `roles` | The roles folder | `●` | `o` |
| `favourites` | The favourites folder | `★` | `*` |
| `recent` | The recent folder | `↻` | `@` |
| `query` | A saved query | `≡` | `=` |
| `folder` | A folder | `▸` | `>` |
| `plan` | A query plan | `⊳` | `>` |
| `note` | A notice | `⚠` | `!` |
| `problem` | A problem | `✗` | `x` |
| `ai` | The AI chat | `✦` | `*` |
| `fold-closed` | A closed fold | `▸` | `>` |
| `fold-open` | An open fold | `▾` | `v` |
| `field` | A form field marker | `▸` | `>` |
| `close` | A close control | `×` | `x` |
| `dot` | A status dot | `●` | `o` |
| `sort-up` | Ascending sort | `↑` | `^` |
| `sort-down` | Descending sort | `↓` | `v` |
| `prompt` | A prompt marker | `❯` | `>` |
| `step-back` | A step back | `‹` | `<` |
| `step-on` | A step forward | `›` | `>` |
| `banner` | A banner | `⚑` | `!` |
| `new-tab` | A new tab | `+` | `+` |

`[ui.palette]`, `[ui.colors]` and `[ui.syntax]` overlay the selected theme. See [themes.md](themes.md) for colour names, token kinds and inheritance.

### Key hint modes

| Mode | Hints |
| --- | --- |
| `full` | Every key hint of the status bar, the title bar, the tab row, the pane strips, the pane borders and the cards |
| `main` | The primary key hints only. See the list below |
| `off` | No key hints. Every bar, strip, border and card keeps its readouts: the count of the statements, the place of the caret, the count of the faults, the rows of the result, what a card is for |

`main` shows the primary keys: the key a pane is there for (open a tree row, run the statement, run all, run a notebook cell), the key that opens the menu of the row under the cursor, the keys a state raises (cancel a running read, run a failed one, fetch more rows, count the rows, edit a table as a query), and the keys no other key reaches (show a hidden tree, step through the connections). On a card it shows the keys that answer it and the key that closes it, without the extras. The chat card shows `ask`, `last reply query to editor` and `close`. The notebook card shows `open` and `close`. The title bar, the pane borders and the plan strip keep their keys. The tab row and the step keys of the result strips show none.

`full` and `main` show the keys that reach the model: `ask ai` on the title bar, the one key of the model on the border of the editor, and `ask ai` on the strip of the plan. `off` hides all three.

Every mode shows the chords in a menu row, the palette and the help card, the answer chips of a question, and a report that carries the key it is answered with, such as the key that undoes a write. A key a mode hides still works, and the palette (`^K`) and the help card (`?`) reach every action in every mode.

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
| `preset` | string | `default` | The initial key set. `default` is the only preset |

Each table below `[keys]` is a scope. Each scope entry binds an action to one chord or a chord list. An empty list removes the binding. Unlisted actions retain the preset bindings.

| Scope | Where its keys apply |
| --- | --- |
| `global` | Across the client, subject to focused pane bindings and dialog handling |
| `tree` | The object tree on the left |
| `grid` | The result grid |
| `editor` | The query editor |
| `plan` | The query plan tree |
| `notebook` | The cell list of a notebook tab |
| `document` | The tree that shows result rows as documents |
| `list` | Any list inside a card: the history, the saved queries, the palette |
| `dialog` | A card that asks a question, and the connection picker |

`alt`, `meta` and `option` are names for the same modifier. Unknown actions and invalid chords produce reports and retain the preset binding. [Keys](keys.md) lists the actions and defaults. [Usage](usage.md) covers the corresponding workflows.

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
| `enabled` | boolean | `true` | `false` disables the AI chat and its interface elements. MCP remains separate |
| `default_provider` | `anthropic` or `openai` | `anthropic` | The initial AI chat provider. The palette changes the provider for the session. An unknown name produces a report |
| `statement_timeout_ms` | integer above zero | `30000` | Execution timeout for AI chat `run_query`, in milliseconds. Other tools and undo capture are outside this timeout |

One table per provider, `[ai.providers.anthropic]` and `[ai.providers.openai]`:

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `model` | string | `claude-opus-5`, `gpt-5` | The provider model ID |
| `api_key` | string | empty | The API key stored directly in the file. A non-empty value takes priority over `api_key_env` |
| `api_key_env` | string | empty | The environment variable with the API key. No variable name is assumed by default |
| `base_url` | string | empty | The provider or proxy address. A non-empty value takes priority over `base_url_env` |
| `base_url_env` | string | empty | The environment variable with the provider or proxy address. No variable name is assumed by default |

Without a configured address, Anthropic uses `https://api.anthropic.com/v1` and OpenAI uses `https://api.openai.com/v1`. masume removes trailing slashes and appends `/v1` unless the configured address already ends with `/v1`. Requests then use `/messages` for Anthropic or `/responses` for OpenAI.

Unknown provider tables produce reports and are ignored. See [ai.md](ai.md) for provider data and credential sources.

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
| `profiles` | list of strings | empty | The allowed profiles. An empty list serves no profiles |
| `access` | `off`, `read-only`, `read-write` or `full` | `read-only` | The global access limit. Profile `mcp` can lower the limit. `mode = "read-only"` limits access to reads |
| `row_limit` | integer above zero | `500` | Maximum returned rows for `run_query`. Catalog results, undo rows and changed rows are outside this limit |
| `timeout_ms` | integer above zero | `30000` | Execution timeout for MCP `run_query`, in milliseconds. Other tools and undo capture are outside this timeout |

An unknown `access` level and a `row_limit` or `timeout_ms` that is not above zero keep the default without a report.

See [mcp.md](mcp.md) for tools and write confirmation.

## Notebooks

`[notebooks]` adds directories to the notebook list. The list always holds `<project root>/.masume/notebooks` and `$XDG_STATE_HOME/masume/notebooks`.

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

`XDG_CONFIG_HOME` is the configuration base directory. `XDG_STATE_HOME` is the state base directory, normally `~/.local/state`. Keyring entries are separate from these files. See [security](../SECURITY.md#stored-data) for permissions, retention and diagnostic limits.
