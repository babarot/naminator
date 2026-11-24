# Architecture

The packages are layered: a package imports only from lower layers, and only the outside modules listed for it. `arch_test.go` checks every import against this table. To change it, edit the table there and run `go test -run TestArchitectureDoc -update`.

<!-- layers:start -->
| Layer | Package | Does | May import | Outside modules |
|---|---|---|---|---|
| 0 | `internal/photo` | Photos: their EXIF data, read with exiftool, and the files that are photos | - | `github.com/barasher/go-exiftool`, `github.com/gabriel-vasile/mimetype` |
| 1 | `internal/rename` | Where photos go: templates, paths no two photos share, sidecars, and the directories left empty | `internal/photo` | - |
| 2 | `internal/config` | The configuration file | `internal/rename` | `github.com/goccy/go-yaml` |
| 3 | `internal/ui/keymap` | Keys bound to actions by name | - | - |
| 3 | `internal/ui/statusline` | What the status line shows, for a moment | - | - |
| 3 | `internal/ui/theme` | The colors, by what they are for, and their styles | - | `charm.land/lipgloss/v2` |
| 4 | `internal/ui` | The interactive screen and the display of a run | `internal/config`, `internal/photo`, `internal/rename`, `internal/ui/keymap`, `internal/ui/statusline`, `internal/ui/theme` | `charm.land/bubbles/v2`, `charm.land/bubbletea/v2`, `github.com/charmbracelet/x/ansi`, `github.com/mattn/go-isatty`, `golang.org/x/sys`, `golang.org/x/term` |
| 5 | `internal/cli` | The command line: the flags, the workers, and moving the files | `internal/config`, `internal/photo`, `internal/rename`, `internal/ui` | `github.com/jessevdk/go-flags`, `github.com/mattn/go-isatty`, `github.com/nxadm/tail` |
| 6 | `main` | Calls cli.Main with the version | `internal/cli` | - |
<!-- layers:end -->
