# The AI chat frame for the README. Rebuild it with `mise run shot-chat`.
# It runs opencode over ACP, so opencode must be installed and signed in.

Output .demo/chat.mp4

Require masume
Require opencode

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
Type "cd $(mktemp -d)" Enter
Type "clear" Enter
Type "masume" Enter
Sleep 2s
Enter
Sleep 4s
Show

Ctrl+k
Sleep 1500ms
Type "Ask AI"
Sleep 1s
Enter
Sleep 1500ms
# Each "y" answers a run_query confirmation. Backspace clears a "y" that found none.
Type "Run a query: which five countries bring the most revenue?"
Enter
Sleep 20s
Type "y"
Sleep 20s
Type "y"
Sleep 20s
Type "y"
Sleep 20s
Type "y"
Sleep 20s
Type "y"
Sleep 20s
Type "y"
Sleep 20s
Type "y"
Sleep 20s
Backspace 10
Sleep 1s
Screenshot vhs/shots/20-ai-chat.png
