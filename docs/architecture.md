# Architecture

The packages are layered: a package imports only from lower layers, and only the outside modules listed for it. `arch_test.go` checks every import against this table. To change it, edit the table there and run `go test -run TestArchitectureDoc -update`.

<!-- layers:start -->
| Layer | Package | Does | May import | Outside modules |
|---|---|---|---|---|
| 0 | `internal/photo` | Photos: their EXIF data, read with exiftool, and the files that are photos | - | `github.com/barasher/go-exiftool`, `github.com/gabriel-vasile/mimetype` |
| 1 | `internal/rename` | Where photos go: templates, paths no two photos share, sidecars, and the directories left empty | `internal/photo` | - |
| 2 | `internal/config` | The configuration file | `internal/rename` | `github.com/goccy/go-yaml` |
| 3 | `main` | The flags, the interactive screen, the run and its display | `internal/config`, `internal/photo`, `internal/rename` | `charm.land/bubbles/v2`, `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `github.com/charmbracelet/x/ansi`, `github.com/jessevdk/go-flags`, `github.com/mattn/go-isatty`, `github.com/nxadm/tail`, `golang.org/x/sys`, `golang.org/x/term` |
<!-- layers:end -->
