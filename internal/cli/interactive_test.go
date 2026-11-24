package cli

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/babarot/naminator/internal/config"
	"github.com/babarot/naminator/internal/photo"
	"github.com/babarot/naminator/internal/rename"
)

func TestStartFormat(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir, "card/a.jpg", "b.HEIC")
	card, file := filepath.Join(dir, "card"), filepath.Join(dir, "b.HEIC")
	cfg := config.Config{Format: "{ext}/{date}/{date}_{time}"}
	tests := []struct {
		name string
		opt  Option
		cfg  config.Config
		args []string
		want string
	}{
		{"the config's format", Option{}, cfg, []string{card}, cfg.Format},
		{"files only: in place", Option{}, cfg, []string{file}, rename.Presets[0].Format},
		{"no config: in place", Option{}, config.Config{}, []string{card}, rename.Presets[0].Format},
		{"--format first", Option{Format: "{date}"}, cfg, []string{file}, "{date}"},
		{"--preset", Option{Preset: "by date"}, cfg, []string{card}, rename.Presets[1].Format},
	}
	for _, tt := range tests {
		if got, err := startFormat(tt.opt, tt.cfg, tt.args); err != nil || got != tt.want {
			t.Errorf("%s: %q, %v, want %q", tt.name, got, err, tt.want)
		}
	}
	if err := checkInteractive(Option{GroupByDate: true}); err == nil {
		t.Error("-i with -t should be an error")
	}
}

// The run renames a photo to the path previewed: renaming one by one would
// free a path the preview saw taken, and drop its _1.
func TestRenameUsesThePlannedPath(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, "card/2026-10-06_11-00-00.jpg", "card/screenshot.jpg")
	card := filepath.Join(root, "card")
	named := filepath.Join(card, "2026-10-06_11-00-00.jpg")
	shot := filepath.Join(card, "screenshot.jpg")
	at := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	photos := []photo.Photo{
		{Name: "2026-10-06_11-00-00.jpg", Path: named, Dir: card, Extension: "jpg", CreatedAt: at.Add(10 * time.Hour)},
		{Name: "screenshot.jpg", Path: shot, Dir: card, Extension: "jpg", CreatedAt: at.Add(11 * time.Hour)},
	}
	planned := map[string]string{
		named: filepath.Join(card, "2026-10-06_10-00-00.jpg"),
		shot:  filepath.Join(card, "2026-10-06_11-00-00_1.jpg"),
	}
	cli := CLI{
		opt:     Option{},
		fs:      osFS{},
		paths:   rename.NewReserver(),
		moves:   rename.NewPendingMoves(),
		tracker: &fakeTracker{},
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		namer:   rename.NewNamer(rename.MustParseTemplate("{dir}/{date}_{time}"), []string{card}, ""),
		planned: planned,
	}
	cli.renameAll(photos, true)
	for _, p := range []string{"2026-10-06_10-00-00.jpg", "2026-10-06_11-00-00_1.jpg"} {
		if _, err := os.Stat(filepath.Join(card, p)); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	// a planned path taken by then is not overwritten
	makeTree(t, root, "card2/a.jpg", "card2/taken.jpg")
	card2 := filepath.Join(root, "card2")
	cli.paths = rename.NewReserver()
	cli.planned = map[string]string{filepath.Join(card2, "a.jpg"): filepath.Join(card2, "taken.jpg")}
	cli.namer = rename.NewNamer(rename.MustParseTemplate("{dir}/taken"), []string{card2}, "")
	p, _, err := cli.rename(photo.Photo{Name: "a.jpg", Path: filepath.Join(card2, "a.jpg"), Dir: card2, Extension: "jpg", CreatedAt: at})
	if err != nil || p.RenamedPath != filepath.Join(card2, "taken_1.jpg") {
		t.Errorf("renamed to %q, %v; want taken_1.jpg", p.RenamedPath, err)
	}
}

func TestInteractiveByDefault(t *testing.T) {
	tests := []struct {
		name     string
		opt      Option
		terminal bool
		want     bool
	}{
		{"a terminal", Option{}, true, true},
		{"a terminal with -c, -n, -d, --format", Option{Clean: true, Dryrun: true, DestDir: "/out", Format: "{date}"}, true, true},
		{"--yes", Option{Yes: true}, true, false},
		{"no terminal", Option{}, false, false},
		{"-tecE as before", Option{GroupByDate: true, GroupByExt: true, GroupExtFirst: true}, true, false},
		{"-i asks for it", Option{Interactive: true, Yes: true}, false, true},
	}
	for _, tt := range tests {
		if got := interactive(tt.opt, tt.terminal); got != tt.want {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
}

// -d alone starts with the photos put into the destination as they are,
// as -d has always done.
func TestStartFormatWithDest(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir, "card/a.jpg")
	got, err := startFormat(Option{DestDir: "/out"}, config.Config{}, []string{filepath.Join(dir, "card")})
	if err != nil || got != "{date}_{time}" {
		t.Errorf("startFormat = %q, %v", got, err)
	}
	if got, _ := startFormat(Option{DestDir: "/out"}, config.Config{Format: "{ext}/{date}"}, []string{filepath.Join(dir, "card")}); got != "{ext}/{date}" {
		t.Errorf("with a config: %q", got)
	}
}
