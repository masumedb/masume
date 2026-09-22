# MCP server

`masume --mcp` serves database tools to an external agent over stdin and stdout. The protocol is JSON-RPC 2.0, one message per line.

These settings apply to external agents that connect to masume. They do not apply to the AI chat, which serves its own tools to its agent on the chat connection. See [agents](ai.md#agents).

The server process opens its own database connections, separate from any running terminal client. Each profile connects on its first database tool call.

```sh
masume --mcp
masume --mcp --profile=shop
masume --mcp --check
```

Protocol messages go to stdout. Startup reports and errors go to stderr, usually with the prefix `masume mcp: `. `--check` result lines and some argument help have no prefix.

## Allowed profiles

`[mcp] profiles` is empty by default. An empty list serves no profile.

```toml
[mcp]
profiles = ["shop"]
```

`--profile=shop` limits the server to `shop`, which must be an allowed profile. Access settings still apply. The connection tools then have no `profile` argument. Without `--profile`, every connection tool needs the argument, even when only one profile is allowed.

`list_profiles` returns profiles with effective access above `off`. It shows names, engines, targets, configured databases, environments, descriptions, and access levels. A profile that needs a password prompt can appear as `unreachable`.

`--check` connects the same profiles and lists their tables. Exit status `1` means there was no profile to check or a connection failed. Exit status `0` means every checked profile connected.

## Config files

The server reads the global config and the nearest `.masume.toml` in or above its working directory. A global profile replaces a project profile with the same name; other project profiles are added.

Only the global config can set `[mcp]` and `[ai]`. A project file cannot add its profiles to `[mcp] profiles`, but an allowed name can still match a project profile in the server's working directory.

The server runs in the working directory of the agent client. Project profiles can use `auth = "keyring"`. A keyring entry uses the service `masume` and the profile name only, with no host or project identifier.

The server cannot prompt for a password. It reads passwords from environment variables, keyring entries, password commands, and named secret stores. A profile that needs a terminal prompt is unusable. A literal `password` in a profile is ignored.

MCP still works when `[ai] enabled = false`. MCP does not use the AI chat's provider credentials.

See [configuration](configuration.md#mcp) for the `[mcp]` key table.

## Registering the server

### Claude Code

`--scope user` registers the server for every project:

```sh
claude mcp add --scope user masume -- masume --mcp
```

### opencode

The server entry goes under `mcp` in `opencode.json`. The global file is `~/.config/opencode/opencode.json`. Project settings merge with global settings and override matching keys.

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "masume": {
      "type": "local",
      "command": ["masume", "--mcp"],
      "enabled": true
    }
  }
}
```

### Cursor, Claude Desktop, and others

Cursor reads `~/.cursor/mcp.json`. Claude Desktop uses its own config file. These clients use this form:

```json
{
  "mcpServers": {
    "masume": { "command": "masume", "args": ["--mcp"] }
  }
}
```

### One profile per server

```sh
claude mcp add --scope user masume-shop -- masume --mcp --profile=shop
```

`list_profiles` and `--check` then cover only that profile. Access settings still apply.

## Tools and results

| Tool | Result |
| --- | --- |
| `list_profiles` | Allowed profiles and their effective access levels |
| `list_tables` | Table or collection names, kinds, and row estimates |
| `describe_table` | Columns or fields, types, defaults, choices, and foreign keys where they exist |
| `list_indexes` | Index names and definitions |
| `list_constraints` | Constraint names and definitions |
| `get_table_ddl` | Table or collection creation statements |
| `list_relationships` | Foreign keys into and out of tables |
| `validate_query` | Adapter diagnostics |
| `explain_query` | An estimated or analyzed plan |
| `plan_write` | Row counts, assigned columns, trigger names, foreign-key effects, and undo information |
| `run_query` | Execution status, returned rows, and optional undo statements |

`initialize` returns the protocol version, the server information, and the tool capabilities. `tools/list` returns the tool definitions. Profile and catalog data come from later tool calls.

Tool results are text content. A successful result is JSON text.

| Field | Meaning |
| --- | --- |
| nonempty `error` | MCP `isError: true` |
| handler failure | Plain error text with `isError: true` |
| denied `run_query` | `ran: false` and `reason`, with `isError` unset |
| execution failure | `ran: true` and `error`. `ran: true` means the statement was attempted |
| `undo` | Reversal statements when present |
| `undo_reason` | Reason for a missing undo, when present |
| unknown tool or bad protocol | JSON-RPC error |

`validate_query` prepares SQL where the engine supports it. It uses local diagnostics on MongoDB. An open transaction returns `checked: false`.

The [AI chat](ai.md) uses the same database tools. It does not use `list_profiles`.

## Notebooks

`list_notebooks` returns the project and user notebooks, with name, title, origin, cell count, and the number of cells that write. `read_notebook` returns one notebook with its run policy and every cell, each with id, kind, title, and text.

Neither tool runs a cell. A notebook with profiles in its front matter is listed only when one of those profiles is served. An agent runs a cell by sending the cell text through `run_query`, where the access level and confirmation apply. See the [notebook guide](notebooks.md).

## Access limits

```toml
[mcp]
profiles   = ["shop"]
access     = "read-only"
row_limit  = 500
timeout_ms = 30000
```

`access` is the maximum MCP access level. The default is `read-only`.

| `access` | Allowed statement classes |
| --- | --- |
| `off` | No profile tools |
| `read-only` | Statements classified as reads, plus catalog tools |
| `read-write` | Ordinary writes, including `INSERT`, filtered `UPDATE`, `CREATE`, `ALTER`, `GRANT`, and `REVOKE` |
| `full` | Also `DELETE`, `DROP`, `TRUNCATE`, statements that run a routine, and writes classified as affecting every row |

The classifier uses statement structure only. Special cases:

- An `UPDATE` without `WHERE` needs `full`, even when the statement changes no rows.
- A statement that creates or runs a routine needs `full`: `CALL`, `EXECUTE`, `DO`, and `CREATE` or `ALTER` of a function, a procedure or a trigger. masume does not read routine bodies.
- A write to a table with a trigger gets the same confirmation as a routine call. The write plan lists the trigger.
- MongoDB `runCommand` is classified by the command inside its document.
- A Redis `EVAL`, `EVALSHA` or `FCALL` needs `full`. `EVAL_RO`, `EVALSHA_RO` and `FCALL_RO` are reads.
- Unrecognized `SET` and `RESET` settings are writes. Recognized settings such as `search_path`, time zones, and timeouts can be reads.
- Disabling read-only transactions is a write. `BEGIN READ WRITE` is a write.
- MySQL and MariaDB executable comments are classified by the statement inside the comment.

A profile can lower the global access level:

```toml
[profile.shop-prod]
mcp = "read-only"
```

`mode = "read-only"` also limits effective MCP access to reads. The profile mode applies outside MCP too.

MCP opens a read-only connection when effective access is read-only. The connection fails on a server without read-only sessions, so a TiDB profile with read-only MCP access cannot connect. See [read-only access](engines.md#read-only-access).

A statement classified as a read runs inside a read-only transaction where the engine has one, so a routine it calls cannot write. See [read-only access](engines.md#read-only-access). An agent cannot keep a transaction open: `BEGIN` is a read, and its transaction ends with the statement.

`explain_query` with `analyze: true` executes a statement classified as a read; a write gets only an estimated plan, subject to access and confirmation. `plan_write` runs counts that evaluate the write predicate.

`row_limit` is the row limit for `run_query` results. A call can ask for fewer rows. The limit does not apply to catalog results, undo rows, changed rows, or plan results.

`timeout_ms` is the execution timeout for `run_query`. It does not cover connection setup, confirmation, catalog calls, validation, explain, write-plan measurement, or undo capture. A profile timeout can apply separately. Cancellation can fail, and a statement can keep running after a timeout.

## Confirming a write

MCP applies the profile `confirm_writes` setting after the access check. Statements classified as reads need no confirmation.

| `confirm_writes` | Confirmation |
| --- | --- |
| `off` | None |
| `delete` | Deletes, destructive statements, statements that run a routine, and writes classified as affecting every row |
| `write` | Every statement classified as a write |
| `agent` | Every statement classified as a write. Clients without elicitation get a plan token |

When unset, the defaults are `off` on `dev`, `delete` on `test`, and `write` on `prod`.

When confirmation is needed, a client with elicitation support receives `elicitation/create`. An accepted answer must contain `confirm: true`, and the timeout is 120 seconds. Any other answer, or a timeout, cancels the statement.

`write_plan` adds the measurements to the confirmation request. See [write plans](configuration.md#write-plans).

When an undo is available, `run_query` returns reversal statements, captured inside the write transaction. The server does not run them. Undo statements can contain old row values.

## Clients without elicitation

For a client without elicitation, the server refuses a statement that needs confirmation under `write` or `delete`. Statements that need no confirmation still run.

With `confirm_writes = "agent"`, a client without elicitation gets a plan token instead. The server issues a token only with:

- `write_plan` enabled on the profile
- an engine with write-plan support
- one recognized write with a target found in the connection catalog
- a supported target form, such as a simple `UPDATE`, `DELETE`, `TRUNCATE`, or recognized `INSERT`
- no unsupported target alias or multi-target form. Joined updates and deletes are not measured

MongoDB has no write-plan support. Unsupported statements return `measured: false` with no token. A measured plan can still have missing counts.

```toml
[mcp]
profiles = ["shop"]
access = "full"

[profile.shop]
engine = "postgres"
host = "127.0.0.1"
database = "shop"
user = "writer"
auth = "password"
password_env = "SHOP_PASSWORD"
mode = "write"
confirm_writes = "agent"
write_plan = "count"
```

`access = "full"` includes destructive statement classes.

The agent flow is:

1. Call `plan_write` with the statement.
2. Present the plan and ask for human approval.
3. After approval, call `run_query` with the returned `token` as `plan_token`.

masume does not verify that a human saw the plan or approved the statement.

A token is valid for one profile and one statement in one server process. It works once and expires after 10 minutes. Matching ignores leading and trailing whitespace after statement splitting. Any other change to the statement fails to match, and the token stays valid for the original statement.

A token stores no row snapshot and takes no lock, so data can change before execution. The access check runs again when the token is used.

A client with elicitation support gets no token. A supplied token is ignored, and confirmation still uses elicitation. Profiles with `write` or `delete` issue no tokens.

## Data and logs

The external agent receives tool results and can send them to its provider or gateway. The external client has its own storage and sharing rules.

Results are unmasked. Schema definitions, plans, errors, rows, and undo statements can contain secrets. MongoDB collection descriptions sample up to 100 documents and return inferred field names and types.

`$XDG_STATE_HOME/masume/mcp.log` records MCP traffic. Tool arguments and results are truncated after 500 Unicode characters. The log rotates at 2,000,000 bytes and keeps one `.1` backup. The `initialize` entry shows whether the client supports elicitation. See [SECURITY.md](../SECURITY.md#diagnostic-logs).

Every `run_query` call, failed calls included, is also saved in query history. `Ctrl+T` opens that history in the terminal client.

## Testing

The protocol accepts one JSON object per line:

```sh
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe"}}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | masume --mcp
```