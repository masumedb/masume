# The cards the docs point at: the password dialog, the write plan, the import form and
# the save-on-exit prompt. Rebuild them with `mise run shots`, which runs this tape after
# vhs/shots.tape. They are written to vhs/shots/.
#
# Each card gets its own masume run, because the state each one needs differs: a profile
# that prompts, a profile that measures a write, a file to import, a temporary connection.

Output .demo/dialogs.mp4

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
# The picker and the form print the working directory, so the capture runs from a
# neutral path rather than from a contributor's home directory.
Type "rm -rf /tmp/masume-import && mkdir -p /tmp/masume-import && cp vhs/orders.csv /tmp/masume-import/ && cd /tmp/masume-import" Enter
Type "clear" Enter

# --- the password dialog -------------------------------------------------------
# shop-prod uses auth = "prompt", so opening it asks.
Type "masume --profile shop-prod" Enter
Sleep 3s
Show
Sleep 1s
Screenshot .demo/shots/16-password.png

# --- the write plan ------------------------------------------------------------
# Connect, then run a delete that the plan can measure. order_items references
# orders without a cascade, so the plan reports the write as blocked.
Hide
Type "secret"
Sleep 500ms
Enter
Sleep 5s
Alt+n
Sleep 1500ms
Type "delete from orders where status = 'refunded'"
Sleep 800ms
Ctrl+r
Sleep 5s
Show
Sleep 1s
Screenshot .demo/shots/17-write-plan.png

# --- the import form -----------------------------------------------------------
# Cancel the write, focus the tree, and open the import of the orders table.
Hide
Escape
Sleep 1s
Alt+p
Type "s"
Sleep 1s
Right
Sleep 1500ms
Down Sleep 200ms
Down Sleep 200ms
Down Sleep 200ms
Down Sleep 1s
Type "m"
Sleep 1500ms
Type "import"
Sleep 1s
Enter
Sleep 2s
Enter
Sleep 3s
Show
Sleep 1s
Screenshot .demo/shots/18-import-form.png

# --- the save-on-exit prompt ---------------------------------------------------
# A target on the command line is a temporary connection, so quitting offers to save it.
Hide
Escape Sleep 500ms
Escape Sleep 500ms
Ctrl+c
Sleep 1500ms
Type "n"
Sleep 2s
Type "clear" Enter
Type "masume postgres://postgres:secret@127.0.0.1:55432/shop" Enter
Sleep 6s
Ctrl+c
Sleep 2s
Show
Sleep 1s
Screenshot .demo/shots/19-save-connection.png

Hide
Type "n" Sleep 1s
Type "rm -rf /tmp/masume-import" Enter
Sleep 500ms
