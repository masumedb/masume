<h1 align="center">升目 masume</h1>

<h3 align="center">A database client for the terminal</h3>

<p align="center">
  <em>Browse and query databases in the terminal. Share selected connection profiles with an AI agent.</em>
</p>

<p align="center">
  <a href="https://github.com/turanmahmudov/masume/actions/workflows/check.yml"><img src="https://github.com/turanmahmudov/masume/actions/workflows/check.yml/badge.svg" alt="check"></a>
  <img src="https://img.shields.io/badge/go-1.27+-00ADD8.svg?logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/license-Apache--2.0-green.svg" alt="License">
</p>

<p align="center">
  <code>mise use -g github:turanmahmudov/masume@latest</code>
</p>

<p align="center">
  <img src="vhs/demo.gif" alt="masume" />
</p>

---

### Browse

The object tree lists database objects supported by the engine. Table views include data, columns, indexes, constraints, DDL and query plans.

![The object tree](vhs/shots/01-object-tree.png)

### Diagram

An ER diagram shows a table and the tables it is linked to by foreign keys.

![An ER diagram of a table and its related tables](vhs/shots/07-er-diagram.png)

### Query

The editor has syntax highlighting and completion from the database catalog. Local checks and supported server checks mark detected errors before execution. A statement without a diagnostic can still fail.

![The SQL editor with the completion menu open](vhs/shots/08-completion.png)

### Results

Sort, filter, follow a foreign key, or freeze a column. Grid edits stay staged until SQL review and execution. Masking hides matching columns in the grid only; copies, exports and value viewers retain original values.

![A result grid](vhs/shots/09-result.png)

### Explain

Query plans are displayed as a tree, with estimated or measured costs.

![A query plan drawn as a tree](vhs/shots/10-plan.png)

### Notebooks

Cells of prose, values, statements and charts over one connection. Each cell keeps its own result and its own view. A notebook is a Markdown file, and `masume nb run` runs the file.

![A notebook of prose, values, statements and a chart](vhs/shots/13-notebook.png)

### Agents

masume has a built-in AI chat and an MCP server over stdio. The AI chat uses the current connection and asks before each query, including reads. MCP opens separate connections to explicitly allowed profiles. MCP access levels and profile settings apply to its queries and write confirmations.

Both interfaces share database tools, but their policies differ. See [AI data sharing](docs/ai.md), [MCP access](docs/mcp.md), and [security limits](SECURITY.md).

---

## Features

**Multiple engines:** PostgreSQL, MySQL, SQL Server, ClickHouse, SQLite and MongoDB, plus hosted services based on them

**Staged edits:** insert, edit, duplicate and delete supported table rows, then review the SQL. See [editing rows](docs/usage.md#editing-rows).

**Filters:** server predicates and filters on loaded rows. See [sorting and filters](docs/usage.md#sorting-and-filters).

**Named parameters:** a statement with `:name` placeholders opens a form for the values

**Export and copy:** CSV and JSON files. Clipboard formats also include Markdown, `INSERT` statements, row JSON and column `IN` clauses. See [copy and export](docs/usage.md#copy-and-export).

**Dump and restore:** schema and data as a SQL file. See [dump and restore](docs/usage.md#dump-and-restore).

**Import:** CSV or JSON into an existing or new SQL table. See [importing files](docs/usage.md#importing-files).

**Query history and saved queries:** history of statements and named queries. Restored tabs keep query text and settings, not result rows.

**Write plans:** counts and reverse SQL for eligible writes. See [write plans](docs/configuration.md#write-plans).

**Transactions:** begin, commit and rollback, or automatic begin with autocommit disabled

**Server dashboard:** sessions and metrics the engine supports. See [server activity](docs/usage.md#server-activity).

**Password sources:** prompt, keyring, environment variables, commands and named secret stores. Profile files do not store database passwords. See [credentials](SECURITY.md#credentials).

**Read-only profiles:** client checks, with extra protection on engines that support it. See [read-only access](docs/engines.md#read-only-access).

**MCP server:** `masume --mcp` serves selected profiles over stdio, with an access level per profile and for the whole server

**AI chat:** questions about a statement, its error, or its query plan. Anthropic and OpenAI. `[ai] enabled = false` hides the chat.

**MongoDB:** a [subset of shell syntax](docs/engines.md#mongodb)

**Themes:** built-in themes, custom themes, or terminal colours

**Project profiles:** the nearest `.masume.toml` shares connections and saved queries. See [project file](docs/configuration.md#project-file).

---

## Install

Each command below installs the latest tagged release. The packages and archives are on the [releases page](https://github.com/turanmahmudov/masume/releases/latest).

### Script

```sh
curl -fsSL https://raw.githubusercontent.com/turanmahmudov/masume/master/install.sh | sh
```

The script puts `masume` in `~/.local/bin`.

### mise

```sh
mise use -g github:turanmahmudov/masume@latest
```

### npm

```sh
npm install -g masume
```

`npx masume` runs it without an install.

### Debian and Ubuntu

```sh
sudo dpkg -i masume_0.0.4_linux_amd64.deb  # adapt the version and the architecture
```

### Fedora and RHEL

```sh
sudo rpm -i masume_0.0.4_linux_amd64.rpm  # adapt the version and the architecture
```

### Alpine

The packages are unsigned, so `apk` needs `--allow-untrusted`.

```sh
sudo apk add --allow-untrusted masume_0.0.4_linux_amd64.apk  # adapt the version and the architecture
```

### Archive

Unpack the `tar.gz` for the platform and put `masume` on the PATH.

### Go

```sh
go install github.com/turanmahmudov/masume@latest
```

Go 1.27 or later builds it from the module proxy.

### From source

```sh
git clone https://github.com/turanmahmudov/masume.git
cd masume
mise install
mise run install
```

## Usage

```text
masume                                  open the client
masume TARGET                           open a connection, postgres://you@host/shop
masume --profile NAME                   open a user or project profile
masume --detect                         open detected container databases
masume run [TARGET | -p NAME] STATEMENT run statements
masume nb run [TARGET | -p NAME] FILE   run a notebook
masume dump [TARGET | -p NAME] FILE     dump schema and data
masume restore [TARGET | -p NAME] FILE  restore a dump
masume --mcp                            serve allowed MCP profiles
masume --mcp --profile=NAME             serve one allowed MCP profile
masume --mcp --check                    check enabled MCP profiles
masume --version                        print the version
```

With no target or profile, masume opens `$DATABASE_URL`.

Supported URLs are not complete native driver connection strings. Most native URL options are ignored. See [connection targets](docs/usage.md#connection-targets).

### Headless mode

```sh
masume run -p shop-prod -f json 'select count(*) from orders'
masume run -p shop -e ./reports/daily.sql --param day=2026-09-02
masume run ./notes.db -f csv 'select * from notes limit 100000' > notes.csv
```

See [headless mode](docs/headless.md) for formats, exit codes, dump, restore and notebooks.

### For teams

A repository can contain `.masume.toml` with shared profiles and queries:

```toml
[profile.dev]
engine   = "postgres"
host     = "127.0.0.1"
database = "shop"
user     = "shop"
env      = "dev"

[query.recent-orders]
sql         = "select * from orders order by created_at desc limit 50"
description = "the newest 50 orders"
```

masume reads the nearest project file in or above the working directory. See [project file](docs/configuration.md#project-file) and [project security](SECURITY.md#project-files).

## First connection

The quickest first connection is a URL on the command line:

```sh
masume postgres://ada@127.0.0.1:5432/shop
```

The first interactive run creates a starter configuration file if none exists. In the picker, `n` adds a profile. `Ctrl+N` returns to the picker from a connection. Profiles can also be written directly:

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

`auth = "prompt"` asks for the password at connection time. The password stays in memory unless saved to the keyring.

## Docs

| Page | About |
| --- | --- |
| [User guide](docs/usage.md) | Workflows, navigation, editing, data transfer and troubleshooting |
| [Notebooks](docs/notebooks.md) | Cells, charts, run policy, the file format and `masume nb run` |
| [Configuration](docs/configuration.md) | Settings, defaults, profiles and password sources |
| [Engines](docs/engines.md) | Protocols and capabilities |
| [Keys](docs/keys.md) | Default bindings, scopes and overrides |
| [Themes](docs/themes.md) | Built-in themes, and how to write a custom one |
| [AI chat](docs/ai.md) | Providers, tools, what is sent to the provider |
| [MCP server](docs/mcp.md) | Tools, limits, confirming a write |
| [Headless mode](docs/headless.md) | `masume run` for scripts and CI |
| [Security](SECURITY.md) | Storage, data sharing and protection limits |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache License 2.0. See [LICENSE](LICENSE).
