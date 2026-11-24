# AGENTS.md

naminator renames photos by their EXIF data. In a terminal, it opens a screen that shows every new path before anything moves.

Two rules decide what no other rule covers:

- A photo goes where the screen showed it going. The run takes the paths previewed (`ui.Decision.Targets`) instead of working them out again, and takes another only when that one is taken by then.
- No file is overwritten or lost. A path is claimed or reserved (`rename.Reserver`) before anything moves to it, sidecars' included, and what cannot be moved is left where it is and reported.

## Checking a change

- `mise run test` (gofmt, vet, the tests with the race detector) and `mise run lint` (golangci-lint, `go mod tidy -diff`) are what CI runs. `mise run demo` records the GIFs of the README again, in Docker.
- Golden views pin how the screens look. When a change is meant to alter them, rewrite them and read the diff: `go test ./internal/ui -run TestGolden -update`. The package goes before `-update`: after it, go test runs the root package and reports ok without writing anything.
- A change to how a program starts, ends or hands over to the next is also to be tried in a real terminal (tmux will do). The tests see neither what is left on the screen nor a terminal's late replies, which end up in the shell.

## Packages

- `arch_test.go` checks imports against the table in `docs/architecture.md`. A new package or outside module goes into the table in `arch_test.go` first; then `go test -run TestArchitectureDoc -update` rewrites the document.
- Only `internal/cli` moves files and runs workers. It knows no bubbletea: it reports through `ui.Tracker` and gets a `ui.Decision` back.
- `version` and `revision` stay in `main.go`. The release builds set them with `-X main.version`, and the linker says nothing when the variable is gone.
- `-t`, `-e` and `-E` are deprecated but name photos exactly as they always have (`rename.NewLegacyNamer`); they do not follow changes to presets.

## Screens (`internal/ui`)

- Something new over or in the interactive screen is a value of `uiState` in `uistate.go`. Then fill in what the tests in `uistate_test.go` ask for: a screen, a golden view, a key line.
- A key is an action in `keymap.Defaults`, in the scope it is read in. Never compare a key; draw the keys a hint names with `Map.Hint`. The keys table in `README.md` is by hand: change it too.
- A color is in a palette of `internal/ui/theme`, named by what it is for, with a value for a dark and a light background. `colors_test.go` fails on a color anywhere else.
- Tell what happened through the status line (`Say`, `Refuse`, `Fail`), which goes by itself.
- A model holds its own styles, keys and clock (`now`). No package variable holds state.
- A test of what is drawn must not depend on the machine: start with `goldenRoot`, which fixes `$HOME`, the working directory, the time zone and `NO_COLOR`.

## Pull requests

- The template's WHAT and WHY are the only headings.
- A `kind/` label decides the next version: `kind/breaking-change` raises the major, `kind/feature` the minor. Use the first only when a major release is meant.
