# MCP server

`masume --mcp` serves database tools to an external agent over stdin and stdout. The protocol is JSON-RPC 2.0, one message per line.

The process opens its own database connections, separate from a running terminal client. Each profile connects on its first database tool call.

```sh
masume --mcp
masume --mcp --profile=shop
masume --mcp --check
```

Protocol output goes to stdout. Startup reports and errors go to stderr, usually with the prefix `masume mcp: `. Check result lines and some argument help omit that prefix.

## Allowed profiles

`[mcp] profiles` is empty by default. An empty list serves no profile.

```toml
[mcp]
profiles = ["shop"]
```

`--profile=shop` restricts the server to `shop` among the allowed profiles and access settings. Connection tools then omit the `profile` argument. Without this option, every connection tool requires that argument, including when only one profile is allowed.

`list_profiles` returns profiles with effective access above `off`: names, engines, targets, configured databases, environments, descriptions, and access levels. A profile that needs a password prompt can appear as `unreachable`.

`--check` connects those same profiles and runs table discovery. Exit `1` means no profile was available or a checked connection failed. Exit `0` means every checked profile connected.

## Config files

The server reads the global config and the nearest `.masume.toml` in or above its working directory. A global profile replaces a project profile with the same name. Other project profiles join the list.

Only the global config supplies `[mcp]` and `[ai]`. A project file cannot add itself to `[mcp] profiles`. An allowed name can still match a project profile from the server's working directory.

The working directory is the process directory of the agent client. Project profiles permit `auth = "keyring"`. Keyring entries use the service `masume` and the profile name, with no host or project identifier.

The server has no password prompt. Available sources are environment variables, keyring entries, password commands, and named secret stores. Missing credentials that need a terminal prompt leave the profile unavailable. A literal profile `password` in a config file is ignored.

MCP stays available when `[ai] enabled = false`. MCP does not use the AI chat's provider credentials.

See [configuration](configuration.md#mcp) for the `[mcp]` key table.

## Registering the server

### Claude Code

`--scope user` registers the server for every project:

```sh
claude mcp add --scope user masume -- masume --mcp
```

### opencode

The server entry belongs under `mcp` in `opencode.json`. The global file is `~/.config/opencode/opencode.json`. Project settings merge with global settings and override matching keys.

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

`list_profiles` and `--check` then cover that profile alone, subject to the access settings.

## Tools and results

| Tool | Result |
| --- | --- |
| `list_profiles` | Allowed profiles and their effective access levels |
| `list_tables` | Table or collection names, kinds, and available row estimates |
| `describe_table` | Columns or fields, types, defaults, choices, and foreign keys where available |
| `list_indexes` | Index names and definitions |
| `list_constraints` | Constraint names and definitions |
| `get_table_ddl` | Table or collection creation statements |
| `list_relationships` | Foreign keys into and out of tables |
| `validate_query` | Adapter diagnostics |
| `explain_query` | An estimated or analyzed plan |
| `plan_write` | Available row counts, assigned columns, trigger names, foreign-key effects, and undo information |
| `run_query` | Execution status, returned rows, and optional undo statements |

`initialize` returns protocol version, server information, and tool capabilities. `tools/list` returns tool definitions. Profile and catalog data come from later tool calls.

Tool answers use text content. Successful answers are JSON text.

| Field | Meaning |
| --- | --- |
| nonempty `error` | MCP `isError: true` |
| handler failure | Plain error text with `isError: true` |
| denied `run_query` | `ran: false` and `reason`, with `isError` unset |
| execution failure | `ran: true` and `error`. `ran: true` is an attempt |
| `undo` | Reversal statements when available |
| `undo_reason` | Optional, when undo is unavailable |
| unknown tool or bad protocol | JSON-RPC error |

`validate_query` uses SQL preparation where available, and local diagnostics on MongoDB. An open transaction returns `checked: false`.

The [AI chat](ai.md) uses the same database tools, without `list_profiles`.

## Notebooks

`list_notebooks` returns the notebooks of the project and of the user: name, title, origin, cell count, and how many cells write. `read_notebook` returns one notebook with its run policy and every cell: id, kind, title, and text.

Both tools run no cell. A notebook whose front matter names profiles is listed only where one of those profiles is served. An agent runs a cell by sending its text through `run_query`, where the access level and the confirmation apply. See the [notebook guide](notebooks.md).

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
| `full` | Also `DELETE`, `DROP`, `TRUNCATE`, and writes classified as affecting every row |

The classifier uses statement structure. Effects it misses include:

- An `UPDATE` without `WHERE` needs `full`, even when the statement changes no rows.
- Creating a routine is a write, regardless of its body.
- MongoDB `runCommand` uses the command inside its document.
- Unrecognized `SET` and `RESET` settings are writes. Recognized settings such as `search_path`, time zones, and timeouts can be reads.
- Disabling read-only transactions and `BEGIN READ WRITE` are writes.
- MySQL and MariaDB executable comments use the statement inside the comment.

A profile can lower the global access level:

```toml
[profile.shop-prod]
mcp = "read-only"
```

`mode = "read-only"` also limits effective MCP access to reads. That profile mode applies outside MCP too.

MCP requests a read-only connection when effective access is read-only and the engine supports that profile mode. TiDB is the exception: MCP `access = "read-only"` on a writable profile keeps the writable session and applies client checks. An explicit `mode = "read-only"` TiDB profile still fails to connect. See [read-only access](engines.md#read-only-access).

`explain_query` with `analyze: true` executes a statement classified as a read. A write receives an estimated plan only, subject to access and confirmation. `plan_write` runs counts that evaluate the write predicate.

`row_limit` is the maximum returned rows for `run_query`. A call can request fewer rows. Catalog results, undo rows, changed rows, and plan results sit outside this cap.

`timeout_ms` is the execution timeout for `run_query`. Connection setup, confirmation, catalog calls, validation, explain, write-plan measurement, and undo capture sit outside it. A profile timeout can apply separately. Cancellation can fail, and a statement can remain active after a timeout.

## Confirming a write

MCP uses the profile `confirm_writes` setting after its access check. Statements classified as reads need no confirmation.

| `confirm_writes` | Confirmation |
| --- | --- |
| `off` | None |
| `delete` | Deletes, destructive statements, and writes classified as affecting every row |
| `write` | Every statement classified as a write |
| `agent` | Every classified write, with a plan token for clients without elicitation |

When unset, the defaults are `off` on `dev`, `delete` on `test`, and `write` on `prod`.

A client with elicitation support receives `elicitation/create` when confirmation is required. An accepted answer must contain `confirm: true`. The answer timeout is 120 seconds. A refusal or timeout leaves the statement unrun.

`write_plan` adds available measurements to the question. See [write plans](configuration.md#write-plans).

When undo is available, `run_query` returns reversal statements read inside the write transaction. The server leaves those statements unrun. Undo statements can hold old row values.

## Clients without elicitation

A client without elicitation is refused a statement that needs confirmation under `write` or `delete`. Statements that need no confirmation still run.

`confirm_writes = "agent"` issues a plan token when the client has no elicitation. Token issuance needs:

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
2. Present the plan and request human approval.
3. After approval, call `run_query` with the returned `token` as `plan_token`.

masume does not verify that a human saw the plan or approved the statement.

A token belongs to one profile and statement within one server process. It is single-use and expires after 10 minutes. Matching ignores outer whitespace after statement splitting. A change inside the statement fails to match, and the token stays available for the original statement.

A token stores no row snapshot and holds no lock. Data can change before execution. The access check still applies when the token returns.

A client with elicitation support receives no token. A supplied token is ignored, and required confirmation still uses elicitation. Profiles with `write` or `delete` issue no tokens.

## Data and logs

The external agent receives tool results and can send those results to its provider or gateway. The external client has its own storage and sharing rules.

Results are unmasked. Schema definitions, plans, errors, rows, and undo statements can hold secrets. MongoDB collection descriptions sample up to 100 documents and return inferred field names and types.

`$XDG_STATE_HOME/masume/mcp.log` records MCP traffic. Tool arguments and results are truncated after 500 Unicode characters. Rotation uses a 2,000,000-byte threshold and one `.1` backup. The initialize log reports elicitation support. See [SECURITY.md](../SECURITY.md#diagnostic-logs).

`run_query` attempts also enter query history, including failures. `Ctrl+T` opens that history in the terminal client.

## Testing

The protocol accepts one JSON object per line:

```sh
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe"}}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | masume --mcp
```
