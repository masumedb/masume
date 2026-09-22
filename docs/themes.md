# Themes

`Alt+O t` opens the theme picker. Moving the cursor previews a theme. Enter applies it and writes `[ui] theme` to the user configuration file. Esc cancels. A save error is shown in the client.

```toml
[ui]
theme = "tokyonight"
```

The value is the theme file name without `.toml`. `ayu-dark` is the default and the fallback parent.

## Built-in themes

| File | Title | Appearance |
| --- | --- | --- |
| `ayu-dark` | Ayu Dark | dark |
| `masume-ember` | masume ember | dark |
| `masume-indigo` | masume indigo | dark |
| `masume-slate` | masume slate | dark |
| `ayu-mirage` | Ayu Mirage | dark |
| `tokyonight` | Tokyo Night | dark |
| `catppuccin-mocha` | Catppuccin Mocha | dark |
| `kanagawa` | Kanagawa | dark |
| `everforest-dark` | Everforest Dark | dark |
| `gruvbox-dark` | Gruvbox Dark | dark |
| `dracula` | Dracula | dark |
| `nord` | Nord | dark |
| `one-dark` | One Dark | dark |
| `night-owl` | Night Owl | dark |
| `oxocarbon` | Oxocarbon | dark |
| `monokai` | Monokai | dark |
| `github-dark` | GitHub Dark | dark |
| `rose-pine` | Rosé Pine | dark |
| `solarized-dark` | Solarized Dark | dark |
| `high-contrast` | High Contrast | dark |
| `catppuccin-latte` | Catppuccin Latte | light |
| `github-light` | GitHub Light | light |
| `one-light` | One Light | light |
| `ayu-light` | Ayu Light | light |
| `everforest-light` | Everforest Light | light |
| `tokyonight-day` | Tokyo Night Day | light |
| `gruvbox-light` | Gruvbox Light | light |
| `solarized-light` | Solarized Light | light |
| `rose-pine-dawn` | Rosé Pine Dawn | light |
| `high-contrast-light` | High Contrast Light | light |

`high-contrast` and `high-contrast-light` have the strongest contrast between text and background.

## System theme

```toml
[ui]
theme = "system"
```

masume uses the terminal background, foreground, and sixteen palette colours, and queries them about every two seconds. Live updates need a terminal that supports colour queries.

## Custom themes

A custom theme is a TOML file in `$XDG_CONFIG_HOME/masume/themes/`, normally `~/.config/masume/themes/`. The file name without `.toml` is the `[ui] theme` value. A custom file with a built-in name replaces that theme. `system` is reserved: a `system.toml` file is reported and ignored.

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
| `title` | Picker title. Defaults to the file name |
| `appearance` | `dark` or `light`. Inherited from the parent when absent, with `dark` as the final fallback |
| `extends` | Parent theme: a built-in or custom theme, not `system`. Defaults to `ayu-dark`, except in `ayu-dark` itself |

Child values override inherited palette entries, colours, and syntax properties. A missing parent or an inheritance cycle is reported. An inheritance chain has at most eight themes.

`[palette]` contains named hex colours. A palette value cannot refer to another name. Hex colours are `#RGB`, `#RGBA`, `#RRGGBB`, or `#RRGGBBAA`.

`[colors]` sets the colour roles. A value is a hex colour, a palette name, or another colour role. Examples are `border_focus = "blue"` and `border_focus = "accent"`.

`[ui.palette]`, `[ui.colors]`, and `[ui.syntax]` in the user configuration override the selected theme, and still apply after a theme change.

## Colour names

| Name | Meaning |
| --- | --- |
| `background` | Screen background |
| `panel` | Pane or card |
| `header` | Column header row |
| `zebra` | Every second grid row |
| `border` | Pane border |
| `border_focus` | Focused pane border |
| `selection` | Selected row or drag selection. Derived from `panel` and `text` when absent |
| `text` | Normal text |
| `muted` | Hint or label |
| `faint` | Line number or separator line |
| `accent` | Main highlight |
| `accent_alt` | Second highlight |
| `accent_warm` | Third highlight |
| `on_accent` | Text on an accent background. Derived for contrast when absent |
| `info` | Informational message |
| `success` | Successful statement |
| `warning` | Warning |
| `danger` | Destructive action |
| `error` | Failure |
| `env_dev` | Development title bar. Defaults to `success` when absent |
| `env_test` | Test title bar. Defaults to `warning` when absent |
| `env_prod` | Production title bar. Defaults to `danger` when absent |

## Syntax

A theme file uses `[syntax]`, and the user configuration uses `[ui.syntax]`. Each token kind is a table. Missing properties are inherited from the parent theme.

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
| `link` | Another token kind. The linked rule replaces the inherited rule, and local properties override it |

| Kind | Applies to |
| --- | --- |
| `keyword` | `SELECT`, `FROM`, and other keywords |
| `type` | Type name |
| `string` | Quoted string |
| `comment` | Comment |
| `number` | Number |
| `identifier` | Name |
| `quoted` | Quoted identifier |
| `operator` | Operator |
| `parameter` | `:name` placeholder |
| `problem` | Error found by the scanner |
| `bracket` | Bracket at the caret and its matching bracket |
| `guide` | Indent guide |
| `match` | Search match in the statement |