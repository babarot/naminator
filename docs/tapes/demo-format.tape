# naminator demo: write a format, and keep it as the default
#
# Run with: mise run demo demo-format
# Requirements: Docker

Output demo-format.mp4

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
Type "naminator import"
Sleep 400ms
Enter
Sleep 3.5s

# 1. Start from a preset
Tab
Sleep 1.5s

# 2. Edit the format: the preview follows each key, and a format not yet
#    closed says what is wrong
Type "a"
Sleep 800ms
Backspace@120ms 6
Sleep 500ms
Type@180ms "{orig}"
Sleep 1.5s
Ctrl+A
Sleep 400ms
Type@180ms "{camera}/"
Sleep 2s
Enter
Sleep 1s

# 3. The directories it makes
Type "v"
Sleep 3s

# 4. Keep it as the default
Ctrl+S
Sleep 2.5s
Type "q"
Sleep 1s
Type "cat ~/.config/naminator/config.yaml"
Sleep 400ms
Enter
Sleep 3s
