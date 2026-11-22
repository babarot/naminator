package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFindSidecars(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root,
		"card/DSC05501.ARW", "card/DSC05501.JPG", "card/DSC05501.xmp", // shared: the raw file owns it
		"card/DSC05502.ARW", "card/DSC05502.ARW.xmp", // named after the file name
		"card/DSC05503.JPG", "card/DSC05503.HEIC", "card/DSC05503.xmp", // no raw file: left
		"card/DSC05504.JPG", "card/DSC05504.XMP", // upper case extension
		"card/notes.xmp",    // no photo of that name: not a sidecar
		"card/DSC05501.txt", // not a sidecar extension
		"iphone/IMG_2033.HEIC", "iphone/IMG_2033.AAE",
	)
	var photos []Photo
	for _, p := range []string{"card/DSC05501.ARW", "card/DSC05501.JPG", "card/DSC05502.ARW", "card/DSC05503.JPG", "card/DSC05503.HEIC", "card/DSC05504.JPG", "iphone/IMG_2033.HEIC"} {
		photos = append(photos, Photo{Path: filepath.Join(root, p)})
	}
	owned, warnings := findSidecars(osFS{}, photos)
	got := map[string]string{}
	for photo, scs := range owned {
		for _, sc := range scs {
			got[filepath.Base(sc)] = filepath.Base(photo)
		}
	}
	want := map[string]string{
		"DSC05501.xmp":     "DSC05501.ARW",
		"DSC05502.ARW.xmp": "DSC05502.ARW",
		"DSC05504.XMP":     "DSC05504.JPG",
		"IMG_2033.AAE":     "IMG_2033.HEIC",
	}
	if len(got) != len(want) {
		t.Errorf("owned = %v, want %v", got, want)
	}
	for sc, photo := range want {
		if got[sc] != photo {
			t.Errorf("%s belongs to %q, want %q", sc, got[sc], photo)
		}
	}
	if len(warnings) != 1 || filepath.Base(warnings[0].path) != "DSC05503.xmp" || !strings.Contains(warnings[0].reason, "DSC05503.HEIC") {
		t.Errorf("warnings = %+v, want DSC05503.xmp left", warnings)
	}
}

func TestSidecarTarget(t *testing.T) {
	tests := []struct{ sidecar, want string }{
		{"/c/DSC05501.xmp", "/o/2026-10-04_14-22-09.xmp"},
		{"/c/DSC05501.XMP", "/o/2026-10-04_14-22-09.XMP"},
		{"/c/DSC05501.ARW.xmp", "/o/2026-10-04_14-22-09.arw.xmp"},
	}
	for _, tt := range tests {
		if got := sidecarTarget("/c/DSC05501.ARW", "/o/2026-10-04_14-22-09.arw", tt.sidecar); got != tt.want {
			t.Errorf("sidecarTarget(%q) = %q, want %q", tt.sidecar, got, tt.want)
		}
	}
}

func sidecarCLI(root string, opt Option) CLI {
	return CLI{
		opt:    opt,
		fs:     osFS{},
		paths:  newPathReserver(),
		moves:  newPendingMoves(),
		sender: &mockSender{},
		namer:  newNamer(mustParseTemplate("{ext}/{date}/{date}_{time}"), []string{filepath.Join(root, "card")}, ""),
	}
}

func sidecarPhoto(root, name string) Photo {
	return Photo{
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
	if len(photo.RenamedSidecars) != 1 || sidecarNote(photo) != "  + xmp" {
		t.Errorf("RenamedSidecars = %v, note %q", photo.RenamedSidecars, sidecarNote(photo))
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
	empty, _, err := emptyDirs(filepath.Join(root, "card"), cli.moves)
	if err != nil || !empty {
		t.Errorf("emptyDirs = %v, %v, want the card predicted empty", empty, err)
	}
}

func TestProcessAllReportsSidecarsLeft(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, "card/DSC05503.JPG", "card/DSC05503.HEIC", "card/DSC05503.xmp")
	at := time.Date(2026, 10, 4, 14, 22, 9, 0, time.UTC)
	photos := map[string]Photo{}
	var images []string
	for _, name := range []string{"DSC05503.JPG", "DSC05503.HEIC"} {
		path := filepath.Join(root, "card", name)
		images = append(images, path)
		photos[path] = Photo{Name: name, Path: path, Dir: filepath.Join(root, "card"), Extension: strings.ToLower(strings.TrimPrefix(filepath.Ext(name), ".")), CreatedAt: at}
	}
	sender := &mockSender{}
	cli := sidecarCLI(root, Option{Dryrun: true})
	cli.sender = sender
	cli.images = images
	cli.exifs = []ExifExtractor{&mockExif{photos: photos}}
	cli.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	cli.processAll()

	var warned []warnMsg
	for _, m := range sender.getMessages() {
		if w, ok := m.(warnMsg); ok {
			warned = append(warned, w)
		}
	}
	if len(warned) != 1 || warned[0].name != "DSC05503.xmp" {
		t.Errorf("warnings = %+v, want DSC05503.xmp", warned)
	}
}

// writeTIFF writes the start of a TIFF file, which is enough to be read as
// an image (raw files are TIFF based).
func writeTIFF(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("II*\x00\x08\x00\x00\x00\x00\x00"), 0644); err != nil {
		t.Fatal(err)
	}
}

// A raw file that is not renamed (its EXIF data cannot be read, or it was
// not given) still owns the sidecar it shares: the JPEG does not take it.
func TestFindSidecarsKeepsTheRawFilesSidecar(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, "card/DSC05501.JPG", "card/DSC05501.xmp")
	writeTIFF(t, filepath.Join(root, "card", "DSC05501.ARW"))
	owned, warnings := findSidecars(osFS{}, []Photo{{Path: filepath.Join(root, "card", "DSC05501.JPG")}})
	if len(owned) != 0 || len(warnings) != 0 {
		t.Errorf("owned = %v, warnings = %v: the sidecar should stay with the raw file", owned, warnings)
	}
}

// The movie of a Live Photo is not an image, so it does not share the
// photo's sidecar.
func TestFindSidecarsIgnoresLivePhotoMovies(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, "iphone/IMG_2033.HEIC", "iphone/IMG_2033.MOV", "iphone/IMG_2033.AAE")
	heic := filepath.Join(root, "iphone", "IMG_2033.HEIC")
	owned, warnings := findSidecars(osFS{}, []Photo{{Path: heic}})
	if len(owned[heic]) != 1 || len(warnings) != 0 {
		t.Errorf("owned = %v, warnings = %v, want the AAE with the HEIC", owned, warnings)
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
