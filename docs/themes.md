# Themes

`Alt+O t` opens the theme picker. Movement previews a theme, and Enter applies it. It writes `[ui] theme` in the user configuration file. Esc cancels. A save error appears in the client.

```toml
[ui]
theme = "tokyonight"
```

The value is the file name without `.toml`; `ayu-dark` is the default and the fallback parent.

## Built-in themes

| File | Title | Appearance |
| --- | --- | --- |
| `ayu-dark` | Ayu Dark | dark |
| `tokyonight` | Tokyo Night | dark |
| `catppuccin-mocha` | Catppuccin Mocha | dark |
| `gruvbox-dark` | Gruvbox Dark | dark |
| `dracula` | Dracula | dark |
| `nord` | Nord | dark |
| `one-dark` | One Dark | dark |
| `monokai` | Monokai | dark |
| `github-dark` | GitHub Dark | dark |
| `rose-pine` | Rosé Pine | dark |
| `solarized-dark` | Solarized Dark | dark |
| `catppuccin-latte` | Catppuccin Latte | light |
| `github-light` | GitHub Light | light |
| `one-light` | One Light | light |
| `gruvbox-light` | Gruvbox Light | light |
| `solarized-light` | Solarized Light | light |
| `rose-pine-dawn` | Rosé Pine Dawn | light |

## System theme

```toml
[ui]
theme = "system"
```

masume uses the terminal background, the foreground, and the sixteen palette colours. It queries those colours about every two seconds; updates need terminal support for colour queries.

## Custom themes

A custom theme is a TOML file in `$XDG_CONFIG_HOME/masume/themes/`, normally `~/.config/masume/themes/`. The file name without `.toml` is the `[ui] theme` value. A custom file with a built-in name replaces that theme. `system` is reserved; masume reports `system.toml` and ignores it.

```toml
title = "My Theme"
appearance = "dark"
extends = "tokyonight"

[palette]
ink  = "#c0caf5"
blue = "#7aa2f7"

[colors]
background   = "#16161e"
panel        = "#1a1b26"
border_focus = "blue"
text         = "ink"
accent       = "blue"
```

| Key | Meaning |
| --- | --- |
| `title` | The picker title. The file name is the default |
| `appearance` | `dark` or `light`. An absent value inherits from the parent, with `dark` as the final fallback |
| `extends` | The parent theme. A built-in or custom theme, not `system`. An absent parent uses `ayu-dark`, except in `ayu-dark` itself |

Child values override inherited palette entries, colours, and syntax properties. Missing parents and inheritance cycles produce reports. The inheritance chain is at most eight themes.

`[palette]` holds named hex colours. Palette values cannot reference other names. Hex colours are `#RGB`, `#RGBA`, `#RRGGBB`, or `#RRGGBBAA`.

`[colors]` holds the colour roles. A value is a hex colour, a palette name, or another colour role. Examples are `border_focus = "blue"` and `border_focus = "accent"`.

`[ui.palette]`, `[ui.colors]`, and `[ui.syntax]` in the user configuration overlay the selected theme. The overlay remains after a theme change.

## Colour names

| Name | Meaning |
| --- | --- |
| `background` | The background of the whole screen |
| `panel` | A pane or a card |
| `header` | The row of column names |
| `zebra` | Every second row of the grid |
| `border` | A pane border |
| `border_focus` | The border of the focused pane |
| `selection` | A selected row or drag selection. Derived from `panel` and `text` when absent |
| `text` | Normal text |
| `muted` | A hint or a label |
| `faint` | A line number or a separator line |
| `accent` | The main highlight |
| `accent_alt` | A second highlight |
| `accent_warm` | A third highlight |
| `on_accent` | Text on an accent background. Derived for contrast when absent |
| `info` | An informational message |
| `success` | A statement that succeeded |
| `warning` | A warning |
| `danger` | A destructive action |
| `error` | A failure |
| `env_dev` | The development title bar. Defaults to `success` when absent |
| `env_test` | The test title bar. Defaults to `warning` when absent |
| `env_prod` | The production title bar. Defaults to `danger` when absent |

## Syntax

A theme file uses `[syntax]`. The user configuration uses `[ui.syntax]`. Each token kind is a table. Missing properties inherit from the parent theme.

```toml
[syntax]
keyword    = { fg = "accent", bold = true }
comment    = { fg = "muted", italic = true }
parameter  = { fg = "danger" }
problem    = { fg = "error", underline = true }
bracket    = { fg = "on_accent", bg = "accent_alt" }
guide      = { bg = "header" }
match      = { fg = "on_accent", bg = "accent_warm" }
```

| Key | Meaning |
| --- | --- |
| `fg` | Foreground: a hex colour or a colour name |
| `bg` | Background: a hex colour or a colour name |
| `bold` | Boolean |
| `italic` | Boolean |
| `underline` | Boolean |
| `link` | Another token kind. The linked rule replaces the inherited rule, then local properties overlay it |

| Kind | Applies to |
| --- | --- |
| `keyword` | `SELECT`, `FROM`, and other keywords |
| `type` | A type name |
| `string` | A quoted string |
| `comment` | A comment |
| `number` | A number |
| `identifier` | A name |
| `quoted` | A quoted identifier |
| `operator` | An operator |
| `parameter` | A `:name` placeholder |
| `problem` | An error found by the scanner |
| `bracket` | The bracket at the caret, and its matching bracket |
| `guide` | The indent guide of a line |
| `match` | A search match in the statement |