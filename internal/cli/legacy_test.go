package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/babarot/naminator/internal/photo"
	"github.com/babarot/naminator/internal/rename"
)

// oldBuildNewPath is buildNewPath as it was before templates, to check that
// the flags still name photos the same.
func oldBuildNewPath(photo photo.Photo, opt Option) string {
	dest := opt.DestDir
	if dest == "" {
		dest = photo.Dir
		if opt.GroupByDate || opt.GroupByExt {
			dest = filepath.Dir(dest)
		}
	}
	if opt.GroupByExt && opt.GroupByDate {
		if opt.GroupExtFirst {
			dest = filepath.Join(dest, photo.Extension)
			dest = filepath.Join(dest, photo.CreatedAt.Format("2006-01-02"))
		} else {
			dest = filepath.Join(dest, photo.CreatedAt.Format("2006-01-02"))
			dest = filepath.Join(dest, photo.Extension)
		}
	} else if opt.GroupByDate {
		dest = filepath.Join(dest, photo.CreatedAt.Format("2006-01-02"))
	} else if opt.GroupByExt {
		dest = filepath.Join(dest, photo.Extension)
	}
	return filepath.Join(dest, fmt.Sprintf("%s.%s", photo.CreatedAt.Format("2006-01-02_15-04-05"), photo.Extension))
}

func TestLegacyNamerMatchesOldPaths(t *testing.T) {
	photos := []photo.Photo{
		{Name: "a.jpg", Path: "/photos/card/DCIM/100MSDCF/a.jpg", Dir: "/photos/card/DCIM/100MSDCF", Extension: "jpg", CreatedAt: time.Date(2024, 3, 15, 14, 30, 45, 0, time.UTC)},
		{Name: "b.ARW", Path: "card/b.ARW", Dir: "card", Extension: "arw", CreatedAt: time.Date(2025, 1, 29, 21, 7, 43, 0, time.UTC)},
		{Name: "c.heic", Path: "c.heic", Dir: ".", Extension: "heic", CreatedAt: time.Date(2026, 10, 4, 9, 10, 0, 0, time.UTC)},
	}
	for _, dest := range []string{"", "/archive", "out"} {
		for _, flags := range []struct{ t, e, E bool }{{}, {t: true}, {e: true}, {t: true, e: true}, {t: true, e: true, E: true}} {
			opt := Option{DestDir: dest, GroupByDate: flags.t, GroupByExt: flags.e, GroupExtFirst: flags.E}
			for _, p := range photos {
				if got, want := rename.NewLegacyNamer(opt.legacy()).Target(p), oldBuildNewPath(p, opt); got != want {
					t.Errorf("-d %q %+v %s: %q, want %q", dest, flags, p.Path, got, want)
				}
			}
		}
	}
}

// A {seq} name taken by a file from an earlier import is not overwritten:
// the reserver adds _1 as for any other name taken.
func TestNamerSeqKeepsExistingFiles(t *testing.T) {
	root := t.TempDir()
	card := filepath.Join(root, "card")
	makeTree(t, root, "card/a.jpg", "A/2026-10-04_001.jpg")
	cli := CLI{
		opt:   Option{},
		fs:    osFS{},
		paths: rename.NewReserver(),
		namer: rename.NewNamer(rename.MustParseTemplate("{camera}/{date}_{seq}"), []string{card}, ""),
	}
	photo := photo.Photo{Name: "a.jpg", Path: filepath.Join(card, "a.jpg"), Dir: card, Extension: "jpg", Camera: "A", CreatedAt: time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)}
	renamed, _, err := cli.rename(photo)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "A", "2026-10-04_001_1.jpg"); renamed.RenamedPath != want {
		t.Errorf("renamed to %q, want %q", renamed.RenamedPath, want)
	}
	if b, err := os.ReadFile(filepath.Join(root, "A", "2026-10-04_001.jpg")); err != nil || string(b) != "x" {
		t.Errorf("the existing file should be kept: %q, %v", b, err)
	}
}
