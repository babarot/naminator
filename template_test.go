package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseTemplate(t *testing.T) {
	tests := []struct {
		format string
		err    string // a part of the error; "" for none
	}{
		{"{date}_{time}", ""},
		{"{ext}/{date}/{date}_{time}", ""},
		{"{year}{month}{day}_{hour}{min}{sec}", ""},
		{"{camera}/{date}_{seq}", ""},
		{"{dir}/{orig}", ""},
		{"photos-{date}", ""},
		{"", "empty"},
		{"  ", "empty"},
		{"{date}/{foo}", "unknown placeholder {foo}"},
		{"{date", "not closed"},
		{"/{date}", "starts with /"},
		{"{date}/", "ends with /"},
		{"../{date}", "above the base"},
		{"{date}/../{time}", "above the base"},
	}
	for _, tt := range tests {
		_, err := parseTemplate(tt.format)
		switch {
		case tt.err == "" && err != nil:
			t.Errorf("parseTemplate(%q) = %v, want no error", tt.format, err)
		case tt.err != "" && (err == nil || !strings.Contains(err.Error(), tt.err)):
			t.Errorf("parseTemplate(%q) = %v, want an error with %q", tt.format, err, tt.err)
		}
	}
}

func TestTemplateRender(t *testing.T) {
	photo := Photo{
		Name:      "DSC05501.ARW",
		Extension: "arw",
		Camera:    "ILCE-7CM2",
		CreatedAt: time.Date(2026, 10, 4, 14, 22, 9, 0, time.UTC),
	}
	tests := []struct{ format, want string }{
		{"{date}_{time}", "2026-10-04_14-22-09.arw"},
		{"{year}{month}{day}_{hour}{min}{sec}", "20261004_142209.arw"},
		{"{ext}/{date}/{orig}", "arw/2026-10-04/DSC05501.arw"},
		{"{camera}/{date}_{seq}", "ILCE-7CM2/2026-10-04_" + seqMark + ".arw"},
		{"{dir}/{date}", "10551007/2026-10-04.arw"},
	}
	for _, tt := range tests {
		if got := mustParseTemplate(tt.format).render(photo, "10551007"); got != tt.want {
			t.Errorf("render(%q) = %q, want %q", tt.format, got, tt.want)
		}
	}

	// a camera model is one path segment; an unknown one is named so
	for camera, want := range map[string]string{"iPhone 15 Pro": "iPhone_15_Pro", "A/B": "A_B", "": "unknown"} {
		photo.Camera = camera
		if got := value(photo, "camera", ""); got != want {
			t.Errorf("camera %q = %q, want %q", camera, got, want)
		}
	}
}

func TestPresetsParse(t *testing.T) {
	for _, p := range presets {
		if _, err := parseTemplate(p.format); err != nil {
			t.Errorf("preset %q: %v", p.name, err)
		}
	}
}

// oldBuildNewPath is buildNewPath as it was before templates, to check that
// the flags still name photos the same.
func oldBuildNewPath(photo Photo, opt Option) string {
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
	photos := []Photo{
		{Name: "a.jpg", Path: "/photos/card/DCIM/100MSDCF/a.jpg", Dir: "/photos/card/DCIM/100MSDCF", Extension: "jpg", CreatedAt: time.Date(2024, 3, 15, 14, 30, 45, 0, time.UTC)},
		{Name: "b.ARW", Path: "card/b.ARW", Dir: "card", Extension: "arw", CreatedAt: time.Date(2025, 1, 29, 21, 7, 43, 0, time.UTC)},
		{Name: "c.heic", Path: "c.heic", Dir: ".", Extension: "heic", CreatedAt: time.Date(2026, 10, 4, 9, 10, 0, 0, time.UTC)},
	}
	for _, dest := range []string{"", "/archive", "out"} {
		for _, flags := range []struct{ t, e, E bool }{{}, {t: true}, {e: true}, {t: true, e: true}, {t: true, e: true, E: true}} {
			opt := Option{DestDir: dest, GroupByDate: flags.t, GroupByExt: flags.e, GroupExtFirst: flags.E}
			for _, p := range photos {
				if got, want := legacyNamer(opt.legacy()).target(p), oldBuildNewPath(p, opt); got != want {
					t.Errorf("-d %q %+v %s: %q, want %q", dest, flags, p.Path, got, want)
				}
			}
		}
	}
}

func TestNewNamer(t *testing.T) {
	at := time.Date(2026, 10, 4, 14, 22, 9, 0, time.UTC)
	photo := func(path string) Photo {
		return Photo{Name: filepath.Base(path), Path: path, Dir: filepath.Dir(path), Extension: strings.ToLower(strings.TrimPrefix(filepath.Ext(path), ".")), CreatedAt: at}
	}
	tests := []struct {
		name   string
		args   []string
		dest   string
		format string
		path   string
		want   string
	}{
		// card folders copied to the desktop: the base is their parent
		{"by extension, then date", []string{"Desktop/10551007", "Desktop/10651008"}, "", "{ext}/{date}/{date}_{time}", "Desktop/10551007/DSC05501.ARW", "Desktop/arw/2026-10-04/2026-10-04_14-22-09.arw"},
		{"in place", []string{"Desktop/10551007"}, "", "{dir}/{date}_{time}", "Desktop/10551007/DSC05501.ARW", "Desktop/10551007/2026-10-04_14-22-09.arw"},
		{"folders inside", []string{"Pictures/import"}, "", "{dir}/{date}_{time}", "Pictures/import/DCIM/100MSDCF/a.jpg", "Pictures/import/DCIM/100MSDCF/2026-10-04_14-22-09.jpg"},
		{"a trailing slash", []string{"Desktop/10551007/"}, "", "{date}/{date}_{time}", "Desktop/10551007/DSC05501.ARW", "Desktop/2026-10-04/2026-10-04_14-22-09.arw"},
		// files given: the base is their directory
		{"files given", []string{"iPhone/IMG_8101.HEIC"}, "", "{dir}/{date}_{time}", "iPhone/IMG_8101.HEIC", "iPhone/2026-10-04_14-22-09.heic"},
		// -d moves the output base only: {dir} is still from the input base
		{"dest", []string{"in"}, "/out", "{date}_{time}", "in/a.jpg", "/out/2026-10-04_14-22-09.jpg"},
		{"dest with dir", []string{"in"}, "/out", "{dir}/{date}_{time}", "in/a.jpg", "/out/in/2026-10-04_14-22-09.jpg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := newNamer(mustParseTemplate(tt.format), tt.args, tt.dest)
			if got := n.target(photo(tt.path)); got != tt.want {
				t.Errorf("target = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNamerSeq(t *testing.T) {
	n := newNamer(mustParseTemplate("{camera}/{date}_{seq}"), []string{"card"}, "")
	day := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	var got []string
	for i, camera := range []string{"A", "A", "B", "A"} {
		got = append(got, n.target(Photo{Name: fmt.Sprintf("%d.jpg", i), Path: fmt.Sprintf("card/%d.jpg", i), Dir: "card", Extension: "jpg", Camera: camera, CreatedAt: day.Add(time.Duration(i) * time.Hour)}))
	}
	// numbered per path: each camera and day from 001, in the order given
	want := []string{"A/2026-10-04_001.jpg", "A/2026-10-04_002.jpg", "B/2026-10-04_001.jpg", "A/2026-10-04_003.jpg"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("targets = %v, want %v", got, want)
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
		paths: newPathReserver(),
		namer: newNamer(mustParseTemplate("{camera}/{date}_{seq}"), []string{card}, ""),
	}
	photo := Photo{Name: "a.jpg", Path: filepath.Join(card, "a.jpg"), Dir: card, Extension: "jpg", Camera: "A", CreatedAt: time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)}
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
