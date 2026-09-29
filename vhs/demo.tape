# The README recording. Rebuild it with `mise run demo`. That task starts the postgres
# container, loads vhs/seed.sql into it, and runs this tape.
#
# vhs needs ttyd and ffmpeg on the PATH. See https://github.com/charmbracelet/vhs.
# The glyphs in vhs/config.toml need a Nerd Font, so FontFamily below is required.
# Width, Height and FontSize scale together. Raising all three keeps the same character
# grid and renders it with more pixels.

Output vhs/demo.mp4

Require masume

Set Shell bash
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 19
Set Width 1800
Set Height 1060
Set Padding 14
Set Framerate 24
Set Theme "ayu"
Set TypingSpeed 40ms

Hide
Type "export XDG_CONFIG_HOME=$PWD/.demo/config XDG_STATE_HOME=$PWD/.demo/state MASUME_DEMO_PASSWORD=secret" Enter
Type "clear" Enter
Show

Type "masume" Sleep 500ms Enter
Sleep 2s

# Pick the connection.
Enter
Sleep 3s

# Unfold a table in the tree. It shows the columns, with the primary and foreign keys marked.
Down Sleep 350ms
Down Sleep 350ms
Down Sleep 350ms
Down Sleep 350ms
Right
Sleep 2s

# Open the table. Each table view is one key press away.
Enter
Sleep 2500ms
Type "2" Sleep 2200ms
Type "3" Sleep 2200ms
Type "4" Sleep 2200ms
Type "5" Sleep 2500ms
Type "1" Sleep 800ms

# The object menu, and the ER diagram of the tables related to this one. Esc closes the
# diagram and then the menu it was opened from.
Type "m"
Sleep 1500ms
Down Sleep 400ms
Down Sleep 400ms
Down Sleep 800ms
Enter
Sleep 3500ms
Escape Sleep 500ms
Escape Sleep 800ms

# A new tab opens with the caret already in the editor.
Alt+n
Sleep 1200ms
Type "select c.country," Enter
Type "count(*) as orders," Enter
Type "sum(o.total) as revenue" Enter

# The completion menu suggests names from the catalog. Tab accepts the highlighted name.
Type "from ord"
Sleep 2200ms
Tab
Sleep 700ms
Type " o" Enter
Type "join cust"
Sleep 2200ms
Tab
Sleep 700ms
Type " c on c.id = o.customer_id" Enter
Type "group by c.country" Enter
Type "order by revenue desc"
Sleep 1200ms

# Run it.
Ctrl+r
Sleep 2500ms

# The plan, first estimated and then measured.
Ctrl+e
Sleep 3s
Ctrl+y
Sleep 3500ms
Type "1"
Sleep 1s

# The split view puts two tabs side by side: here the query and the table it reads.
Alt+v
Sleep 3s

# The explorer opens a table on the side with the focus.
Alt+p
Type "s"
Sleep 600ms
Up Sleep 350ms
Up Sleep 350ms
Up Sleep 800ms
Enter
Sleep 3s

# Alt+V again closes the split.
Alt+v
Sleep 1500ms

# The query builder writes a select from tables, joins and picked columns.
Alt+j
Sleep 1500ms
Type "orders"
Sleep 800ms
Enter
Sleep 1500ms

# The join follows the foreign key.
Type "t"
Sleep 1200ms
Enter
Sleep 1500ms
Enter
Sleep 1500ms

# Pick the country, then sum the totals and sort by the sum.
Down Sleep 300ms
Down Sleep 300ms
Down Sleep 600ms
Space
Sleep 1200ms
Left
Sleep 600ms
Enter
Sleep 1200ms
Right Sleep 400ms
Right Sleep 800ms
Down Sleep 300ms
Down Sleep 400ms
Right Sleep 400ms
Right Sleep 1000ms
Enter
Sleep 2s
Ctrl+r
Sleep 3500ms

# A notebook is cells of prose, values, statements and charts over one connection, kept as
# a Markdown file.
Alt+o
Type "n"
Sleep 2200ms
Enter
Sleep 2500ms

# Alt+R runs every cell, in order, one result each.
Alt+r
Sleep 3500ms

# The cell list moves with j and k. The result pane draws the result of the cell it stands
# on, and a chart cell draws the result of the cell it names.
Type "j" Sleep 900ms
Type "j" Sleep 1600ms
Type "j" Sleep 2500ms
Type "j" Sleep 2500ms

# The command palette searches actions by name.
Ctrl+k
Sleep 1200ms
Type "theme"
Sleep 1200ms
Enter
Sleep 1500ms

# Moving the cursor repaints the whole screen in the highlighted theme before you choose one.
Down Sleep 400ms
Down Sleep 1200ms
Down Sleep 1200ms
Down Sleep 1200ms
Down Sleep 1200ms
Down Sleep 1400ms

# The filter finds a light theme by name.
Type "latte"
Sleep 2500ms

# Esc cancels the preview, and the screen returns to the theme from the config file.
Escape
Sleep 2s
