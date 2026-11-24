package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/naminator/internal/photo"
)

var t0 = time.Date(2026, 10, 3, 14, 0, 0, 0, time.UTC)

func plain(s string) string { return ansi.Strip(s) }

func TestPhaseApply(t *testing.T) {
	t.Run("exif", func(t *testing.T) {
		p := newPhase(phaseStartMsg{kind: phaseExif, keys: []string{"/p/a.jpg", "/p/b.jpg"}, limit: 2}, t0)
		p.apply(exifStartMsg{image: "/p/a.jpg"}, t0)
		if p.items[0].status != itemRunning || p.items[1].status != itemQueued {
			t.Fatalf("statuses = %v, %v", p.items[0].status, p.items[1].status)
		}
		it, f := p.apply(exifResultMsg{image: "/p/a.jpg", photo: photo.Photo{Path: "/p/a.jpg"}, duration: 300 * time.Millisecond}, t0.Add(time.Second))
		if it == nil || it.mark != markNone || f != nil {
			t.Errorf("ok result: item=%+v failure=%v", it, f)
		}
		// exiftool reports no path on some errors: the image is the key
		_, f = p.apply(exifResultMsg{image: "/p/b.jpg", err: errors.New("File format error")}, t0.Add(2*time.Second))
		if f == nil || f.name != "b.jpg" || p.items[1].mark != markFailed {
			t.Errorf("failure = %v, mark = %v", f, p.items[1].mark)
		}
		if got := plain(p.outcome()); got != "✗ Read EXIF of 2 photos in 2.0s, 1 failed" {
			t.Errorf("outcome = %q", got)
		}
	})

	t.Run("rename", func(t *testing.T) {
		keys := []string{"/p/a.jpg", "/p/2026-10-03_14-00-00.jpg", "/p/c.jpg"}
		p := newPhase(phaseStartMsg{kind: phaseRename, keys: keys, base: "/p"}, t0)
		p.apply(renameResultMsg{photo: photo.Photo{Name: "a.jpg", Path: "/p/a.jpg", RenamedPath: "/p/2026-10-03/x.jpg"}}, t0)
		p.apply(renameResultMsg{photo: photo.Photo{Name: "2026-10-03_14-00-00.jpg", Path: keys[1], RenamedPath: keys[1]}}, t0)
		p.apply(renameResultMsg{photo: photo.Photo{Name: "c.jpg", Path: "/p/c.jpg"}, err: errors.New("permission denied")}, t0.Add(time.Second))
		if p.items[0].mark != markDone || p.items[0].label != "→ 2026-10-03/x.jpg" {
			t.Errorf("renamed: mark=%v label=%q", p.items[0].mark, p.items[0].label)
		}
		if p.items[1].mark != markNone || p.items[1].label != "already named" {
			t.Errorf("no-op: mark=%v label=%q", p.items[1].mark, p.items[1].label)
		}
		if got := plain(p.outcome()); got != "✗ Renamed 1 photo in 1.0s, 1 already named, 1 failed" {
			t.Errorf("outcome = %q", got)
		}
		if got := p.title(); got != "Renaming 3 photos in /p" {
			t.Errorf("title = %q", got)
		}
	})

	t.Run("dry-run rename", func(t *testing.T) {
		p := newPhase(phaseStartMsg{kind: phaseRename, keys: []string{"/p/a.jpg"}, dry: true}, t0)
		p.apply(renameResultMsg{photo: photo.Photo{Name: "a.jpg", Path: "/p/a.jpg", RenamedPath: "/q/x.jpg"}, dryrun: true}, t0)
		if p.items[0].mark != markChange {
			t.Errorf("mark = %v, want markChange", p.items[0].mark)
		}
		if got := plain(p.outcome()); got != "✓ Would rename 1 photo (dry run)" {
			t.Errorf("outcome = %q", got)
		}
	})

	t.Run("clean adds rows as the directories come", func(t *testing.T) {
		p := newPhase(phaseStartMsg{kind: phaseClean}, t0)
		p.apply(cleanResultMsg{dir: "DCIM/100MSDCF", empty: true}, t0)
		p.apply(cleanResultMsg{dir: "DCIM", empty: false}, t0)
		if len(p.items) != 2 || p.items[0].mark != markDelete || p.items[1].mark != markNone {
			t.Fatalf("items = %+v", p.items)
		}
		if got := plain(p.outcome()); got != "✓ Removed 1 empty directory in 0.0s, 1 kept" {
			t.Errorf("outcome = %q", got)
		}
	})

	t.Run("results after cancel are ignored", func(t *testing.T) {
		p := newPhase(phaseStartMsg{kind: phaseExif, keys: []string{"/p/a.jpg", "/p/b.jpg"}}, t0)
		p.apply(exifStartMsg{image: "/p/a.jpg"}, t0)
		p.cancel(t0)
		it, f := p.apply(exifResultMsg{image: "/p/a.jpg", err: errors.New("late")}, t0)
		if it != nil || f != nil || p.items[0].status != itemCanceled {
			t.Errorf("item=%v failure=%v status=%v", it, f, p.items[0].status)
		}
		if p.items[1].status != itemQueued {
			t.Errorf("not started should stay queued, got %v", p.items[1].status)
		}
		if got := plain(p.outcome()); got != "✗ Reading EXIF of 2 photos (canceled)" {
			t.Errorf("outcome = %q", got)
		}
	})
}

func TestPhaseTarget(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	p := &phase{phaseStartMsg: phaseStartMsg{base: "/home/u/Pictures/import"}}
	tests := []struct{ path, want string }{
		{"/home/u/Pictures/import/DCIM/2026-10-03/x.arw", "DCIM/2026-10-03/x.arw"},
		// outside the base, as when -t moves photos of the base itself up
		{"/home/u/Pictures/2026-10-03/x.arw", "~/Pictures/2026-10-03/x.arw"},
		{"/home/u/Pictures/importer/x.arw", "~/Pictures/importer/x.arw"},
	}
	for _, tt := range tests {
		if got := p.target(tt.path); got != tt.want {
			t.Errorf("target(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
	p.base = ""
	if got := p.target("/home/u/a/x.jpg"); got != "~/a/x.jpg" {
		t.Errorf("without base: %q", got)
	}
}

func TestPhaseRow(t *testing.T) {
	p := newPhase(phaseStartMsg{kind: phaseExif, keys: []string{"/p/broken.jpg", "/p/写真のファイル名がとても長い場合のテスト.jpg"}}, t0)
	p.apply(exifResultMsg{image: "/p/broken.jpg", err: errors.New("no date found\ntried: DateTimeOriginal"), duration: 100 * time.Millisecond}, t0)

	row := plain(p.row(p.items[0], "", t0, 0))
	if !strings.HasPrefix(row, "  ✗ broken.jpg") || !strings.HasSuffix(row, "0.1s  no date found…") {
		t.Errorf("error row = %q", row)
	}
	// names are padded to the same width, in cells
	if w := ansi.StringWidth(padName("写真のファイル名がとても長い場合のテスト.jpg", maxNameWidth)); w != maxNameWidth {
		t.Errorf("padded width = %d, want %d", w, maxNameWidth)
	}
	for _, width := range []int{20, 40, 80} {
		for _, it := range p.items {
			if w := ansi.StringWidth(p.row(it, "x", t0, width)); w > width {
				t.Errorf("row is %d cells, wider than %d", w, width)
			}
		}
	}
}

func TestPhaseCompact(t *testing.T) {
	keys := func(n int) []string {
		var k []string
		for i := range n {
			k = append(k, fmt.Sprintf("/p/%03d.jpg", i))
		}
		return k
	}
	if newPhase(phaseStartMsg{kind: phaseExif, keys: keys(10)}, t0).wantCompact(40) {
		t.Error("10 rows on a tall terminal should be drawn whole")
	}
	if !newPhase(phaseStartMsg{kind: phaseExif, keys: keys(11)}, t0).wantCompact(0) {
		t.Error("11 rows should be compact")
	}
	if !newPhase(phaseStartMsg{kind: phaseExif, keys: keys(5)}, t0).wantCompact(8) {
		t.Error("rows taller than the terminal should be compact")
	}

	// the compact view keeps within the terminal, with a line for the cursor
	p := newPhase(phaseStartMsg{kind: phaseExif, keys: keys(40), limit: 8}, t0)
	for _, k := range p.keys[:8] {
		p.apply(exifStartMsg{image: k}, t0)
	}
	for _, h := range []int{4, 5, 6, 8, 30} {
		lines := p.compactView("x", t0, 80, h)
		if len(lines)+1 > h {
			t.Errorf("height %d: %d lines", h, len(lines))
		}
	}
	got := plain(strings.Join(p.compactView("x", t0, 120, 30), "\n"))
	for _, want := range []string{"+3 more · 32 queued", "0/40 photos", "exiftool 8/8", "ctrl+c to cancel"} {
		if !strings.Contains(got, want) {
			t.Errorf("compact view lacks %q:\n%s", want, got)
		}
	}
}

func TestPhaseCompactKeepsHeight(t *testing.T) {
	var keys []string
	for i := range 20 {
		keys = append(keys, fmt.Sprintf("/p/%03d.jpg", i))
	}
	p := newPhase(phaseStartMsg{kind: phaseExif, keys: keys, limit: 8}, t0)
	for _, k := range keys[:8] {
		p.apply(exifStartMsg{image: k}, t0)
	}
	full := len(p.compactView("x", t0, 120, 30))
	// the queue empties and the running rows go away one by one
	for i, k := range keys {
		p.apply(exifResultMsg{image: k, photo: photo.Photo{Path: k}}, t0)
		if next := i + 8; next < len(keys) {
			p.apply(exifStartMsg{image: keys[next]}, t0)
		}
		lines := p.compactView("x", t0, 120, 30)
		if len(lines) != full {
			t.Fatalf("after %d photos the view is %d lines, want %d:\n%s", i+1, len(lines), full, plain(strings.Join(lines, "\n")))
		}
		if !strings.Contains(plain(lines[len(lines)-1]), "photos") {
			t.Fatalf("the last line should be the progress, got %q", plain(lines[len(lines)-1]))
		}
	}
	// a terminal too short for the height kept: within it still
	if lines := p.compactView("x", t0, 120, 5); len(lines)+1 > 5 {
		t.Errorf("on 5 lines the view is %d lines", len(lines))
	}
}

func TestErrorReport(t *testing.T) {
	got := plain(strings.Join(errorReport([]failure{{name: "a.jpg", err: "line 1\nline 2"}}), "\n"))
	if want := "  a.jpg:\n    line 1\n    line 2\n"; got != want {
		t.Errorf("report = %q, want %q", got, want)
	}
}
