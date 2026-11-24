package ui

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/babarot/naminator/internal/photo"
)

// The golden views pin how the screens look, characters and colors, so
// that a change that alters them shows as a diff of testdata/golden. Run
// go test ./internal/ui -run TestGolden -update to accept one.

var updateGolden = flag.Bool("update", false, "rewrite testdata/golden")

// goldenNow is the time the golden views are drawn at.
var goldenNow = time.Date(2026, 10, 4, 15, 0, 0, 0, time.UTC)

// goldenRoot makes the views independent of the machine: the working
// directory and $HOME become one new directory, so paths show as
// ~/card, the local time zone is UTC, and the styles are on whatever
// NO_COLOR says.
func goldenRoot(t *testing.T) string {
	t.Helper()
	t.Chdir(t.TempDir())
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root)
	// the file's time shows in the local time zone
	local := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = local })
	enableStyles()
	t.Cleanup(func() {
		if _, ok := os.LookupEnv("NO_COLOR"); ok {
			disableStyles()
		}
	})
	return root
}

// goldenTUI returns the screen over n photos in card/, all read, with the
// files on disk (for the file's time), on a dark or light background.
func goldenTUI(t *testing.T, n int, dark bool) tuiModel {
	t.Helper()
	m, photos := newTestTUI(t, n)
	if err := os.MkdirAll("card", 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range photos {
		if err := os.WriteFile(p.Path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p.Path, goldenNow, goldenNow); err != nil {
			t.Fatal(err)
		}
	}
	m.dark = dark
	return m.send(t, readAll(photos)...)
}

// moreLine returns the index of the first "⋯ N more files" line of the tree.
func moreLine(t *testing.T, m tuiModel) int {
	t.Helper()
	for i, it := range m.items() {
		if it.kind == itemMore {
			return i
		}
	}
	t.Fatal("no ⋯ more line in the tree")
	return 0
}

// goldenCase is a screen pinned in testdata/golden/tui: n photos, and what
// is done from there.
type goldenCase struct {
	name string
	n    int
	do   func(t *testing.T, m tuiModel) tuiModel
}

// at returns the screen of the case.
func (tc goldenCase) at(t *testing.T, dark bool) tuiModel {
	t.Helper()
	goldenRoot(t)
	m := goldenTUI(t, tc.n, dark)
	if tc.do != nil {
		m = tc.do(t, m)
	}
	return m
}

// goldenCases has at least one case of each state (TestEveryStateIsPinned).
func goldenCases() []goldenCase {
	return []goldenCase{
		{"list", 6, nil},
		{"reading", 6, func(t *testing.T, m tuiModel) tuiModel {
			fresh, photos := newTestTUI(t, 6)
			fresh.dark = m.dark
			return fresh.send(t, readAll(photos[:2])...)
		}},
		{"list-cursor", 6, func(t *testing.T, m tuiModel) tuiModel { return m.send(t, keys("j", "j")...) }},
		{"tree", 10, func(t *testing.T, m tuiModel) tuiModel { return m.send(t, keys("v")...) }},
		{"tree-folded", 10, func(t *testing.T, m tuiModel) tuiModel { return m.send(t, keys("v", "g", "h")...) }},
		{"tree-more-open", 10, func(t *testing.T, m tuiModel) tuiModel {
			m = m.send(t, keys("v")...)
			m.sel = moreLine(t, m)
			return m.send(t, keys("l")...)
		}},
		{"edit", 6, func(t *testing.T, m tuiModel) tuiModel { return m.send(t, keys("a", "backspace")...) }},
		{"format-error", 6, func(t *testing.T, m tuiModel) tuiModel {
			m.setFormat("{date}/{nope}")
			return m
		}},
		{"preset", 6, func(t *testing.T, m tuiModel) tuiModel { return m.send(t, keys("tab")...) }},
		{"confirm", 6, func(t *testing.T, m tuiModel) tuiModel { return m.send(t, keys("enter")...) }},
		{"more-lines", 40, func(t *testing.T, m tuiModel) tuiModel { return m.send(t, keys("ctrl+d")...) }},
		{"no-shooting-time", 6, func(t *testing.T, m tuiModel) tuiModel {
			fresh, photos := newTestTUI(t, 6)
			fresh.dark = m.dark
			msgs := append(readAll(photos[:5]), exifResultMsg{image: photos[5].Path, err: photo.ErrNoShootingTime})
			return fresh.send(t, append(msgs, keys("G")...)...)
		}},
		{"file-time", 6, func(t *testing.T, m tuiModel) tuiModel {
			fresh, photos := newTestTUI(t, 6)
			fresh.dark = m.dark
			msgs := append(readAll(photos[:5]), exifResultMsg{image: photos[5].Path, err: photo.ErrNoShootingTime})
			return fresh.send(t, append(msgs, keys("G", "m")...)...)
		}},
		{"too-low", 6, func(t *testing.T, m tuiModel) tuiModel {
			m.height = minHeight - 1
			return m
		}},
	}
}

func TestGoldenTUI(t *testing.T) {
	for _, tc := range goldenCases() {
		for _, bg := range []struct {
			name string
			dark bool
		}{{"dark", true}, {"light", false}} {
			t.Run(tc.name+"/"+bg.name, func(t *testing.T) {
				m := tc.at(t, bg.dark)
				got := normalize(t, strings.Join(m.lines(goldenNow), "\n"))
				compareGolden(t, filepath.Join(testdataDir(t), "tui", tc.name+"-"+bg.name+".golden"), got)
			})
		}
	}
}

// goldenRun returns the run display of the photos keys, with what it has
// printed above its view held in its queue.
func goldenRun(keys []string, height int) *model {
	m := newTestModel(phaseStartMsg{kind: phaseExif, keys: keys, limit: 4}, height)
	m.now = func() time.Time { return goldenNow }
	m.phase.start = goldenNow
	return m
}

// runView is what the run display shows: what it printed, then its view.
func runView(m *model) string {
	return strings.Join(append(append([]string{}, m.queue...), m.View().Content), "\n")
}

func TestGoldenRun(t *testing.T) {
	keys := photoKeys(5)
	readSome := func(m *model) {
		for i, k := range keys[:3] {
			m.send(exifStartMsg{image: k})
			if i < 2 {
				m.send(exifResultMsg{image: k, photo: photo.Photo{Path: k}})
			}
		}
	}
	readAllOf := func(m *model) {
		for i, k := range keys {
			var err error
			if i == 3 {
				err = errors.New("File format error")
			}
			m.send(exifStartMsg{image: k}, exifResultMsg{image: k, photo: photo.Photo{Path: k}, err: err})
		}
	}
	renamed := func(m *model) {
		readAllOf(m)
		rk := append(append([]string{}, keys[:3]...), keys[4])
		m.send(phaseStartMsg{kind: phaseRename, keys: rk})
		for i, k := range rk {
			to := fmt.Sprintf("/p/2026-10-03/2026-10-03_14-00-%02d.heic", i)
			if i == 0 {
				to = k // already named
			}
			m.send(renameResultMsg{photo: photo.Photo{Name: filepath.Base(k), Path: k, RenamedPath: to}})
		}
	}
	cases := []struct {
		name   string
		keys   []string
		height int
		do     func(m *model)
	}{
		{"exif-reading", keys, 30, readSome},
		{"exif-compact", photoKeys(20), 30, func(m *model) {
			for i, k := range photoKeys(20)[:12] {
				m.send(exifStartMsg{image: k})
				if i%3 != 0 {
					m.send(exifResultMsg{image: k, photo: photo.Photo{Path: k}})
				}
			}
		}},
		{"renamed", keys, 30, func(m *model) { renamed(m); m.send(finishMsg{}) }},
		{"cleaned", keys, 30, func(m *model) {
			renamed(m)
			m.send(phaseStartMsg{kind: phaseClean})
			m.send(cleanResultMsg{dir: "/p/DCIM", empty: true}, cleanResultMsg{dir: "/p", empty: false})
			m.send(finishMsg{})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			goldenRoot(t)
			m := goldenRun(tc.keys, tc.height)
			tc.do(m)
			got := normalize(t, runView(m))
			compareGolden(t, filepath.Join(testdataDir(t), "run", tc.name+".golden"), got)
		})
	}
}

// testdataDir is testdata/golden of the package, found before goldenRoot
// moved the working directory.
var goldenDir = func() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return filepath.Join(wd, "testdata", "golden")
}()

func testdataDir(t *testing.T) string {
	t.Helper()
	return goldenDir
}

func compareGolden(t *testing.T, path, got string) {
	t.Helper()
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/ui -run TestGolden -update)", err)
	}
	if !sameView(got, string(want)) {
		t.Errorf("view differs from %s (run go test ./internal/ui -run TestGolden -update to accept)\n--- got ---\n%s--- want ---\n%s", path, got, want)
	}
}

// sameView compares two normalized views, letting each channel of a 24-bit
// color differ by one. Renderers round colors differently (lipgloss v1
// truncates #AD58B4 to #ad58b3), and a difference that small does not show.
func sameView(got, want string) bool {
	if got == want {
		return true
	}
	gl, wl := strings.Split(got, "\n"), strings.Split(want, "\n")
	if len(gl) != len(wl) {
		return false
	}
	styles := false // past "-- styles --", where colors may differ
	for i := range gl {
		if gl[i] == wl[i] {
			styles = styles || gl[i] == "-- styles --"
			continue
		}
		if !styles {
			return false
		}
		gt, wt := strings.Split(gl[i], " "), strings.Split(wl[i], " ")
		if len(gt) != len(wt) {
			return false
		}
		for j := range gt {
			if gt[j] != wt[j] && !closeColors(gt[j], wt[j]) {
				return false
			}
		}
	}
	return true
}

// closeColors reports whether a and b are the same fg= or bg= attribute
// with 24-bit colors no more than one apart in each channel.
func closeColors(a, b string) bool {
	if len(a) != len("fg=#rrggbb") || len(b) != len(a) || a[:4] != b[:4] {
		return false
	}
	if a[:4] != "fg=#" && a[:4] != "bg=#" {
		return false
	}
	for k := 4; k < len(a); k += 2 {
		x, err1 := strconv.ParseUint(a[k:k+2], 16, 8)
		y, err2 := strconv.ParseUint(b[k:k+2], 16, 8)
		if err1 != nil || err2 != nil || max(x, y)-min(x, y) > 1 {
			return false
		}
	}
	return true
}
