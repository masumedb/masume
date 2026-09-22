# AI chat

Each connection has one AI chat. The chat runs on that connection and uses the same database tools as the [MCP server](mcp.md), except `list_profiles`.

Opening the panel sends nothing. The first question sends the system prompt, the connection summary, the tool schemas, and the question to the configured provider.

## Keys

These are the default keys. See [keys.md](keys.md) for other bindings.

| Key | Action |
| --- | --- |
| `Ctrl+I` | Open the chat |
| `Alt+I` | Open the chat with the trimmed editor buffer in the input, unsent |
| `Ctrl+H` | Open the chat and ask about the editor error or failed check. Sends only when the editor is not empty |
| `i` in Plan | Send the displayed raw plan to the chat |
| `Enter` | Submit the input |
| `Shift+Enter` or `Alt+Enter` | Insert a newline |
| `Ctrl+X` | Stop the reply, reject a pending statement, and keep the text already received |
| `Ctrl+L` | Start a new conversation |
| `Ctrl+O` | Open the conversation list |
| `Ctrl+D` in the list | Remove the selected conversation |
| `Ctrl+A` | Copy the last reply to the clipboard |
| `Ctrl+J` | Insert the last statement from the conversation into the editor, without running it |
| `Ctrl+R` | Ask the last question again |
| `Ctrl+G` | Open the conversation as a notebook, without running cells |

Closing the panel leaves a running reply and a pending statement in place. `Ctrl+X` does not undo a completed database operation.

The palette has **Ask AI: explain this query**, **Ask AI: optimize this query**, **Ask AI: build a notebook**, and the provider picker. These have no default chord.

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

[ai.providers.grok]
model       = "grok-4.6"
api_key_env = "XAI_API_KEY"

[ai.providers.openai_compatible]
model    = "qwen3-coder:30b"
base_url = "http://localhost:11434"
```

`default_provider` is `anthropic`, `openai`, `grok` or `openai_compatible`. The provider picker in the palette changes the provider for the session. The conversation stays, and the next question sends it to the new provider.

`api_key_env` is the environment variable with the API key. `api_key` is the key itself, stored in the config file, and takes priority. A config file with `api_key` contains a secret.

`base_url` and `base_url_env` set a gateway address, and `base_url` takes priority. The gateway receives the API key and the request content. See [configuration](configuration.md#ai) for defaults, `/v1` rewriting, and the key table.

The [settings screen](usage.md#settings) edits these settings in the client. **Settings** in the palette opens it.

`enabled = false` hides the chat and its actions. masume still reads `[ai]` and its API keys. `enabled` does not affect `[mcp]`. A project file cannot set `[ai]`.

## Agents

An agent is a coding agent that masume runs over the [Agent Client Protocol](https://agentclientprotocol.com/), such as Claude Code, Codex, Gemini CLI, OpenCode, Goose or Qwen Code. The agent uses its own subscription and model. masume sends it no API key.

Each agent has its own table. The config file has tables for `claude`, `codex`, `gemini` and `opencode`. `claude` and `codex` start with `npx`, which comes with Node. `gemini` and `opencode` start directly. An agent with a command that is not on the PATH is listed as not installed, and the chat shows this before the first question. The [settings screen](usage.md#settings) adds and removes agents. An agent with no command is reported and skipped.

```toml
[ai]
default_agent = "claude"

[ai.agents.claude]
command = "npx"
args    = ["@zed-industries/claude-code-acp"]
```

masume runs the command as a child process and exchanges JSON-RPC messages with it over stdin and stdout. The protocol version is 1.

An agent runs in its own process and accepts tools only as MCP servers. masume therefore serves the chat tools to the agent over an MCP server. The server listens on the loopback address for one question and needs a bearer token. It runs the tools on the chat connection. It refuses a request with an `Origin` header, such as one from a browser page. The token is never written to the log.

Providers and agents use the same tool list. A provider calls the tools in the same process. An agent calls them over the loopback server. `masume --mcp` serves the same list for several profiles, so it adds a `profile` argument and the tools that find a connection.

`[mcp]` does not apply to the chat. `[mcp]` configures `masume --mcp`, the server for agents outside masume. The chat follows the connection settings: `mode` sets whether the agent can write, and `write_plan` still applies before a write runs. `confirm_writes` does not apply; see [Confirmation and limits](#confirmation-and-limits). A profile with a password prompt also works, because the agent uses the open connection.

The agent must support MCP servers over a URL. Claude Code, Codex and OpenCode do. An agent without this support fails at the handshake, and the error shows the agent name.

`model` in the agent table is the model. An empty `model` keeps the model configured in the agent. `Ctrl+T` on an agent page loads the agent model list, and the model row then cycles through it.

Model names follow the agent. Claude Code uses `default`, `sonnet` and `haiku`. OpenCode uses a provider and a model, such as `opencode-go/deepseek-v4-flash`.

`default_agent` is the agent the chat uses. `set as default` on an agent page in the [settings screen](usage.md#settings) writes it. **AI agent** in the palette changes it for the current session only.

Each question opens and closes one agent session, so the agent keeps no history between questions. The prompt has the same instructions, earlier turns and question that a provider receives.

Key differences from a provider:

| Piece | Provider | Agent |
| --- | --- | --- |
| Model | `model` under the provider table | Chosen by the agent |
| Credentials | `api_key` or `api_key_env` | Agent subscription |
| Tools | masume tools, run by masume | masume MCP server, plus the agent's own tools |
| Step limit | `max_tool_steps` | Agent limit |
| Token counts | Reported per question | Reported per question, if the agent sends them |
| Writes | Write plan and confirmation | Write plan and confirmation for masume tools. Permission request for the agent's own tools |

`env` entries are added to the masume environment for the agent process. An entry with an empty value sets that variable to empty. `CLAUDECODE=` clears the variable that stops Claude Code from running inside another Claude Code session.

masume does not implement the ACP file and terminal methods, so it refuses `fs/read_text_file`, `fs/write_text_file` and the terminal methods. An agent that reads files or runs commands does it itself, under its own settings.

An agent is not limited to masume tools. It has its own system prompt, its own file and command tools, and the MCP servers from its own config file. The chat confirmation and `write_plan` apply to `run_query`, not to a database command the agent runs in its own shell. A provider accesses the database only through masume tools.

An agent asks permission before it runs a tool it does not know. masume allows its own tools without a question, and `run_query` then asks for confirmation itself. masume passes every other permission request to the user. The panel shows the request like a statement confirmation, with the action and its input. `n` refuses the action.

## Local models

`grok` sends requests to `/chat/completions` on `https://api.x.ai/v1`. `base_url` sets another address.

`openai_compatible` sends requests to `/chat/completions` on any server with the OpenAI chat endpoint, including Ollama, LM Studio, llama.cpp and vLLM. It has no default address, so `base_url` is required. `model` is a model name on that server. `api_key` is optional. A request has an `Authorization` header only when `api_key` or `api_key_env` has a key.

| Server | `base_url` |
| --- | --- |
| Ollama | `http://localhost:11434` |
| LM Studio | `http://localhost:1234/v1` |
| llama.cpp | `http://localhost:8080` |
| vLLM | `http://localhost:8000` |

The chat sends the schema context and every tool schema with each question. One answer can take up to 25 provider rounds. A small model hits that limit more often than a hosted model. `max_tool_steps` in the provider table changes the limit.

A local model needs tool calls to read the schema. A model without tool support answers from the system prompt only.

## The panel

A reply shows text and tool calls in the order the model produced them. A finished call has a check mark. The running call has a spinner and its elapsed time. A finished turn keeps its calls, so the reply shows what the model read.

`Ctrl+R` asks the last question again, for example after a failed or stopped reply. The question stays in the conversation, and the new answer follows it.

`Ctrl+A` copies the last reply to the clipboard. `Ctrl+J` puts the last statement from the conversation into the editor. A later reply with prose only does not replace that statement. The key hint shows only after the chat has written a statement.

The input field grows with the question, from one row to six. The faint line under it shows the last action or the session token counts.

## Request content

| Piece | Content |
| --- | --- |
| System prompt | Role, answer format, tool-use instructions, dialect, default schema or database, up to 300 other schema or database names from the loaded catalog, and `ai_instructions` when set |
| Tools | Tool names, descriptions, and argument schemas |
| Question | Submitted text |
| Editor | Full buffer, trimmed, truncated at 4000 Unicode characters. Typed questions, `Alt+I` input, error text, raw plans, and tool results are not truncated |
| Error | Last execution error, when the editor is nonempty and the last run failed |
| Plan | Full raw plan, when the Plan action starts the question |
| History | Earlier user and assistant text, with the editor context stored on those turns |

The catalog summary has no table or column names. The prompt tells the model to call `list_tables` and `describe_table`, but the model can skip them.

On PostgreSQL, the named namespaces are schemas of the connected database.

An unchanged editor buffer is not sent as new context. The earlier context stays in the history and is sent again with it.

Tool results go back to the provider in later rounds of the same reply. Later questions keep the user and assistant text, but not the tool calls or tool results.

## Tools

One question allows 25 provider rounds, or the provider's `max_tool_steps`. Each round can request several tool calls. A run that reaches the limit without a text reply stops.

| Tool | Result |
| --- | --- |
| `list_tables` | Table or collection names, kinds, and row estimates |
| `describe_table` | Columns or fields, types, defaults, choices, and foreign keys where they exist |
| `list_indexes` | Index names and definitions |
| `list_constraints` | Constraint names and definitions |
| `get_table_ddl` | Table or collection creation statements |
| `list_relationships` | Foreign keys into and out of tables |
| `validate_query` | Adapter diagnostics. SQL is prepared where the engine supports it; MongoDB uses local diagnostics. An open transaction returns `checked: false` |
| `explain_query` | An estimated or analyzed plan |
| `plan_write` | Row counts, assigned columns, trigger names, foreign-key effects, and undo information |
| `run_query` | Execution status, returned rows, and optional undo statements |

`run_query` returns unmasked values; grid masking does not apply. Undo statements can contain old row values. Plans, defaults, constraints, and errors can contain secrets.

MongoDB `describe_table` samples up to 100 documents per collection and returns inferred field names and types.

`explain_query` with `analyze` true executes a statement classified as a read. `plan_write` runs counts that evaluate the write predicate.

## Confirmation and limits

The chat asks for confirmation before every `run_query` call, reads included, and ignores `confirm_writes`. It also asks before `explain_query` on a statement classified as a write. `plan_write` needs no confirmation.

A statement the chat classifies as a read runs inside a read-only transaction where the engine has one, so a routine it calls cannot write. See [read-only access](engines.md#read-only-access).

`write_plan` adds a plan when the profile, the engine, and the statement support measurement. After execution, `Alt+U` opens the undo. See [write plans](configuration.md#write-plans).

The chat uses the profile `mode` and database permissions. `[mcp] access`, `row_limit`, and `timeout_ms` do not apply.

`page_size` is the row limit for `run_query` results. A tool argument can lower it. The limit does not cap the rows a write changes or the work the server does.

`[ai] statement_timeout_ms` is the execution timeout for `run_query`. It does not cover calls to the provider, confirmation, catalog calls, validation, explain, write-plan measurement, or undo capture. A profile `statement_timeout_ms` can apply separately. Cancellation can fail, and a statement can keep running after a timeout.

Read-only profiles use the same client checks and engine protection as the rest of the client. See [read-only access](engines.md#read-only-access).

## Notebooks

**Ask AI: build a notebook** asks for the notebook topic and sends it as a question. The reply opens as a notebook: prose becomes text cells, and each query becomes a statement cell. A parameter cell has the `:name` parameters of the statements. No cell runs.

`Ctrl+J` inserts the last statement from the conversation. On a notebook tab, the statement becomes a new cell below the focused cell. On any other tab, it goes into a query editor.

`Ctrl+G` turns the conversation into a notebook. The prose of each turn becomes a text cell, and each statement from the model becomes a statement cell. The notebook opens unsaved and runs nothing. See the [notebook guide](notebooks.md).

## Storage

The provider or gateway receives the system prompt, the tool schemas, questions, editor contexts, stored messages, and tool results.

The history file stores conversations and their editor contexts: up to 50 conversations per profile and 100 messages per conversation. These limits do not apply to the current conversation in memory. The conversation title is the first question, truncated at 90 characters.

Every `run_query` call, failed calls included, is also saved in query history. `Ctrl+T` opens query history.

`$XDG_STATE_HOME/masume/ai-chat.log` records chat traffic. Tool arguments and results are truncated after 500 Unicode characters. The log rotates at 2,000,000 bytes and keeps one `.1` backup. See [SECURITY.md](../SECURITY.md#diagnostic-logs).

## Caching

Each call still sends its full content. For Anthropic, masume marks the last content block with `cache_control: ephemeral`. For OpenAI, masume sends `prompt_cache_key`, which is `masume/` plus the profile name. Provider cache rules and prices apply.