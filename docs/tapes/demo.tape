# naminator demo: rename the photos of a memory card by when they were taken
#
# Run with: mise run demo
# Requirements: Docker
#
# The photos are dummies and mock-exiftool stands in for exiftool (see
# setup.sh). naminator runs for real: the spinners, the progress, the renames
# and the colors are all genuine.

Output demo.mp4

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
Type "source /data/setup.sh"
Enter
Sleep 2s
Type "clear"
Enter
Sleep 500ms
Show

# --- Demo starts ---

# 1. The card as copied: camera numbers, no dates
Sleep 700ms
Type "tree import | head -12"
Sleep 350ms
Enter
Sleep 2.5s

# 2. Rename by the shooting time into a directory per date, and remove the
#    directories left empty
Type "clear; naminator -t -c import"
Sleep 350ms
Enter
Sleep 12s

# 3. The result
Type "clear; tree -d import"
Sleep 350ms
Enter
Sleep 3s
