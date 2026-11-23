# naminator demo: the tree of the directories as they will be
#
# Run with: mise run demo demo-tree
# Requirements: Docker

Output demo-tree.mp4

# Dark theme (Tokyo Night inspired), as gh-infra's demos
Set Theme { "name": "naminator-dark", "black": "#16161e", "red": "#f7768e", "green": "#9ece6a", "yellow": "#e0af68", "blue": "#7aa2f7", "magenta": "#bb9af7", "cyan": "#7dcfff", "white": "#a9b1d6", "brightBlack": "#565f89", "brightRed": "#f7768e", "brightGreen": "#9ece6a", "brightYellow": "#e0af68", "brightBlue": "#7aa2f7", "brightMagenta": "#bb9af7", "brightCyan": "#7dcfff", "brightWhite": "#c0caf5", "background": "#1a1b26", "foreground": "#a9b1d6", "selection": "#283457", "cursor": "#c0caf5" }

Set Shell "bash"
Set FontSize 20
Set Width 1400
Set Height 1100
Set Padding 20
Set WindowBar ""
Set TypingSpeed 50ms

# --- Hidden setup ---
Hide
Type "source /data/setup.sh; export MOCK_DELAY=0.3"
Enter
Sleep 2s
Type "clear"
Enter
Sleep 500ms
Show

# --- Demo starts ---
Sleep 500ms
Type "naminator -c import"
Sleep 400ms
Enter
Sleep 3.5s

# 1. The tree: a long run of files is folded to its ends
Type "v"
Sleep 2s

# 2. Show the files folded, then fold them again
Down@200ms 14
Sleep 800ms
Type "l"
Sleep 2s
Type "h"
Sleep 1.5s

# 3. A screenshot has no shooting time: use the file's time instead
Type "G"
Sleep 1.2s
Type "m"
Sleep 2s

# 4. A directory per date: the directories left empty are removed (-c)
Tab
Sleep 3.5s

# 5. Fold a directory
Type "g"
Sleep 400ms
Down@300ms 2
Sleep 500ms
Type "h"
Sleep 2s
Type "q"
Sleep 1s
