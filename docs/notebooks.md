# Notebooks

A notebook is an ordered list of cells over one connection. The cells share one parameter set. They share one run policy. The file is Markdown.

The [key reference](keys.md#notebook) lists the bindings of the cell list.

## Opening

`Alt+B` opens an empty notebook. `Alt+Shift+N` does the same where the terminal reports Shift. `Alt+O n` opens the notebooks card. It lists project notebooks and user notebooks. A notebook from `[notebooks] paths` names that directory.

Enter opens the selected row in this tab. `Alt+Enter` opens it in a new tab. `n` opens an empty notebook. `e` renames the file. `d` deletes it after a question. The filter also matches the directory. `project` lists project notebooks.

`masume ./review.masume.md` opens a notebook file by path. The connection comes from the arguments.

Opening a notebook runs no cell. Every cell is marked `not run` until a run key is pressed.

## Cell list

A notebook tab has two panes. The cell list sits where a query tab draws its editor. The result pane sits under it. Each row shows the cell state, number, kind, name, and last result. An open cell draws its text under that row.

Up and Down move between cells. `Enter` puts the caret in the focused cell. `Esc` returns to the list. Inside a cell every editor key works. These include completion. They include formatting, comments, find and replace, and local checks.

The result pane draws the result of the focused cell. It has the views, sort, filters, and row editing of a query tab. Sort and filter belong to that cell. `o` folds one cell. `O` folds every cell.

The wheel moves the list. A drag of the bar at its right does the same. The list stays where the wheel left it until the focus moves. A press on a row selects that cell. A second press on the same cell opens it.

`b` adds a cell below. `a` adds a cell above. Both ask for the kind first. Then they open the cell. `c` changes the kind of an existing cell. `t` names the cell. It writes a comment on its first line. `d d` deletes the cell. `u` undoes a list change. `K` and `J` move the focused cell. `y`, `x`, and `p` copy, cut, and paste a cell.

A statement cell is completed. It is checked against the catalog. A prose cell is not. A parameter cell is not. They get no completion. They get no problem row. They get no statement colour.

## Cell kinds

| Kind | Holds | Runs |
| --- | --- | --- |
| `sql` | One or more statements of the engine | Yes, in order, one result per statement |
| `md` | Prose | No |
| `param` | The values every cell binds | Binds values |
| `chart` | A source cell, a label column and a value column | Draws the result of the source cell |

A statement cell takes its name from its first `--` comment line. A cell with three statements shows three numbered results.

A parameter cell holds one `name = value` line per parameter. Every `:name` mark of every cell binds from these values. A mark without a value opens the same form a query tab opens.

A value in single quotes is text. A value in double quotes is text. A value in no quotes is text. The quotes are not part of the value. A number is a number. `true` is a boolean.

```
day = '2026-09-01'
region = "EU"
status = paid
limit = 100
paid = true
```

A chart cell draws the result of another cell. It holds no text of its own. A value below zero draws from the zero line.

To draw one, run the statement cell first. Then add a cell with `b`. Choose `chart`. The form opens on the source cell above it. It opens on the first two columns of its result. A non-number is the label. The first number is the value. Up and Down move between rows. Left and Right change the value of a row. `Ctrl+S` applies it. `Esc` closes it. `Enter` on a chart cell opens the same form again.

| Row | Values |
| --- | --- |
| source cell | Every statement cell of the notebook |
| label column | The columns of the source result, or the row number |
| value column | The columns of the source result |
| shape | `bar` draws one bar per row. `line` draws every value in one row of blocks |
| order | The order of the result, or the value, highest first |
| rows | Every row, or the first 5, 10, 20 or 50 |

The form shows the columns the source cell returned. A source cell that has not run shows none. The form names the cell to run.

## Cell references

A statement cell can name another cell in place of a relation:

```sql
-- the total of the paid orders
select count(*) as orders, sum(total_cents) as cents from {{cell:paid-orders}} as paid
```

`{{cell:id}}` expands to the statement of that cell in parentheses. This happens before `:name` values are bound. It carries no rows. The named cell's statement runs again inside this one. A missing cell stops the run. A self-reference stops it. A chain more than eight cells deep stops it. The run names the reference. A MongoDB cell cannot name another cell.

## Running

| Key | Runs |
| --- | --- |
| `r` | The focused cell |
| `R` | The focused cell and every cell below it |
| `Alt+R` | Every cell |
| `space` then `m` | The marked cells |
| `Ctrl+R` | The focused cell, or the selection inside it |
| `Ctrl+X` | Stops the run. A stopped run sends no further cell, whatever the error policy says |

A run is the batch runner of a query tab with a longer plan. One result comes per statement, in order. It has the same run identity. It has the same cancellation. It has the same query history. Every write goes through the same guard. A read-only profile rejects a write. `confirm_writes` still asks. `write_plan` still measures. See [write plans](configuration.md#write-plans).

`Alt+O p` sets the run policy of this notebook.

| Setting | Values | Default | Meaning |
| --- | --- | --- | --- |
| Transaction | `autocommit`, `single` | `autocommit` | `single` begins one transaction before the first cell and commits after the last one. A failure leaves the transaction open. `Ctrl+L` and `Ctrl+U` close it |
| On error | `stop`, `continue` | `stop` | `stop` ends the run at the first failure. `continue` runs the cells after a failed one |

A notebook that names another engine reports the difference. It runs.

The store of a tab holds the results of one run. A run of one cell replaces the rows of the cells it did not run. Those cells are marked `stale · run again`. They point at no result of the new run.

## File format

A notebook is Markdown with a TOML front matter block. It has fenced code cells. Every fence with a known language is a cell. The prose between two fences is a text cell. An unknown fence is kept as written. An unknown front matter key is also kept as written.

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

`profiles` lists the profiles where the notebook appears. It opens no connection of its own. `write=confirm` on a fence asks before that cell.

## Reports

`Alt+O r` writes a report of the notebook. It includes the prose. It includes every statement. It includes the rows each cell returned. It includes every chart as a block of text. The path is the notebook file with a Markdown ending. Or it is a path typed into the field. A masked column stays masked. Files are created with mode `0600`.

`Ctrl+S` and `Ctrl+G` export the result of the focused cell. `Ctrl+S` exports it as CSV. `Ctrl+G` exports it as JSON.

## Locations

| Kind | Location |
| --- | --- |
| Project | `<project root>/.masume/notebooks/*.masume.md`, beside the nearest `.masume.toml` |
| Personal | `$XDG_STATE_HOME/masume/notebooks/*.masume.md`, beside the history file |
| Extra | The directories of `[notebooks] paths` in the config file |
| Any path | `masume ./one-off.masume.md`, or a path typed into the save field |

`Ctrl+P` writes the notebook. A notebook that was never saved asks for a name. A name without a directory is written to the project directory. This happens when a project file is present. Otherwise it is written to the personal directory. Files are created with mode `0600`. They are created in a directory of mode `0700`.

A notebook file holds statements. It holds prose. It holds parameter defaults. It holds no result rows. Open tabs of a profile keep the text of every notebook. An unsaved notebook survives a restart. Results are not stored.

## AI chat

**Ask AI: build a notebook** in the palette asks what the notebook is to cover. Then it asks the model. The reply opens as a notebook. Prose becomes text cells. Each statement becomes a statement cell. A parameter cell holds the `:name` marks those statements bind. The notebook opens unsaved. No cell runs.

It asks the model for one fenced block per query. It asks for a `-- name` line on each. A reply that holds no statement reports that. It opens nothing.

`Ctrl+J` in the chat inserts the statement of the last reply. It becomes a new cell under the focused one. `Ctrl+G` in the chat turns the conversation into a notebook. The prose of every turn becomes a text cell. Every statement the model wrote becomes a statement cell. See [AI chat](ai.md#notebooks).

## Headless mode

`masume nb run FILE` runs a notebook. It writes the results. Profile follows [headless mode](headless.md). Timeouts follow it. The read-only check follows it.

```
masume nb run .masume/notebooks/revenue-review.masume.md \
  -p shop --param day=2026-09-01 -f markdown > review.md
```

| Argument | Meaning |
| --- | --- |
| `-p`, `--profile NAME` | The profile of the run |
| `-f`, `--format FORMAT` | `table`, `csv`, `json` or `markdown` |
| `-l`, `--limit ROWS` | The output cap per statement |
| `--param NAME=VALUE` | A value over the value of a parameter cell |
| `--only CELL` | Runs only the cell of that id. Repeat for each cell |
| `--explain` | Writes a JSON plan of every statement and runs none of them |
| `--allow-writes` | Runs the cells that write |

Headless runs have no write confirmation. They have no write plan. They have no undo. A write cell needs `--allow-writes`.

| Code | Meaning |
| --- | --- |
| `0` | The run completed |
| `1` | A cell failed, or a write cell ran without `--allow-writes` |
| `2` | Argument, input file, password, or connection failure |
| `3` | The profile is read-only and a cell writes |

`markdown` writes the whole notebook as one report. It includes the prose. It includes the statements. It includes the rows of every cell as tables. It includes the charts as blocks of text. A masked column stays masked. See [headless notebooks](headless.md#notebooks).

## MCP

The [MCP server](mcp.md#notebooks) has `list_notebooks` and `read_notebook`. Both run no cell. An agent runs a cell through `run_query`. The access level applies. The confirmation applies.