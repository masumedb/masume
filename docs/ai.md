# AI chat

Each connection has one AI chat. The chat uses that connection. It uses the database tools shared with the [MCP server](mcp.md). It does not use `list_profiles`.

Opening the panel sends nothing. The first submitted question sends the system prompt. It sends the connection summary. It sends the tool schemas and the question. All of them go to the configured provider.

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

Closing the panel leaves a running reply in place. It leaves a pending statement in place. `Ctrl+X` does not undo a completed database operation.

The palette has **Ask AI: explain this query**. It has **Ask AI: optimize this query**. It has **Ask AI: build a notebook**. It has the provider picker. Those have no default chord.

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

`default_provider` is `anthropic` or `openai`. The palette changes the provider for the session. Switching providers keeps the conversation. The next question sends those messages to the new provider.

`api_key_env` is the environment variable for the API key. `api_key` is a key stored in the config file. It takes priority. A file with `api_key` holds a secret.

`base_url` and `base_url_env` set a gateway. The direct value takes priority. The gateway receives the API key. It receives the request content. See [configuration](configuration.md#ai) for defaults, `/v1` rewriting, and the key table.

`enabled = false` hides the chat. It hides its actions. The loader still reads `[ai]`. It reads API keys too. `[mcp]` is separate. A project file cannot set `[ai]`.

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

The catalog summary holds no table names. It holds no column names. The prompt tells the model to call `list_tables` and `describe_table`. The model can skip those tools.

On PostgreSQL, the named namespaces are schemas of the connected database.

An unchanged editor buffer is not a new message. The earlier context stays in the conversation. It is sent again with that history.

Tool results return to the provider in later rounds of the same reply. Later questions keep user and assistant text. They do not keep a separate history of tool calls. They do not keep a separate history of tool results.

## Tools

One question allows 25 provider rounds. Each round can ask for several tool calls. A run that hits the limit with no text reply stops.

| Tool | Result |
| --- | --- |
| `list_tables` | Table or collection names, kinds, and row estimates |
| `describe_table` | Columns or fields, types, defaults, choices, and foreign keys where they exist |
| `list_indexes` | Index names and definitions |
| `list_constraints` | Constraint names and definitions |
| `get_table_ddl` | Table or collection creation statements |
| `list_relationships` | Foreign keys into and out of tables |
| `validate_query` | Adapter diagnostics. SQL uses preparation where the engine supports it; MongoDB uses local diagnostics. An open transaction returns `checked: false` |
| `explain_query` | An estimated or analyzed plan |
| `plan_write` | Row counts, assigned columns, trigger names, foreign-key effects, and undo information |
| `run_query` | Execution status, returned rows, and optional undo statements |

`run_query` returns unmasked values. Grid masking does not apply. Undo statements can hold old row values. Plans can hold secrets. Defaults can hold them. Constraints can hold them. Errors can hold them.

MongoDB `describe_table` samples up to 100 documents per collection. It returns inferred field names. It returns inferred field types.

`explain_query` with `analyze` true executes a statement classified as a read. `plan_write` runs counts that evaluate the write predicate.

## Confirmation and limits

The chat asks before every `run_query` call. This includes reads. It does not use `confirm_writes`. It also asks before `explain_query` of a statement classified as a write. `plan_write` runs without that question.

`write_plan` adds a plan when the profile supports measurement. It adds it when the engine supports measurement. It adds it when the statement supports it. After execution, `Alt+U` opens the undo. See [write plans](configuration.md#write-plans).

The chat uses the profile `mode`. It uses database permissions. `[mcp] access` does not apply. `[mcp] row_limit` does not apply. `[mcp] timeout_ms` does not apply.

`page_size` is the maximum returned rows for `run_query`. A tool argument can lower that cap. The cap does not bound changed rows. It does not bound database work.

`[ai] statement_timeout_ms` is the execution timeout for `run_query`. It does not cover calls to the provider. It does not cover confirmation. It does not cover catalog calls. It does not cover validation. It does not cover explain, write-plan measurement, or undo capture. A profile `statement_timeout_ms` can apply separately. Cancellation can fail. A statement can remain active after a timeout.

Read-only profiles use the same client checks. They use the same engine protection as the rest of the client. See [read-only access](engines.md#read-only-access).

## Notebooks

**Ask AI: build a notebook** asks what the notebook is to cover. It sends that question. It opens the reply as a notebook. Prose becomes text cells. One statement cell comes per query. A parameter cell holds the `:name` marks the statements bind. Nothing runs.

`Ctrl+J` inserts the statement of the last reply. On a notebook tab it becomes a new cell under the focused one. On any other tab it goes into a query editor.

`Ctrl+G` turns the conversation into a notebook. The prose of every turn becomes a text cell. Every statement the model wrote becomes a statement cell. The notebook opens unsaved. It runs nothing. See the [notebook guide](notebooks.md).

## Storage

The provider or gateway receives the system prompt. It receives the tool schemas. It receives questions, editor contexts, stored messages, and tool results.

The history file stores conversations. It stores their editor contexts. It stores 50 conversations per profile. It stores 100 messages per stored conversation. Those limits do not cap the current conversation in memory. The conversation title is the first question. It is truncated at 90 characters.

Runs of `run_query` also enter query history. This includes failures. `Ctrl+T` opens query history.

`$XDG_STATE_HOME/masume/ai-chat.log` records chat traffic. Tool arguments are truncated after 500 Unicode characters. Tool results are truncated too. Rotation uses a 2,000,000-byte threshold. It keeps one `.1` backup. See [SECURITY.md](../SECURITY.md#diagnostic-logs).

## Caching

Each call still transmits its content. Anthropic marks the last content block of the call. It uses `cache_control: ephemeral`. OpenAI sends `prompt_cache_key`. This is `masume/` plus the profile name. Provider cache rules and prices apply.