package ui

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/babarot/naminator/internal/config"
	"github.com/babarot/naminator/internal/photo"
	"github.com/babarot/naminator/internal/rename"

	tea "charm.land/bubbletea/v2"
)

// benchTUI returns a screen over n photos in two directories of base, none
// read, and what reading them tells. The photos are not on disk.
func benchTUI(tb testing.TB, n int, base string) (tuiModel, []tea.Msg) {
	tb.Helper()
	at := time.Date(2026, 10, 4, 14, 22, 0, 0, time.UTC)
	var images []string
	var msgs []tea.Msg
	for i := range n {
		dir := filepath.Join(base, fmt.Sprintf("%dMSDCF", 100+i%2))
		p := filepath.Join(dir, fmt.Sprintf("DSC%05d.ARW", i))
		images = append(images, p)
		msgs = append(msgs, exifResultMsg{image: p, photo: photo.Photo{
			Name: filepath.Base(p), Path: p, Dir: dir, Extension: "arw", Camera: "ILCE-7CM2", CreatedAt: at.Add(time.Duration(i) * 7 * time.Second),
		}})
	}
	cfg := config.Config{}
	m := newTUIModel([]string{base}, images, rename.Presets[0].Format, "", false, false, cfg.AllPresets(), diskFS{}, filepath.Join(tb.TempDir(), "config.yaml"), nil)
	m.width, m.height = 160, 60
	return m, msgs
}

// benchRead returns the screen with every photo read.
func benchRead(tb testing.TB, n int, base string) tuiModel {
	tb.Helper()
	m, msgs := benchTUI(tb, n, base)
	for _, msg := range msgs {
		if e, ok := msg.(exifResultMsg); ok {
			r := m.byKey[e.image]
			r.read, r.photo = true, e.photo
			m.nRead++
		}
	}
	m.plan()
	return m.update0(tb, tea.WindowSizeMsg{Width: 160, Height: 60})
}

func (m tuiModel) update0(tb testing.TB, msg tea.Msg) tuiModel {
	tb.Helper()
	next, _ := m.Update(msg)
	nm, ok := next.(tuiModel)
	if !ok {
		tb.Fatalf("Update returned %T", next)
	}
	return nm
}

var (
	keyDown = tea.KeyPressMsg{Code: 'j', Text: "j"}
	keyUp   = tea.KeyPressMsg{Code: 'k', Text: "k"}
)

// BenchmarkKey is moving the cursor by a line: what a key changes, and the
// screen drawn after it.
func BenchmarkKey(b *testing.B) {
	for _, base := range []string{"/Volumes/card", "card"} {
		for _, v := range []tuiView{viewList, viewTree} {
			for _, n := range []int{50, 1000, 5000} {
				b.Run(fmt.Sprintf("%s/%s/n=%d", map[bool]string{true: "absolute", false: "relative"}[filepath.IsAbs(base)], v, n), func(b *testing.B) {
					m := benchRead(b, n, base)
					m.view = v
					m = m.update0(b, tea.WindowSizeMsg{Width: 160, Height: 60})
					b.ReportAllocs()
					for i := 0; b.Loop(); i++ {
						msg := keyDown
						if i%2 == 1 {
							msg = keyUp
						}
						m = m.update0(b, msg)
						m.View()
					}
				})
			}
		}
	}
}

// BenchmarkReading is what the screen does while n photos are read: what
// bubbletea does for the results gathered is an Update and a View.
func BenchmarkReading(b *testing.B) {
	for _, n := range []int{50, 300, 1000} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			for b.Loop() {
				b.StopTimer()
				m, msgs := benchTUI(b, n, "/Volumes/card")
				b.StartTimer()
				var results exifResultsMsg
				for i, msg := range msgs {
					if r, ok := msg.(exifResultMsg); ok {
						results = append(results, r)
					}
					// as many as ten processes read in gatherEvery
					if len(results) == 40 || i == len(msgs)-1 {
						m = m.update0(b, results)
						m.View()
						results = nil
					}
				}
			}
		})
	}
}
