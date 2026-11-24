package ui

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/babarot/naminator/internal/photo"

	tea "charm.land/bubbletea/v2"
)

// newTestModel returns a model whose prints are held in its queue: with
// printing set, flush sends nothing, so a test reads what would be printed.
func newTestModel(first phaseStartMsg, height int) *model {
	m := newModel(first, nil)
	m.width, m.height = 100, height
	m.now = func() time.Time { return t0 }
	m.phase.start = t0
	m.printing = true
	return &m
}

func (m *model) send(msgs ...tea.Msg) {
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		nm, ok := next.(model)
		if !ok {
			panic(fmt.Sprintf("Update returned %T", next))
		}
		*m = nm
	}
}

func (m *model) printed() string {
	return plain(strings.Join(m.queue, "\n"))
}

func photoKeys(n int) []string {
	var keys []string
	for i := range n {
		keys = append(keys, fmt.Sprintf("/p/IMG_%04d.HEIC", i))
	}
	return keys
}

func TestModelCompactRun(t *testing.T) {
	keys := photoKeys(20)
	m := newTestModel(phaseStartMsg{kind: phaseExif, keys: keys, limit: 4}, 30)
	if title := m.startCompact(); title != "Reading EXIF of 20 photos\n" {
		t.Fatalf("title to print first = %q", title)
	}
	for i, k := range keys {
		var err error
		if i == 3 {
			err = errors.New("File format error\nsecond line")
		}
		m.send(exifStartMsg{image: k}, exifResultMsg{image: k, photo: photo.Photo{Path: k}, err: err})
	}
	// only the failure is printed while reading; the rest is counted
	if got := m.printed(); !strings.Contains(got, "✗ IMG_0003.HEIC") || strings.Contains(got, "IMG_0004") {
		t.Errorf("printed while reading:\n%s", got)
	}
	if view := plain(m.View().Content); !strings.Contains(view, "19 read") || !strings.Contains(view, "20/20 photos") {
		t.Errorf("view:\n%s", view)
	}

	renameKeys := append(append([]string{}, keys[:3]...), keys[4:]...)
	m.send(phaseStartMsg{kind: phaseRename, keys: renameKeys})
	for i, k := range renameKeys {
		to := fmt.Sprintf("/p/2026-10-03_14-00-%02d.heic", i)
		if i == 0 {
			to = k // already named
		}
		m.send(renameResultMsg{photo: photo.Photo{Name: k[3:], Path: k, RenamedPath: to}})
	}
	m.send(finishMsg{})

	got := m.printed()
	for _, want := range []string{
		"  ✓ 19 read\n\n✗ Read EXIF of 20 photos in 0.0s, 1 failed\n\nRenaming 19 photos\n",
		"  ✓ IMG_0001.HEIC  → /p/2026-10-03_14-00-01.heic\n",
		"  ✓ 1 already named\n\n✓ Renamed 18 photos in 0.0s, 1 already named\n",
		"\n\n  IMG_0003.HEIC:\n    File format error\n    second line\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("printed lacks %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "IMG_0001.HEIC  →") != 1 {
		t.Errorf("a row should be printed once:\n%s", got)
	}
	if !m.finished || m.View().Content != "" {
		t.Error("the view should be empty once finished")
	}
}

func TestModelFullView(t *testing.T) {
	keys := photoKeys(3)
	m := newTestModel(phaseStartMsg{kind: phaseExif, keys: keys, limit: 3}, 30)
	if title := m.startCompact(); title != "" {
		t.Fatalf("few rows should start whole, got title %q", title)
	}
	m.send(exifStartMsg{image: keys[0]})
	view := plain(m.View().Content)
	for _, want := range []string{"Reading EXIF of 3 photos", "0/3 photos", "IMG_0000.HEIC", "exiftool", "queued", "ctrl+c to cancel"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if len(m.queue) != 0 {
		t.Errorf("nothing should be printed while drawn whole, got %q", m.queue)
	}
	for _, k := range keys {
		m.send(exifResultMsg{image: k, photo: photo.Photo{Path: k}})
	}
	m.send(phaseStartMsg{kind: phaseRename, keys: keys})
	// the whole view of the phase is kept above, its heading now the outcome
	got := m.printed()
	if !strings.HasPrefix(got, "✓ Read EXIF of 3 photos in 0.0s\n\n  ✓ IMG_0000.HEIC") || strings.Contains(got, "ctrl+c") {
		t.Errorf("printed:\n%s", got)
	}
}

func TestModelCtrlC(t *testing.T) {
	keys := photoKeys(20)
	canceled := false
	m := newTestModel(phaseStartMsg{kind: phaseExif, keys: keys, limit: 2}, 30)
	m.cancel = func() { canceled = true }
	m.startCompact()
	m.send(exifStartMsg{image: keys[0]}, exifStartMsg{image: keys[1]}, exifResultMsg{image: keys[0], photo: photo.Photo{Path: keys[0]}})
	m.send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !canceled {
		t.Fatal("Ctrl+C should cancel the work")
	}
	// the running exiftool still reports; it is not shown
	m.send(exifResultMsg{image: keys[1], err: errors.New("late")}, finishMsg{})
	got := m.printed()
	if !strings.Contains(got, "✗ IMG_0001.HEIC (canceled)") || !strings.Contains(got, "✗ Reading EXIF of 20 photos (canceled)") {
		t.Errorf("printed:\n%s", got)
	}
	if strings.Contains(got, "late") {
		t.Errorf("a result after Ctrl+C should not be reported:\n%s", got)
	}
}

func TestModelInterrupt(t *testing.T) {
	keys := photoKeys(3)
	canceled := false
	m := newTestModel(phaseStartMsg{kind: phaseExif, keys: keys, limit: 1}, 30)
	m.cancel = func() { canceled = true }
	m.send(exifStartMsg{image: keys[0]}, interruptMsg{}, finishMsg{})
	if !canceled {
		t.Fatal("a signal should cancel the work")
	}
	if got := m.printed(); !strings.Contains(got, "✗ IMG_0000.HEIC (canceled)") || !strings.Contains(got, "(canceled)") {
		t.Errorf("printed:\n%s", got)
	}
}

func TestModelFlush(t *testing.T) {
	m := newTestModel(phaseStartMsg{kind: phaseExif}, 30)
	m.printing = false
	m.queue = []string{"a", "b"}
	if cmd := m.flush(); cmd == nil || !m.printing || len(m.queue) != 0 {
		t.Fatalf("flush should send the batch: cmd=%v printing=%v queue=%v", cmd, m.printing, m.queue)
	}
	m.queue = []string{"c"}
	if cmd := m.flush(); cmd != nil {
		t.Error("a second batch should wait for the first")
	}
	m.printing = false
	m.queue = nil
	m.finished = true
	if cmd := m.flush(); cmd == nil {
		t.Fatal("should quit once finished and printed")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("cmd() = %T, want tea.QuitMsg", cmd())
	}
}

func TestPlainSender(t *testing.T) {
	var buf bytes.Buffer
	keys := photoKeys(3)
	s := newPlainSender(&buf, phaseStartMsg{kind: phaseExif, keys: keys})
	for i, k := range keys {
		var err error
		if i == 1 {
			err = errors.New("bad")
		}
		s.Send(exifStartMsg{image: k})
		s.Send(exifResultMsg{image: k, photo: photo.Photo{Path: k}, err: err})
	}
	s.Send(phaseStartMsg{kind: phaseRename, keys: []string{keys[0]}, dry: true})
	s.Send(renameResultMsg{photo: photo.Photo{Name: "IMG_0000.HEIC", Path: keys[0], RenamedPath: "/p/x.heic"}, dryrun: true})
	s.Send(finishMsg{})

	got := plain(buf.String())
	want := strings.Join([]string{
		"Reading EXIF of 3 photos",
		"  ✗ IMG_0001.HEIC   0.0s  bad",
		"✗ Read EXIF of 3 photos in 0.0s, 1 failed",
		"",
		"Planning 1 rename (dry run)",
		"  ~ IMG_0000.HEIC  → /p/x.heic",
		"✓ Would rename 1 photo (dry run)",
		"",
		"  IMG_0001.HEIC:",
		"    bad",
		"",
		"",
	}, "\n")
	if got != want {
		t.Errorf("output:\n%s\nwant:\n%s", got, want)
	}
}

// With awaitReply, the run quits once the terminal has answered where the
// cursor is, or after the wait: answers to bubbletea's own queries would
// otherwise reach the shell after a run that ends at once.
func TestModelAwaitsReply(t *testing.T) {
	var quits func(cmd tea.Cmd) bool
	quits = func(cmd tea.Cmd) bool {
		for cmd != nil {
			msg := cmd()
			switch msg := msg.(type) {
			case tea.QuitMsg:
				return true
			case tea.BatchMsg:
				return slices.ContainsFunc(msg, quits)
			default:
				return false
			}
		}
		return false
	}
	for _, reply := range []tea.Msg{tea.CursorPositionMsg{}, repliedMsg{}} {
		m := newTestModel(phaseStartMsg{kind: phaseRename, keys: photoKeys(1)}, 30)
		m.printing = false
		m.awaitReply = true
		step := func(msg tea.Msg) tea.Cmd {
			next, cmd := m.Update(msg)
			nm, ok := next.(model)
			if !ok {
				t.Fatalf("Update returned %T", next)
			}
			*m = nm
			return cmd
		}
		cmd := step(finishMsg{})
		// what was queued is printed first
		for !quits(cmd) && m.printing {
			cmd = step(printedMsg{})
		}
		if quits(cmd) {
			t.Fatalf("%T: quit before the terminal answered", reply)
		}
		if cmd = step(reply); !quits(cmd) {
			t.Errorf("%T: did not quit once answered", reply)
		}
	}
}
