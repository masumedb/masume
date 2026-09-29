# Screenshots of the client, for the README, a blog post or a store page.
# Rebuild them with `mise run shots`. They are written to vhs/shots/.
#
# This tape renders larger than vhs/demo.tape and holds each frame only long enough to
# capture it. So it is a screenshot run, not a recording. The mp4 that vhs must write
# goes to .demo, which git does not track.

Output .demo/shots.mp4

Require masume

Set Shell bash
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 27
Set Width 2560
Set Height 1500
Set Padding 20
Set Framerate 24
Set Theme "ayu"
Set TypingSpeed 25ms

Hide
Type "export XDG_CONFIG_HOME=$PWD/.demo/config XDG_STATE_HOME=$PWD/.demo/state MASUME_DEMO_PASSWORD=secret" Enter
Type "clear" Enter
Type "masume" Enter
Sleep 2s
Enter
Sleep 4s
Show

# The tree, unfolded down to a table.
Down Sleep 200ms
Down Sleep 200ms
Down Sleep 200ms
Down Sleep 200ms
Right
Sleep 2s
Screenshot .demo/shots/01-object-tree.png

# The rows of that table.
Enter
Sleep 3s
Screenshot .demo/shots/02-grid.png

Type "2" Sleep 2s
Screenshot .demo/shots/03-columns.png

Type "3" Sleep 2s
Screenshot .demo/shots/04-indexes.png

Type "5" Sleep 2s
Screenshot .demo/shots/05-ddl.png
Type "1" Sleep 1s

# The object menu, and the diagram that its ER diagram row opens.
Type "m" Sleep 2s
Screenshot .demo/shots/06-object-menu.png

Down Sleep 200ms
Down Sleep 200ms
Down Sleep 200ms
Enter Sleep 3s
Screenshot .demo/shots/07-er-diagram.png
Escape Sleep 500ms
Escape Sleep 800ms

# A partly typed statement with the completion menu open.
Alt+n
Sleep 1500ms
Type "select c.country," Enter
Type "count(*) as orders," Enter
Type "sum(o.total) as revenue" Enter
Type "from orders o" Enter
Type "join cust"
Sleep 2500ms
Screenshot .demo/shots/08-completion.png

Tab
Sleep 700ms
Type " c on c.id = o.customer_id" Enter
Type "group by c.country" Enter
Type "order by revenue desc"
Sleep 1s

Ctrl+r
Sleep 3s
Screenshot .demo/shots/09-result.png

# The plan, with the times that the server measured.
Ctrl+y
Sleep 4s
Screenshot .demo/shots/10-plan.png
Type "1" Sleep 1s

# The split view: the query on the left, and a table opened from the explorer on the right.
Alt+v
Sleep 1500ms
Alt+p
Type "s"
Sleep 500ms
Up Sleep 200ms
Up Sleep 200ms
Up Sleep 200ms
Enter
Sleep 3s
Screenshot .demo/shots/21-split-view.png
Alt+v
Sleep 1s

# A query builder tab: two joined tables, a group and an aggregate, and the result.
Alt+j
Sleep 1500ms
Type "orders"
Sleep 800ms
Enter
Sleep 1500ms
Type "t"
Sleep 1200ms
Enter
Sleep 1500ms
Enter
Sleep 1500ms
Down Sleep 200ms
Down Sleep 200ms
Down Sleep 200ms
Space
Sleep 800ms
Left
Sleep 500ms
Enter
Sleep 1s
Right Sleep 200ms
Right Sleep 200ms
Down Sleep 200ms
Down Sleep 200ms
Right Sleep 200ms
Right Sleep 200ms
Enter
Sleep 1500ms
Ctrl+r
Sleep 3s
Screenshot .demo/shots/22-query-builder.png

# The server activity dashboard.
Alt+o
Type "a"
Sleep 4s
Screenshot .demo/shots/23-server-activity.png
Escape Sleep 800ms

# The palette.
Ctrl+k
Sleep 2s
Screenshot .demo/shots/11-palette.png
Escape Sleep 800ms

# A notebook of prose, values, statements and a chart, with every cell run.
Alt+o
Type "n"
Sleep 2s
Screenshot .demo/shots/12-notebooks-card.png

Enter
Sleep 2s
Alt+r
Sleep 4s
Screenshot .demo/shots/13-notebook.png

Type "j" Sleep 400ms
Type "j" Sleep 400ms
Type "j" Sleep 2s
Screenshot .demo/shots/14-notebook-chart.png

# The settings screen.
Ctrl+k
Sleep 1s
Type "settings" Sleep 1500ms
Enter
Sleep 2s
Screenshot .demo/shots/24-settings.png
Escape Sleep 800ms

# And the same notebook in a light theme.
Ctrl+k
Sleep 1500ms
Type "theme" Sleep 1500ms
Enter Sleep 1500ms
Type "latte" Sleep 1500ms
Enter
Sleep 2500ms
Screenshot .demo/shots/15-light-theme.png
Sleep 500ms

# Choosing a theme writes it to the config file, so the theme of the config file comes back
# before vhs/dialogs.tape runs with that file.
Ctrl+k
Sleep 1s
Type "theme" Sleep 1s
Enter Sleep 1s
Type "ayu dark" Sleep 1s
Enter
Sleep 1500ms
