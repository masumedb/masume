# AI chat

Each connection has one AI chat. The chat uses that connection and the database tools shared with the [MCP server](mcp.md), without `list_profiles`.

Opening the panel sends nothing. The first submitted question sends the system prompt, the connection summary, the tool schemas, and the question to the configured provider.

## Keys

These are the default keys. See [keys.md](keys.md) for other bindings.

| Key | Action |
| --- | --- |
| `Ctrl+I` | Open the chat |
| `Alt+I` | Open the chat with the editor buffer in the input, outer whitespace removed. Submission sends it |
| `Ctrl+H` | Open the chat and send a question about the editor error or failed check. Sends when the editor is nonempty |
| `i` in Plan | Send the displayed raw plan |
| `Enter` | Submit the input |
| `Shift+Enter` or `Alt+Enter` | Insert a newline |
| `Ctrl+X` | Stop the reply, reject a pending statement, and keep the text already received |
| `Ctrl+L` | Start a new conversation |
| `Ctrl+O` | Open the conversation list |
| `Ctrl+D` in the list | Remove the selected conversation |
| `Ctrl+J` | Insert SQL from the last reply into the editor, without execution |
| `Ctrl+G` | Open the conversation as a notebook, without running cells |

Closing the panel leaves a running reply and a pending statement in place. `Ctrl+X` does not undo a completed database operation.

The palette has **Ask AI: explain this query**, **Ask AI: optimize this query**, **Ask AI: build a notebook**, and the provider picker. Those have no default chord.

## Configuration

```toml
[ai]
enabled              = true
default_provider     = "anthropic"
statement_timeout_ms = 30000

[ai.providers.anthropic]
model       = "claude-opus-5"
api_key_env = "ANTHROPIC_API_KEY"

[ai.providers.openai]
model       = "gpt-5"
api_key_env = "OPENAI_API_KEY"
```

`default_provider` is `anthropic` or `openai`. The palette changes the provider for the session. Switching providers keeps the conversation. The next question sends the retained messages to the new provider.

`api_key_env` is the environment variable for the API key. `api_key` is a key stored in the config file and takes priority. A file with `api_key` holds a secret.

`base_url` and `base_url_env` set a gateway. The direct value takes priority. The gateway receives the API key and the request content. See [configuration](configuration.md#ai) for defaults, `/v1` rewriting, and the key table.

`enabled = false` hides the chat and its actions. The loader still reads `[ai]`, including API keys. `[mcp]` is separate. A project file cannot set `[ai]`.

## Request content

| Piece | Content |
| --- | --- |
| System prompt | Role, answer format, tool-use instructions, dialect, default schema or database, up to 300 other schema or database names from the loaded catalog, and `ai_instructions` when set |
| Tools | Names, descriptions, and argument schemas of the tools |
| Question | The submitted text |
| Editor | The full buffer, outer whitespace removed, truncated at 4000 Unicode characters. Typed questions, `Alt+I` input, error text, raw plans, and tool results are not truncated |
| Error | The last execution error, when the editor is nonempty and the last run failed |
| Plan | The full raw plan, when the Plan action starts the question |
| History | Earlier user and assistant text, with the editor context stored on those turns |

The catalog summary holds no table or column names. The prompt tells the model to call `list_tables` and `describe_table`. The model can skip those tools.

On PostgreSQL, the named namespaces are schemas of the connected database.

An unchanged editor buffer is not a new message. The earlier context stays in the conversation and is sent again with that history.

Tool results return to the provider in later rounds of the same reply. Later questions keep user and assistant text. They do not keep a separate history of tool calls and results.

## Tools

One question allows 25 provider rounds. Each round can request several tool calls. A run that hits the limit with no text reply stops.

| Tool | Result |
| --- | --- |
| `list_tables` | Table or collection names, kinds, and available row estimates |
| `describe_table` | Columns or fields, types, defaults, choices, and foreign keys where available |
| `list_indexes` | Index names and definitions |
| `list_constraints` | Constraint names and definitions |
| `get_table_ddl` | Table or collection creation statements |
| `list_relationships` | Foreign keys into and out of tables |
| `validate_query` | Adapter diagnostics. SQL uses preparation where available; MongoDB uses local diagnostics. An open transaction returns `checked: false` |
| `explain_query` | An estimated or analyzed plan |
| `plan_write` | Available row counts, assigned columns, trigger names, foreign-key effects, and undo information |
| `run_query` | Execution status, returned rows, and optional undo statements |

`run_query` returns unmasked values. Grid masking does not apply. Undo statements can hold old row values. Plans, defaults, constraints, and errors can hold secrets.

MongoDB `describe_table` samples up to 100 documents per collection and returns inferred field names and types.

`explain_query` with `analyze` true executes a statement classified as a read. `plan_write` runs counts that evaluate the write predicate.

## Confirmation and limits

The chat asks before every `run_query` call, including reads. It does not use `confirm_writes`. It also asks before `explain_query` of a statement classified as a write. `plan_write` runs without that question.

`write_plan` adds a plan when the profile, engine, and statement support measurement. After execution, `Alt+U` opens available undo. See [write plans](configuration.md#write-plans).

The chat uses the profile `mode` and database permissions. `[mcp] access`, `[mcp] row_limit`, and `[mcp] timeout_ms` do not apply.

`page_size` is the maximum returned rows for `run_query`. A tool argument can lower that cap. The cap does not bound changed rows or database work.

`[ai] statement_timeout_ms` is the execution timeout for `run_query`. It does not cover provider requests, confirmation, catalog calls, validation, explain, write-plan measurement, or undo capture. A profile `statement_timeout_ms` can apply separately. Cancellation can fail, and a statement can remain active after a timeout.

Read-only profiles use the same client checks and engine protection as the rest of the client. See [read-only access](engines.md#read-only-access).

## Notebooks

**Ask AI: build a notebook** asks what the notebook is to cover, sends that request, and opens the reply as a notebook: prose as text cells, one statement cell per query, and a parameter cell for the `:name` marks the statements bind. Nothing runs.

`Ctrl+J` inserts the statement of the last reply. On a notebook tab it becomes a new cell under the focused one. On any other tab it goes into a query editor.

`Ctrl+G` turns the conversation into a notebook: the prose of every turn becomes a text cell, and every statement the model wrote becomes a statement cell. The notebook opens unsaved and runs nothing. See the [notebook guide](notebooks.md).

## Storage

The provider or gateway receives the system prompt, tool schemas, questions, editor contexts, retained messages, and tool results.

The history file stores conversations and their editor contexts: 50 conversations per profile, 100 messages per stored conversation. Those limits do not cap the current conversation in memory. The conversation title is the first question, truncated at 90 characters.

`run_query` attempts also enter query history, including failures. `Ctrl+T` opens query history.

`$XDG_STATE_HOME/masume/ai-chat.log` records chat traffic. Tool arguments and results are truncated after 500 Unicode characters. Rotation uses a 2,000,000-byte threshold and one `.1` backup. See [SECURITY.md](../SECURITY.md#diagnostic-logs).

## Caching

Requests still transmit their content. Anthropic marks the last content block of the request with `cache_control: ephemeral`. OpenAI sends `prompt_cache_key` as `masume/` plus the profile name. Provider cache rules and prices apply.
