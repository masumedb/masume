# AI chat

Each connection has one AI chat. The chat uses that connection and the database tools shared with the [MCP server](mcp.md), but not `list_profiles`.

Opening the panel sends nothing. The first submitted question sends the system prompt, the connection summary, the tool schemas, and the question. All of them go to the configured provider.

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
| `Ctrl+A` | Copy the last reply to the clipboard |
| `Ctrl+J` | Insert the most recent statement of the conversation into the editor, without execution |
| `Ctrl+R` | Ask the last question again |
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

[ai.providers.openai_compatible]
model    = "qwen3-coder:30b"
base_url = "http://localhost:11434"
```

`default_provider` is `anthropic`, `openai` or `openai_compatible`. The palette changes the provider for the session. Switching providers keeps the conversation; the next question sends those messages to the new provider.

`api_key_env` is the environment variable for the API key; `api_key`, a key stored in the config file, takes priority. A file with `api_key` holds a secret.

`base_url` and `base_url_env` set a gateway, where the direct value takes priority. The gateway receives the API key and the request content. See [configuration](configuration.md#ai) for defaults, `/v1` rewriting, and the key table.

`enabled = false` hides the chat and its actions. The loader still reads `[ai]` and its API keys. `[mcp]` is separate. A project file cannot set `[ai]`.

## Agents

An agent is a coding agent masume reaches over the [Agent Client Protocol](https://agentclientprotocol.com/), such as Claude Code, Codex, Gemini CLI, OpenCode, Goose or Qwen Code. The agent uses its own subscription and its own model, and masume sends it no API key.

masume knows how to start `claude`, `codex` and `opencode`, so a config file needs no table for those three. `claude` and `codex` are started with `npx`, which comes with Node; `opencode` is started directly. The chat says so before the first question where the command of an agent is not on the PATH. A table changes an agent masume knows, and a table of any other name adds one.

```toml
[ai]
default_agent = "claude"

[ai.agents.claude]
command = "npx"
args    = ["@zed-industries/claude-code-acp"]
```

masume runs the command as a child process and speaks JSON-RPC on its standard input and output, at protocol version 1.

An agent runs in a process of its own and can only be given MCP servers, so masume serves the tools of the chat to it. The server stands on the loopback address for the length of one question, behind a bearer token, and it runs the tools on the connection the chat is open on. It refuses a request that carries an origin, which is how a page in a browser reaches a port, and the token is never written to the log.

One tool list answers a question whichever source reads it. A provider calls the tools in the same process, and an agent calls the same list over the loopback server. `masume --mcp` serves that list too, with a `profile` argument and the tools that find a connection, because it serves several.

`[mcp]` therefore does not reach the chat at all. That table governs `masume --mcp`, the server an agent outside masume connects to. The chat reads what the connection reads: `mode` sets whether the agent can write, and `confirm_writes` and `write_plan` still ask before a write runs. A profile that needs a password prompt works too, because the agent uses the connection that is already open rather than opening one of its own.

The agent must read an MCP server over a URL. Claude Code, Codex and OpenCode all do. An agent that does not is refused at the handshake, with a message naming it.

`model` under the agent table sets the model. masume reads the list the agent offers at session start and sets the model there, in whichever of the two shapes the agent uses. An empty `model` keeps the model the agent is configured with.

Model names follow the agent. Claude Code uses `default`, `sonnet` and `haiku`. OpenCode uses a provider and a model, such as `opencode-go/deepseek-v4-flash`.

`default_agent` sets the agent the chat sends to, and the client starts on it. The palette row AI agent changes it for the session alone.

Each question opens one session and closes it, so the agent keeps no history between questions. The prompt carries the same instructions, earlier turns and question that a provider receives.

Key differences from a provider:

| Piece | Provider | Agent |
| --- | --- | --- |
| Model | `model` under the provider table | The agent chooses it |
| Credentials | `api_key` or `api_key_env` | The subscription of the agent |
| Tools | The tools of masume, run by masume | The MCP server of masume, plus the tools of the agent |
| Step limit | `max_tool_steps` | The limit of the agent |
| Token counts | Reported per question | Reported per question, where the agent sends them |
| Writes | The write plan and its confirmation | The permission request of the agent |

An entry of `env` is added to the environment of masume, and an entry with an empty value replaces what that environment holds. `CLAUDECODE=` clears the variable that stops Claude Code from running inside another Claude Code session.

masume serves no file or terminal methods to an agent, so `fs/read_text_file`, `fs/write_text_file` and the terminal methods are refused. An agent that runs commands or reads files does so itself, under its own settings.

An agent is not limited to the tools of masume. It brings its own system prompt, its own file and command tools, and the MCP servers of its own config file. `confirm_writes` and `write_plan` guard `run_query`; they do not guard a database command the agent runs in a shell of its own. A provider reaches the database through the tools of masume alone.

An agent asks permission before it runs a tool it does not know. masume answers for its own tools itself, because it has rules for them: a read runs, and a write is asked about by the tool, which knows the profile and measures the rows. Every other action of the agent is put to the reader, since masume has no rules for it. The panel asks the same question the write plan asks, with the action and its input, and `n` refuses it.

## Local models

`openai_compatible` sends to `/chat/completions` on any server with the OpenAI chat endpoint, including Ollama, LM Studio, llama.cpp and vLLM. It has no default address, so `base_url` is required, and `model` is the model the server serves. `api_key` is optional; a request carries an `Authorization` header only when the config file has a key.

| Server | `base_url` |
| --- | --- |
| Ollama | `http://localhost:11434` |
| LM Studio | `http://localhost:1234/v1` |
| llama.cpp | `http://localhost:8080` |
| vLLM | `http://localhost:8000` |

The chat sends the schema context and every tool schema with each question, and one answer can run 25 provider rounds. A small model reaches that limit more often than a hosted model does. `max_tool_steps` under the provider table sets a different limit.

A local model needs tool calls to read the schema. A model served without tool support answers from the system prompt alone.

## The panel

A reply is drawn in the order the model worked: a block of text, then the call it made, then the next block. A call that finished carries a mark, and the one that runs carries the wheel and the time it has taken. A turn that ended keeps its calls, so the reply says what it read to answer.

`Ctrl+R` asks the last question again, which a reply that failed or was stopped needs. The question stays in the conversation and the new answer follows it.

`Ctrl+A` copies the last reply to the clipboard. `Ctrl+J` fills the editor with the most recent statement of the conversation. A question answered in prose does not hide the query of the answer before it, so a follow-up question costs nothing. The key is offered where the chat has written a statement.

The field grows with the question, from one row to six. The faint line under it reports the last action, or what the conversation has spent.

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

The catalog summary holds no table or column names. The prompt tells the model to call `list_tables` and `describe_table`; the model can skip those tools.

On PostgreSQL, the named namespaces are schemas of the connected database.

An unchanged editor buffer is not a new message; the earlier context stays in the conversation and is sent again with that history.

Tool results return to the provider in later rounds of the same reply. Later questions keep user and assistant text, but not a separate history of tool calls or tool results.

## Tools

One question allows 25 provider rounds, or the `max_tool_steps` of the provider. Each round can ask for several tool calls. A run that hits the limit with no text reply stops.

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

`run_query` returns unmasked values; grid masking does not apply. Undo statements can hold old row values. Plans, defaults, constraints, and errors can hold secrets.

MongoDB `describe_table` samples up to 100 documents per collection and returns inferred field names and types.

`explain_query` with `analyze` true executes a statement classified as a read. `plan_write` runs counts that evaluate the write predicate.

## Confirmation and limits

The chat asks before every `run_query` call, including reads; it does not use `confirm_writes`. It also asks before `explain_query` of a statement classified as a write. `plan_write` runs without that question.

`write_plan` adds a plan when the profile, the engine, and the statement support measurement. After execution, `Alt+U` opens the undo. See [write plans](configuration.md#write-plans).

The chat uses the profile `mode` and database permissions. `[mcp] access`, `row_limit`, and `timeout_ms` do not apply.

`page_size` is the maximum returned rows for `run_query`; a tool argument can lower that cap. The cap does not bound changed rows or database work.

`[ai] statement_timeout_ms` is the execution timeout for `run_query`. It does not cover calls to the provider, confirmation, catalog calls, validation, explain, write-plan measurement, or undo capture. A profile `statement_timeout_ms` can apply separately. Cancellation can fail. A statement can remain active after a timeout.

Read-only profiles use the same client checks and engine protection as the rest of the client. See [read-only access](engines.md#read-only-access).

## Notebooks

**Ask AI: build a notebook** asks what the notebook is to cover and sends that question. It opens the reply as a notebook. Prose becomes text cells, and one statement cell comes per query. A parameter cell holds the `:name` marks the statements bind. Nothing runs.

`Ctrl+J` inserts the most recent statement of the conversation. On a notebook tab it becomes a new cell under the focused one, and on any other tab it goes into a query editor.

`Ctrl+G` turns the conversation into a notebook. The prose of every turn becomes a text cell, and every statement the model wrote becomes a statement cell. The notebook opens unsaved and runs nothing. See the [notebook guide](notebooks.md).

## Storage

The provider or gateway receives the system prompt, the tool schemas, questions, editor contexts, stored messages, and tool results.

The history file stores conversations and their editor contexts: 50 conversations per profile and 100 messages per stored conversation. Those limits do not cap the current conversation in memory. The conversation title is the first question, truncated at 90 characters.

Runs of `run_query`, including failures, also enter query history. `Ctrl+T` opens query history.

`$XDG_STATE_HOME/masume/ai-chat.log` records chat traffic. Tool arguments and results are truncated after 500 Unicode characters. Rotation uses a 2,000,000-byte threshold and keeps one `.1` backup. See [SECURITY.md](../SECURITY.md#diagnostic-logs).

## Caching

Each call still transmits its content. Anthropic marks the last content block with `cache_control: ephemeral`; OpenAI sends `prompt_cache_key`, which is `masume/` plus the profile name. Provider cache rules and prices apply.