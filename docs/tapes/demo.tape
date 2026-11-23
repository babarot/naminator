# naminator demo: the screen that opens in a terminal
#
# Run with: mise run demo [demo]
# Requirements: Docker
#
# The photos are dummies and mock-exiftool stands in for exiftool (see
# setup.sh). naminator runs for real.

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
Type "source /data/setup.sh; export MOCK_DELAY=0.5"
Enter
Sleep 2s
Type "clear"
Enter
Sleep 500ms
Show

# --- Demo starts ---
# 1. Open the screen: each photo's new path shows as its EXIF data is read
Sleep 500ms
Type "naminator -c import"
Sleep 400ms
Enter
Sleep 5s

# 2. Move through the photos: the parts of the one selected show above
Down@400ms 3
Sleep 1.2s

# 3. Another preset: a directory per date
Tab
Sleep 2.5s

# 4. The directories as they will be, those left empty removed (-c)
Type "v"
Sleep 3.5s

# 5. Rename, after asking
Enter
Sleep 1.5s
Type "y"
Sleep 5s

# 6. The result: a directory per date next to import, which keeps only
#    the screenshot
Type "clear; tree"
Sleep 400ms
Enter
Sleep 4s
