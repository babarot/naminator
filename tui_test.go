package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// newTestTUI returns a screen over n photos in card/, two of them taken in
// the same second, none read yet.
func newTestTUI(t *testing.T, n int) (tuiModel, []Photo) {
	t.Helper()
	var images []string
	var photos []Photo
	at := time.Date(2026, 10, 4, 14, 22, 0, 0, time.UTC)
	for i := range n {
		path := filepath.Join("card", fmt.Sprintf("DSC%05d.JPG", i))
		images = append(images, path)
		taken := at.Add(time.Duration(i) * time.Second)
		if i == 2 {
			taken = at.Add(time.Second) // the same second as photo 1
		}
		photos = append(photos, Photo{Name: filepath.Base(path), Path: path, Dir: "card", Extension: "jpg", CreatedAt: taken})
	}
	cfg := config{}
	m := newTUIModel([]string{"card"}, images, presets[0].format, "", false, false, cfg.allPresets(), osFS{}, filepath.Join(t.TempDir(), "config.yaml"), nil)
	m.width, m.height = 100, 30
	return m, photos
}

func (m tuiModel) send(t *testing.T, msgs ...tea.Msg) tuiModel {
	t.Helper()
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		nm, ok := next.(tuiModel)
		if !ok {
			t.Fatalf("Update returned %T", next)
		}
		m = nm
	}
	return m
}

func keys(s ...string) []tea.Msg {
	var out []tea.Msg
	for _, k := range s {
		switch k {
		case "enter":
			out = append(out, tea.KeyPressMsg{Code: tea.KeyEnter})
		case "esc":
			out = append(out, tea.KeyPressMsg{Code: tea.KeyEscape})
		case "tab":
			out = append(out, tea.KeyPressMsg{Code: tea.KeyTab})
		case "backspace":
			out = append(out, tea.KeyPressMsg{Code: tea.KeyBackspace})
		case "ctrl+c":
			out = append(out, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		default:
			for _, r := range k {
				out = append(out, tea.KeyPressMsg{Code: r, Text: string(r)})
			}
		}
	}
	return out
}

func readAll(photos []Photo) []tea.Msg {
	var out []tea.Msg
	for _, p := range photos {
		out = append(out, exifResultMsg{image: p.Path, photo: p})
	}
	return out
}

func TestTUIPlan(t *testing.T) {
	m, photos := newTestTUI(t, 4)
	m = m.send(t, readAll(photos[:3])...)
	if m.nRead != 3 || m.rows[3].read {
		t.Fatalf("read %d", m.nRead)
	}
	// in place; the photo of the same second gets _1, in the order taken
	if got := m.rows[1].target; got != "card/2026-10-04_14-22-01.jpg" {
		t.Errorf("row 1 = %q", got)
	}
	if m.rows[2].target != "card/2026-10-04_14-22-01_1.jpg" || m.rows[2].suffix != "_1" {
		t.Errorf("row 2 = %q (suffix %q)", m.rows[2].target, m.rows[2].suffix)
	}
	// a format that cannot be used stops the plan, with what to fix
	m.setFormat("{date}/{nope}")
	if m.tplErr == "" || !strings.Contains(strings.Join(m.lines(time.Now()), "\n"), "unknown placeholder {nope}") {
		t.Errorf("tplErr = %q", m.tplErr)
	}
	m.setFormat("{ext}/{date}/{date}_{time}")
	if got := m.rows[0].target; got != "jpg/2026-10-04/2026-10-04_14-22-00.jpg" {
		t.Errorf("row 0 = %q", got)
	}
	// a photo without a shooting time is skipped
	m = m.send(t, exifResultMsg{image: photos[3].Path, photo: Photo{Path: photos[3].Path}, err: errors.New("no date")})
	if !strings.Contains(m.countLine(), "1 without a shooting time") {
		t.Errorf("count line = %q", ansi.Strip(m.countLine()))
	}
}

func TestTUIModes(t *testing.T) {
	m, photos := newTestTUI(t, 3)
	m = m.send(t, readAll(photos)...)

	// normal mode: the keys move in the list, and do not edit
	m = m.send(t, keys("j", "j", "k")...)
	if m.sel != 1 || m.format != presets[0].format {
		t.Fatalf("sel = %d, format = %q", m.sel, m.format)
	}
	m = m.send(t, keys("G")...)
	if m.sel != 2 {
		t.Errorf("G: sel = %d", m.sel)
	}
	// insert mode: the keys edit the format, at its end with a
	m = m.send(t, keys("a", "_{orig}")...)
	if !m.insert || m.format != presets[0].format+"_{orig}" {
		t.Fatalf("insert = %v, format = %q", m.insert, m.format)
	}
	m = m.send(t, keys("backspace")...)
	if m.format != presets[0].format+"_{orig" || m.tplErr == "" {
		t.Errorf("format = %q, tplErr = %q", m.format, m.tplErr)
	}
	// enter only leaves insert mode: it never starts a rename
	m = m.send(t, keys("}", "enter")...)
	if m.insert || m.confirm {
		t.Errorf("insert = %v, confirm = %v", m.insert, m.confirm)
	}
	// esc leaves too
	m = m.send(t, keys("i", "esc")...)
	if m.insert {
		t.Error("esc should leave insert mode")
	}
}

func TestTUIPresets(t *testing.T) {
	m, _ := newTestTUI(t, 1)
	m = m.send(t, keys("tab")...)
	if m.format != presets[1].format || m.presetName() != presets[1].name {
		t.Errorf("tab: %q (%s)", m.format, m.presetName())
	}
	m = m.send(t, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.format != presets[len(presets)-1].format {
		t.Errorf("shift+tab twice: %q", m.format)
	}
	// from a format of one's own, the first preset
	m.setFormat("{date}")
	if m.presetName() != "custom" {
		t.Errorf("name = %q", m.presetName())
	}
	m = m.send(t, keys("tab")...)
	if m.format != presets[0].format {
		t.Errorf("tab from custom: %q", m.format)
	}
}

func TestTUIConfirm(t *testing.T) {
	m, photos := newTestTUI(t, 3)
	decided := make(chan tuiDecision, 1)
	m.decided = decided

	// not before every photo is read
	m = m.send(t, readAll(photos[:2])...)
	m = m.send(t, keys("enter")...)
	if m.confirm {
		t.Fatal("enter should wait for EXIF")
	}
	m = m.send(t, readAll(photos[2:])...)
	// n gives up the confirmation
	m = m.send(t, keys("enter", "n")...)
	if m.confirm || m.result != tuiRunning {
		t.Fatalf("confirm = %v, result = %v", m.confirm, m.result)
	}
	// y confirms; the display takes over after a frame, and only then is
	// the work told, so that the renames reach the display
	m = m.send(t, keys("enter", "y")...)
	if m.result != tuiConfirmed || len(decided) != 0 {
		t.Fatalf("result = %v, decided %d", m.result, len(decided))
	}
	if m.View().Content != "" {
		t.Error("the screen should be gone before the display takes over")
	}
	// while it waits, nothing changes the format confirmed
	m = m.send(t, keys("tab", "i", "x")...)
	if m.format != presets[0].format || m.insert {
		t.Fatalf("format = %q, insert = %v after the confirmation", m.format, m.insert)
	}
	m = m.send(t, handoverMsg{})
	if m.run == nil || len(decided) != 1 {
		t.Fatalf("run = %v, decided %d", m.run, len(decided))
	}
	if d := <-decided; d.format != presets[0].format {
		t.Errorf("decided %+v", d)
	}
}

// Ctrl+C while the display is about to take over gives up, and tells the
// work so.
func TestTUICancelAfterConfirm(t *testing.T) {
	m, photos := newTestTUI(t, 1)
	decided := make(chan tuiDecision, 1)
	m.decided = decided
	m = m.send(t, readAll(photos)...)
	m = m.send(t, keys("enter", "y", "ctrl+c")...)
	m = m.send(t, handoverMsg{})
	if m.result != tuiCanceled || m.run != nil || len(decided) != 1 {
		t.Fatalf("result = %v, run = %v, decided %d", m.result, m.run, len(decided))
	}
	if d := <-decided; d.result != tuiCanceled {
		t.Errorf("decided %+v", d)
	}
}

func TestTUIQuit(t *testing.T) {
	for _, tt := range []struct {
		key  string
		want tuiResult
	}{{"q", tuiQuit}, {"ctrl+c", tuiCanceled}} {
		m, _ := newTestTUI(t, 2)
		canceled := false
		m.cancel = func() { canceled = true }
		decided := make(chan tuiDecision, 1)
		m.decided = decided
		m = m.send(t, keys(tt.key)...)
		if m.result != tt.want || !canceled || len(decided) != 1 {
			t.Errorf("%s: result = %v, canceled = %v, decided %d", tt.key, m.result, canceled, len(decided))
		}
	}
	// a signal is Ctrl+C
	m, _ := newTestTUI(t, 1)
	if m = m.send(t, interruptMsg{}); m.result != tuiCanceled {
		t.Errorf("interrupt: %v", m.result)
	}
}

func TestTUIView(t *testing.T) {
	m, photos := newTestTUI(t, 40)
	m = m.send(t, readAll(photos[:20])...)
	for _, size := range [][2]int{{100, 30}, {60, 14}, {140, 50}} {
		m.width, m.height = size[0], size[1]
		for _, mode := range []struct {
			insert bool
			view   tuiView
		}{{false, viewList}, {true, viewList}, {false, viewTree}} {
			m.insert, m.view = mode.insert, mode.view
			insert := mode.insert
			lines := m.lines(time.Now())
			if len(lines) > m.viewHeight() || len(lines)+1 > max(size[1], 13) {
				t.Errorf("%v insert=%v: %d lines", size, insert, len(lines))
			}
			for _, l := range lines {
				if w := ansi.StringWidth(l); w > size[0] {
					t.Errorf("%v: a line of %d cells: %q", size, w, ansi.Strip(l))
				}
			}
		}
	}
}

func TestSaveFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "naminator", "config.yaml")
	if err := saveFormat(path, "{ext}/{date}"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "format: \"{ext}/{date}\"\n" {
		t.Errorf("new file = %q", b)
	}
	// the format line is replaced; comments and the rest are kept
	if err := os.WriteFile(path, []byte("# mine\nclean: true\nformat: \"{date}\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := saveFormat(path, "{year}/{date}"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "# mine\nclean: true\nformat: \"{year}/{date}\"\n" {
		t.Errorf("replaced = %q", b)
	}
	if c, err := loadConfig(path); err != nil || c.Format != "{year}/{date}" || !c.Clean {
		t.Errorf("config = %+v, %v", c, err)
	}
}

func TestStartFormat(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir, "card/a.jpg", "b.HEIC")
	card, file := filepath.Join(dir, "card"), filepath.Join(dir, "b.HEIC")
	cfg := config{Format: "{ext}/{date}/{date}_{time}"}
	tests := []struct {
		name string
		opt  Option
		cfg  config
		args []string
		want string
	}{
		{"the config's format", Option{}, cfg, []string{card}, cfg.Format},
		{"files only: in place", Option{}, cfg, []string{file}, presets[0].format},
		{"no config: in place", Option{}, config{}, []string{card}, presets[0].format},
		{"--format first", Option{Format: "{date}"}, cfg, []string{file}, "{date}"},
		{"--preset", Option{Preset: "by date"}, cfg, []string{card}, presets[1].format},
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

// newCardsTUI returns a screen over two card folders in a temporary
// directory, all read, with --clean: card1 left empty, card2 keeping a
// photo without a shooting time.
func newCardsTUI(t *testing.T) tuiModel {
	t.Helper()
	root := t.TempDir()
	at := time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)
	var images []string
	var msgs []tea.Msg
	for i := range 8 {
		card := "card1"
		if i >= 6 {
			card = "card2"
		}
		name := fmt.Sprintf("DSC%04d.JPG", i)
		makeTree(t, root, card+"/"+name)
		path := filepath.Join(root, card, name)
		images = append(images, path)
		p := Photo{Name: name, Path: path, Dir: filepath.Join(root, card), Extension: "jpg", CreatedAt: at.Add(time.Duration(i) * time.Minute)}
		if i == 7 {
			msgs = append(msgs, exifResultMsg{image: path, photo: Photo{Path: path}, err: errors.New("no date")})
			continue
		}
		msgs = append(msgs, exifResultMsg{image: path, photo: p})
	}
	cfg := config{}
	m := newTUIModel([]string{filepath.Join(root, "card1"), filepath.Join(root, "card2")}, images, "{ext}/{date}/{date}_{time}", "", false, true, cfg.allPresets(), osFS{}, filepath.Join(root, "config.yaml"), nil)
	m.width, m.height = 120, 40
	return m.send(t, msgs...)
}

func treeText(m tuiModel) []string {
	var out []string
	for _, it := range m.items() {
		out = append(out, ansi.Strip(it.draw(false)))
	}
	return out
}

func TestTUITree(t *testing.T) {
	m := newCardsTUI(t)
	m = m.send(t, keys("t")...)
	if m.view != viewTree {
		t.Fatal("t should show the tree")
	}
	got := strings.Join(treeText(m), "\n")
	for _, want := range []string{
		"card1/  (removed by -c)",
		"card2/  1 · 1 to check",
		"DSC0007.JPG  (no shooting time)",
		"jpg/  7",
		"2026-10-04/  7",
		"2026-10-04_09-00-00.jpg  ← ",
		"⋯ 4 more files",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("tree lacks %q:\n%s", want, got)
		}
	}
	if !strings.Contains(m.countLine(), "1 directory removed") {
		t.Errorf("count line = %q", ansi.Strip(m.countLine()))
	}
	// the parts line says what to do on a directory
	if !strings.Contains(m.partsLine(), "h folds it") {
		t.Errorf("parts line on the top = %q", ansi.Strip(m.partsLine()))
	}
}

// A directory folded earlier can become the top of the tree when the
// format changes: the top is never folded, and l on it opens everything.
// The cursor stays on a line when the tree gets shorter.
func TestTUITreeAfterFormatChange(t *testing.T) {
	m := newCardsTUI(t).send(t, keys("t")...)
	// fold every directory, then pick a format with fewer of them
	m = m.send(t, keys("g", "h", "G")...)
	for _, it := range m.items() {
		if it.kind == itemDir {
			m.folded[it.path] = true
		}
	}
	m.setFormat("{date}_{time}")
	m = m.send(t, keys("G", "g")...)
	m = m.send(t, keys("l")...)
	if !strings.Contains(strings.Join(treeText(m), "\n"), "2026-10-04_09-00-00.jpg") {
		t.Errorf("l on the top should open everything:\n%s", strings.Join(treeText(m), "\n"))
	}
	// the cursor at the end, then fewer lines
	m = m.send(t, keys("G")...)
	m = m.send(t, keys("g", "h")...)
	m.sel = 999
	m = m.send(t, tea.WindowSizeMsg{Width: 120, Height: 40})
	if n := len(m.items()); m.sel >= n {
		t.Errorf("sel = %d with %d lines", m.sel, n)
	}
}

// A tree without directories under its top folds the top itself, and l
// on it opens everything again.
func TestTUITreeFoldTop(t *testing.T) {
	m, photos := newTestTUI(t, 10)
	m = m.send(t, readAll(photos)...)
	m = m.send(t, keys("t")...)
	if n := len(m.items()); n < 3 {
		t.Fatalf("tree has %d lines", n)
	}
	m = m.send(t, keys("g", "h")...)
	if n := len(m.items()); n != 1 || !strings.Contains(treeText(m)[0], "▸") {
		t.Fatalf("h on the top should fold it:\n%s", strings.Join(treeText(m), "\n"))
	}
	m = m.send(t, keys("l")...)
	if n := len(m.items()); n < 3 || !strings.Contains(treeText(m)[0], "▾") {
		t.Fatalf("l on the top should open it:\n%s", strings.Join(treeText(m), "\n"))
	}
	// the arrows do the same
	m = m.send(t, tea.KeyPressMsg{Code: tea.KeyLeft})
	if len(m.items()) != 1 {
		t.Error("← should fold the top")
	}
	m = m.send(t, tea.KeyPressMsg{Code: tea.KeyRight})
	if len(m.items()) < 3 {
		t.Error("→ should open the top")
	}
}

func TestTUITreeFold(t *testing.T) {
	m := newCardsTUI(t).send(t, keys("t")...)
	find := func(m tuiModel, text string) int {
		for i, line := range treeText(m) {
			if strings.Contains(line, text) {
				return i
			}
		}
		t.Fatalf("no line with %q:\n%s", text, strings.Join(treeText(m), "\n"))
		return -1
	}
	// l on a "more" line shows the files it hides; h on one of them folds
	// them again and goes up to their directory
	m.sel = find(m, "more files")
	m = m.send(t, keys("l")...)
	if strings.Contains(strings.Join(treeText(m), "\n"), "more files") {
		t.Fatal("l should show the hidden files")
	}
	m.sel = find(m, "09-03-00")
	m = m.send(t, keys("h")...)
	if !strings.Contains(strings.Join(treeText(m), "\n"), "more files") || !strings.Contains(treeText(m)[m.sel], "2026-10-04/") {
		t.Fatalf("h should fold the files and go up: sel on %q", treeText(m)[m.sel])
	}
	// h folds an open directory, l opens it again
	m = m.send(t, keys("h")...)
	if strings.Contains(strings.Join(treeText(m), "\n"), "2026-10-04_09-00-00") || !strings.Contains(treeText(m)[m.sel], "▸") {
		t.Fatal("h should fold the directory")
	}
	m = m.send(t, keys("l")...)
	if !strings.Contains(strings.Join(treeText(m), "\n"), "2026-10-04_09-00-00") {
		t.Fatal("l should open the directory")
	}
	// on the top line, h folds every directory
	m = m.send(t, keys("g", "h")...)
	if n := len(m.items()); n != 4 {
		t.Errorf("after folding all: %d lines:\n%s", n, strings.Join(treeText(m), "\n"))
	}
	// h and l do nothing in the list
	m = m.send(t, keys("t", "j", "h")...)
	if m.view != viewList || m.sel != 1 {
		t.Errorf("view = %v, sel = %d", m.view, m.sel)
	}
}

// m gives a photo without a shooting time the file's modification time,
// and takes it back.
func TestTUIFileTime(t *testing.T) {
	m := newCardsTUI(t)
	decided := make(chan tuiDecision, 1)
	m.decided = decided
	r := m.rows[7] // read without a shooting time
	mtime := time.Date(2026, 10, 6, 18, 2, 11, 0, time.Local)
	if err := os.Chtimes(r.key, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	// m does nothing on a photo with a shooting time
	m.sel = 0
	m = m.send(t, keys("m")...)
	if m.countFileTime() != 0 || strings.Contains(ansi.Strip(m.help()), "file time") {
		t.Fatal("m should do nothing on a photo with a shooting time")
	}
	m.sel = 7
	if !strings.Contains(ansi.Strip(m.help()), "m file time") {
		t.Errorf("help = %q, want m", ansi.Strip(m.help()))
	}
	// on a narrow terminal the keys still fit, q included
	m.width = 100
	if h := ansi.Strip(m.help()); ansi.StringWidth(h) > 98 || !strings.HasSuffix(h, "q quit") {
		t.Errorf("help on 100 columns = %q", h)
	}
	m.width = 120
	m = m.send(t, keys("m")...)
	r = m.rows[7]
	if !r.fileTime || !r.usable() || !strings.HasSuffix(r.target, "jpg/2026-10-06/2026-10-06_18-02-11.jpg") {
		t.Fatalf("fileTime = %v, target = %q", r.fileTime, r.target)
	}
	line := ansi.Strip(m.row(r, false, 20))
	if !strings.Contains(line, iconFileTime) || !strings.Contains(line, "(file time)") {
		t.Errorf("row = %q", line)
	}
	if c := ansi.Strip(m.countLine()); !strings.Contains(c, "1 by file time") || strings.Contains(c, "without a shooting time") {
		t.Errorf("count line = %q", c)
	}
	// renamed with the others once confirmed, and not reported as failed
	m = m.send(t, keys("enter", "y")...)
	m = m.send(t, handoverMsg{})
	d := <-decided
	if len(d.fileTime) != 1 || d.fileTime[0].Path != r.key || len(d.failed) != 0 {
		t.Errorf("decided %+v", d)
	}
	// m again takes it back
	m2 := newCardsTUI(t)
	m2.sel = 7
	m2 = m2.send(t, keys("m", "m")...)
	if m2.rows[7].fileTime || !m2.rows[7].skipped() {
		t.Error("m twice should skip the photo again")
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
	photos := []Photo{
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
		paths:   newPathReserver(),
		moves:   newPendingMoves(),
		sender:  &mockSender{},
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		namer:   newNamer(mustParseTemplate("{dir}/{date}_{time}"), []string{card}, ""),
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
	cli.paths = newPathReserver()
	cli.planned = map[string]string{filepath.Join(card2, "a.jpg"): filepath.Join(card2, "taken.jpg")}
	cli.namer = newNamer(mustParseTemplate("{dir}/taken"), []string{card2}, "")
	p, _, err := cli.rename(Photo{Name: "a.jpg", Path: filepath.Join(card2, "a.jpg"), Dir: card2, Extension: "jpg", CreatedAt: at})
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
	got, err := startFormat(Option{DestDir: "/out"}, config{}, []string{filepath.Join(dir, "card")})
	if err != nil || got != "{date}_{time}" {
		t.Errorf("startFormat = %q, %v", got, err)
	}
	if got, _ := startFormat(Option{DestDir: "/out"}, config{Format: "{ext}/{date}"}, []string{filepath.Join(dir, "card")}); got != "{ext}/{date}" {
		t.Errorf("with a config: %q", got)
	}
}

// The screen takes the lines it has to show, at most half the terminal or
// the rows from the prompt down, as gomi's; nothing is drawn until the
// terminal tells where the prompt is, or the wait is over.
func TestTUIHeight(t *testing.T) {
	m, photos := newTestTUI(t, 3)
	m.width, m.height = 100, 40
	m = m.send(t, readAll(photos)...)
	if m.View().Content != "" {
		t.Fatal("nothing should be drawn before the cursor position")
	}
	m = m.send(t, cursorWaitedMsg{})
	if got := len(m.lines(time.Now())); got != 3+8 {
		t.Errorf("3 photos: %d lines, want %d", got, 3+8)
	}

	many, photos := newTestTUI(t, 100)
	many.width, many.height = 100, 40
	many = many.send(t, readAll(photos)...)
	many = many.send(t, cursorWaitedMsg{})
	if got := len(many.lines(time.Now())); got != 40/2-1 {
		t.Errorf("100 photos, cursor unknown: %d lines, want %d", got, 40/2-1)
	}
	// the prompt near the top: the rows from it down
	many = many.send(t, tea.CursorPositionMsg{X: 0, Y: 5})
	if got := len(many.lines(time.Now())); got != 40-5-1 {
		t.Errorf("100 photos, prompt on row 5: %d lines, want %d", got, 40-5-1)
	}
	// the prompt near the bottom: half the terminal
	many = many.send(t, tea.CursorPositionMsg{X: 0, Y: 35})
	if got := len(many.lines(time.Now())); got != 40/2-1 {
		t.Errorf("100 photos, prompt on row 35: %d lines, want %d", got, 40/2-1)
	}
}

// Photos already named are not renamed: with nothing to move, enter does
// not ask, and the question counts only the photos that move.
func TestTUIConfirmCountsMoves(t *testing.T) {
	m, photos := newTestTUI(t, 2)
	for i := range photos {
		p := &photos[i]
		p.Name = p.CreatedAt.Format("2006-01-02_15-04-05") + ".jpg"
		p.Path = filepath.Join("card", p.Name)
		m.rows[i].key = p.Path
		m.byKey[p.Path] = m.rows[i]
	}
	m = m.send(t, readAll(photos)...)
	m = m.send(t, keys("enter")...)
	if m.confirm || !strings.Contains(ansi.Strip(m.help()), "nothing to rename") {
		t.Errorf("confirm = %v, help = %q", m.confirm, ansi.Strip(m.help()))
	}
	m.setFormat("{date}/{date}_{time}")
	m = m.send(t, keys("enter")...)
	if !m.confirm || !strings.Contains(ansi.Strip(m.help()), "Rename 2 photos?") {
		t.Errorf("confirm = %v, help = %q", m.confirm, ansi.Strip(m.help()))
	}
}

func TestTUIWhere(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	m := tuiModel{args: []string{"/home/u/Desktop/photos/", "card"}}
	if got := m.where(); got != "~/Desktop/photos, card" {
		t.Errorf("where = %q", got)
	}
}
