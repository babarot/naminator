package cli

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/babarot/naminator/internal/photo"
	"github.com/babarot/naminator/internal/rename"
)

func sidecarCLI(root string, opt Option) CLI {
	return CLI{
		opt:     opt,
		fs:      osFS{},
		paths:   rename.NewReserver(),
		moves:   rename.NewPendingMoves(),
		tracker: &fakeTracker{},
		namer:   rename.NewNamer(rename.MustParseTemplate("{ext}/{date}/{date}_{time}"), []string{filepath.Join(root, "card")}, ""),
	}
}

func sidecarPhoto(root, name string) photo.Photo {
	return photo.Photo{
		Name:      name,
		Path:      filepath.Join(root, "card", name),
		Dir:       filepath.Join(root, "card"),
		Extension: strings.ToLower(strings.TrimPrefix(filepath.Ext(name), ".")),
		CreatedAt: time.Date(2026, 10, 4, 14, 22, 9, 0, time.UTC),
		Sidecars:  []string{filepath.Join(root, "card", "DSC05501.xmp")},
	}
}

func TestRenameMovesSidecars(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, "card/DSC05501.ARW", "card/DSC05501.xmp")
	cli := sidecarCLI(root, Option{})
	photo, _, err := cli.rename(sidecarPhoto(root, "DSC05501.ARW"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"arw/2026-10-04/2026-10-04_14-22-09.arw", "arw/2026-10-04/2026-10-04_14-22-09.xmp"} {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	if len(photo.RenamedSidecars) != 1 {
		t.Errorf("RenamedSidecars = %v", photo.RenamedSidecars)
	}
	// the card is left empty, so --clean can remove it
	if entries, _ := os.ReadDir(filepath.Join(root, "card")); len(entries) != 0 {
		t.Errorf("card still has %d files", len(entries))
	}
}

// A sidecar whose path is taken stops its photo, so that they keep
// matching, and the file there is kept.
func TestRenameStopsWhenSidecarPathIsTaken(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, "card/DSC05501.ARW", "card/DSC05501.xmp", "arw/2026-10-04/2026-10-04_14-22-09.xmp")
	cli := sidecarCLI(root, Option{})
	_, _, err := cli.rename(sidecarPhoto(root, "DSC05501.ARW"))
	if err == nil || !strings.Contains(err.Error(), "taken") {
		t.Fatalf("err = %v, want the sidecar's path taken", err)
	}
	if _, err := os.Stat(filepath.Join(root, "card", "DSC05501.ARW")); err != nil {
		t.Errorf("the photo should stay: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "arw", "2026-10-04", "2026-10-04_14-22-09.arw")); err == nil {
		t.Error("the photo should not be moved")
	}
}

func TestRenameDryRunPredictsSidecars(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, "card/DSC05501.ARW", "card/DSC05501.xmp")
	cli := sidecarCLI(root, Option{Dryrun: true, Clean: true})
	if _, _, err := cli.rename(sidecarPhoto(root, "DSC05501.ARW")); err != nil {
		t.Fatal(err)
	}
	// nothing moved, and clean predicts the card left empty
	if _, err := os.Stat(filepath.Join(root, "card", "DSC05501.xmp")); err != nil {
		t.Errorf("dry run should not move the sidecar: %v", err)
	}
	empty, _, err := rename.EmptyDirs(filepath.Join(root, "card"), cli.moves)
	if err != nil || !empty {
		t.Errorf("emptyDirs = %v, %v, want the card predicted empty", empty, err)
	}
}

func TestProcessAllReportsSidecarsLeft(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, "card/DSC05503.JPG", "card/DSC05503.HEIC", "card/DSC05503.xmp")
	at := time.Date(2026, 10, 4, 14, 22, 9, 0, time.UTC)
	photos := map[string]photo.Photo{}
	var images []string
	for _, name := range []string{"DSC05503.JPG", "DSC05503.HEIC"} {
		path := filepath.Join(root, "card", name)
		images = append(images, path)
		photos[path] = photo.Photo{Name: name, Path: path, Dir: filepath.Join(root, "card"), Extension: strings.ToLower(strings.TrimPrefix(filepath.Ext(name), ".")), CreatedAt: at}
	}
	sender := &fakeTracker{}
	cli := sidecarCLI(root, Option{Dryrun: true})
	cli.tracker = sender
	cli.images = images
	cli.exifs = []photo.Extractor{&mockExif{photos: photos}}
	cli.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	cli.processAll()

	var warned []warnedEvent
	for _, m := range sender.getEvents() {
		if w, ok := m.(warnedEvent); ok {
			warned = append(warned, w)
		}
	}
	if len(warned) != 1 || warned[0].name != "DSC05503.xmp" {
		t.Errorf("warnings = %+v, want DSC05503.xmp", warned)
	}
}

// Two sidecars that differ only in case would be one file where case does
// not matter: the photo is not moved rather than lose one of them.
func TestRenameStopsOnSidecarsDifferingInCase(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, "card/DSC05501.ARW")
	cli := sidecarCLI(root, Option{})
	photo := sidecarPhoto(root, "DSC05501.ARW")
	photo.Sidecars = []string{filepath.Join(root, "card", "DSC05501.xmp"), filepath.Join(root, "card", "DSC05501.XMP")}
	_, _, err := cli.rename(photo)
	if err == nil || !strings.Contains(err.Error(), "same path") {
		t.Fatalf("err = %v, want the sidecars going to the same path", err)
	}
	if _, err := os.Stat(filepath.Join(root, "card", "DSC05501.ARW")); err != nil {
		t.Errorf("the photo should stay: %v", err)
	}
}
