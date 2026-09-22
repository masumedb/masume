# Notebooks

A notebook is an ordered list of cells over one connection. The cells share one parameter set and one run policy. The file is Markdown.

The [key reference](keys.md#notebook) lists the bindings of the cell list.

## Opening

`Alt+B` opens an empty notebook. `Alt+Shift+N` does the same where the terminal reports Shift. `Alt+O n` opens the notebooks card, which lists project notebooks and user notebooks. A notebook from `[notebooks] paths` shows its directory.

Enter opens the selected row in this tab. `Alt+Enter` opens it in a new tab. `n` opens an empty notebook. `e` renames the file. `d` deletes it after confirmation. The filter also matches the directory. The filter `project` lists the project notebooks.

`masume ./review.masume.md` opens a notebook file by path. The connection comes from the arguments.

Opening a notebook runs no cell. Every cell is marked `not run` until a run key is pressed.

## Cell list

A notebook tab has two panes. The cell list takes the place of the query tab editor, with the result pane below it. Each row shows the cell state, number, kind, name, and last result. An open cell shows its text under its row.

Up and Down move between cells. `Enter` puts the caret in the focused cell. `Esc` returns to the list. Inside a cell every editor key works, including completion, formatting, comments, find and replace, and local checks.

The result pane shows the result of the focused cell. It has the views, sort, filters, and row editing of a query tab. Sort and filters are kept per cell. `o` folds one cell. `O` folds every cell.

The mouse wheel scrolls the list, and so does dragging the scroll bar on its right. The list keeps its scroll position until the focus moves. A click on a row selects that cell. A second click on the same cell opens it.

`b` adds a cell below. `a` adds a cell above. Both ask for the kind first, then open the cell. `c` changes the kind of an existing cell. `t` names the cell by writing a comment on its first line. `d d` deletes the cell. `u` undoes a list change. `K` and `J` move the focused cell up and down. `y`, `x`, and `p` copy, cut, and paste a cell.

A statement cell gets completion and checks against the catalog. A prose cell and a parameter cell get no completion, no problem row, and no statement colour.

## Cell kinds

| Kind | Contains | Runs |
| --- | --- | --- |
| `sql` | One or more statements of the engine | Yes, in order, one result per statement |
| `md` | Prose | No |
| `param` | Parameter values for every cell | Binds values |
| `chart` | A source cell, a label column and a value column | Draws the result of the source cell |

A statement cell takes its name from its first `--` comment line. A cell with three statements shows three numbered results.

A parameter cell contains one `name = value` line per parameter. Every `:name` placeholder in every cell takes its value from here. A placeholder without a value opens the same parameter form as a query tab.

A value in single quotes, double quotes, or no quotes is text; the quotes are not part of the value. A number is a number. `true` is a boolean.

```
day = '2026-09-01'
region = "EU"
status = paid
limit = 100
paid = true
```

A chart cell draws the result of another cell. It has no text of its own. A negative value is drawn down from the zero line.

To draw one, run the statement cell first, then add a cell with `b` and choose `chart`. The form starts with the cell above as the source. The label is the first non-number column of its result, and the value is the first number column. Up and Down move between rows. Left and Right change the value of a row. `Ctrl+S` applies the form, and `Esc` closes it. `Enter` on a chart cell opens the same form again.

| Row | Values |
| --- | --- |
| source cell | Every statement cell of the notebook |
| label column | Source result columns, or the row number |
| value column | Source result columns |
| shape | `bar` draws one bar per row. `line` draws every value in one row of blocks |
| order | Result order, or by value, highest first |
| rows | Every row, or the first 5, 10, 20 or 50 |

The form shows the columns the source cell returned. A source cell that has not run shows none, and the form shows which cell to run.

## Cell references

A statement cell can reference another cell in place of a relation:

```sql
-- the total of the paid orders
select count(*) as orders, sum(total_cents) as cents from {{cell:paid-orders}} as paid
```

`{{cell:id}}` expands to the statement of that cell, in parentheses, before `:name` values are bound. No rows are reused: the referenced statement runs again inside this one. A missing cell, a self-reference, or a chain more than eight cells deep stops the run, and the error shows the reference. A MongoDB cell cannot reference another cell.

## Running

| Key | Runs |
| --- | --- |
| `r` | The focused cell |
| `R` | The focused cell and every cell below it |
| `Alt+R` | Every cell |
| `space` then `m` | The marked cells |
| `Ctrl+R` | The focused cell, or the selection inside it |
| `Ctrl+X` | Stops the run. No further cell is sent, whatever the error policy |

A notebook run uses the same batch runner as a query tab. It returns one result per statement, in order, with the same run identity, cancellation, and query history. Every write goes through the same checks: a read-only profile rejects it, `confirm_writes` still asks, and `write_plan` still measures. See [write plans](configuration.md#write-plans).

`Alt+O p` sets the run policy of this notebook.

| Setting | Values | Default | Meaning |
| --- | --- | --- | --- |
| Transaction | `autocommit`, `single` | `autocommit` | `single` begins one transaction before the first cell and commits after the last one. A failure leaves the transaction open. `Ctrl+L` and `Ctrl+U` close it |
| On error | `stop`, `continue` | `stop` | `stop` ends the run at the first failure. `continue` runs the cells after a failed one |

When the front matter `engine` differs from the connection engine, the run shows a warning and continues.

A tab keeps the results of one run only. Running one cell clears the rows of every other cell, and those cells are marked `stale · run again`.

## File format

A notebook is Markdown with a TOML front matter block and fenced code cells. Every fence with a known language is a cell, and the prose between two fences is a text cell. An unknown fence and an unknown front matter key are kept as written.

````
+++
title = "Weekly revenue review"
profiles = ["shop", "shop-prod"]
engine = "postgres"

[run]
transaction = "single"
on_error = "stop"
+++

# Weekly revenue review

Revenue by country for the trailing week. Every statement cell binds :day and :region.

```param
day = "2026-09-01"
region = "EU"
```

```sql id=countries-by-revenue
-- countries by revenue
select c.country, count(*) as orders, sum(o.total) as revenue
from orders o join customers c on c.id = o.customer_id
where o.placed_at >= :day and c.region = :region
group by c.country order by revenue desc
```

```chart source=countries-by-revenue label=country value=revenue kind=bar
```

```sql id=refresh-revenue-daily write=confirm
-- refresh revenue_daily
update revenue_daily set revenue = 0
```
````

`profiles` lists the profiles the notebook appears under. The notebook opens no connection of its own. `write=confirm` on a fence asks for confirmation before that cell runs.

## Reports

`Alt+O r` writes a report of the notebook. It includes the prose, every statement, the rows each cell returned, and every chart as a block of text. The path is the notebook path with a Markdown extension, or a path typed into the field. A masked column stays masked. Files are created with mode `0600`.

`Ctrl+S` exports the result of the focused cell as CSV, and `Ctrl+G` as JSON.

## Locations

| Kind | Location |
| --- | --- |
| Project | `<project root>/.masume/notebooks/*.masume.md`, beside the nearest `.masume.toml` |
| Personal | `$XDG_STATE_HOME/masume/notebooks/*.masume.md`, beside the history file |
| Extra | Directories in `[notebooks] paths` of the config file |
| Any path | `masume ./one-off.masume.md`, or a path typed into the save field |

`Ctrl+P` saves the notebook. The first save asks for a name. A name without a directory is written to the project directory when a project file is present, and to the personal directory otherwise. Files are created with mode `0600` in a directory of mode `0700`.

A notebook file contains statements, prose, and parameter defaults, but no result rows. The open tabs of a profile keep the text of every notebook, so an unsaved notebook survives a restart. Results are not kept.

## AI chat

**Ask AI: build a notebook** in the palette asks for the notebook topic, then sends it to the model. The reply opens as a notebook. Prose becomes text cells, and each statement becomes a statement cell. A parameter cell lists the `:name` placeholders of those statements. The notebook opens unsaved, and no cell runs.

The prompt asks the model for one fenced block per query, each with a `-- name` line. A reply with no statement opens nothing and shows a report.

`Ctrl+J` in the chat inserts the latest statement of the conversation as a new cell below the focused one. `Ctrl+G` in the chat turns the conversation into a notebook: the prose of each turn becomes a text cell, and each statement from the model becomes a statement cell. See [AI chat](ai.md#notebooks).

## Headless mode

`masume nb run FILE` runs a notebook and writes the results. Profiles, timeouts, and the read-only check work as in [headless mode](headless.md).

```
masume nb run .masume/notebooks/revenue-review.masume.md \
  -p shop --param day=2026-09-01 -f markdown > review.md
```

| Argument | Meaning |
| --- | --- |
| `-p`, `--profile NAME` | Profile for the run |
| `-f`, `--format FORMAT` | `table`, `csv`, `json` or `markdown` |
| `-l`, `--limit ROWS` | Output cap per statement |
| `--param NAME=VALUE` | Parameter value. Overrides the parameter cell |
| `--only CELL` | Runs only the cell with that id. Repeat for more cells |
| `--explain` | Writes a JSON plan of every statement and runs none of them |
| `--allow-writes` | Runs the cells that write |

Headless runs have no write confirmation, no write plan, and no undo. A write cell needs `--allow-writes`.

| Code | Meaning |
| --- | --- |
| `0` | Success |
| `1` | A cell failed, or a write cell ran without `--allow-writes` |
| `2` | Argument, input file, password, or connection failure |
| `3` | A cell writes and the profile is read-only |

`markdown` writes the whole notebook as one report. It includes the prose, the statements, the rows of every cell as tables, and the charts as blocks of text. A masked column stays masked. See [headless notebooks](headless.md#notebooks).

## MCP

The [MCP server](mcp.md#notebooks) has `list_notebooks` and `read_notebook`; neither runs a cell. An agent runs a cell through `run_query`, where the access level and confirmation apply.