package ui

import (
	"image/color"
	"math/rand/v2"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/naminator/internal/photo"
)

// Moving the cursor and drawing the screen take as much with many photos as
// with few: what a key costs must not grow with them.
func TestKeyCostDoesNotGrow(t *testing.T) {
	for _, base := range []string{"/Volumes/card", "card"} {
		for _, v := range []tuiView{viewList, viewTree} {
			cost := func(n int) float64 {
				m := benchRead(t, n, base)
				m.view = v
				m = m.update0(t, tea.WindowSizeMsg{Width: 160, Height: 60})
				i := 0
				return testing.AllocsPerRun(20, func() {
					msg := keyDown
					if i++; i%2 == 0 {
						msg = keyUp
					}
					m = m.update0(t, msg)
					m.View()
				})
			}
			// more than fit the screen, so that as many lines are drawn
			few, many := cost(200), cost(3000)
			if many > few*1.2 {
				t.Errorf("%s, %s: %.0f allocations with 200 photos, %.0f with 3000", base, v, few, many)
			}
		}
	}
}

// The lines kept show what building them again would, whatever the screen
// has been told.
func TestKeptLinesAreRight(t *testing.T) {
	goldenRoot(t)
	rnd := rand.New(rand.NewPCG(1, 2))
	for range 20 {
		m, photos := newTestTUI(t, 30)
		pending := readAll(photos)
		pending[7] = exifResultMsg{image: photos[7].Path, err: photo.ErrNoShootingTime}
		for step := range 120 {
			var msg tea.Msg
			switch r := rnd.IntN(10); {
			case r < 3 && len(pending) > 0:
				msg, pending = pending[0], pending[1:]
			case r < 4:
				msg = tea.MouseClickMsg{X: 4, Y: listTop + rnd.IntN(20), Button: tea.MouseLeft}
			case r < 5:
				msg = tea.BackgroundColorMsg{Color: color.Gray{Y: uint8(rnd.IntN(256))}}
			default:
				all := []tea.Msg{
					keyDown, keyUp, keys("v")[0], keys("h")[0], keys("l")[0], keys("m")[0], keys("g")[0], keys("G")[0], keys("tab")[0], keys("i")[0], keys("esc")[0],
					keys("backspace")[0], keys("x")[0], tea.WindowSizeMsg{Width: 60 + rnd.IntN(120), Height: 12 + rnd.IntN(60)},
				}
				msg = all[rnd.IntN(len(all))]
			}
			m = m.send(t, msg)
			fresh := m
			fresh.list = itemList{}
			got, want := strings.Join(m.lines(), "\n"), strings.Join(fresh.lines(), "\n")
			if got != want {
				t.Fatalf("step %d, after %T %v: the lines kept\n%s\n--- building again\n%s", step, msg, msg, ansi.Strip(got), ansi.Strip(want))
			}
		}
	}
}
