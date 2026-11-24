package ui

import (
	"image/color"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/naminator/internal/ui/statusline"
	"github.com/babarot/naminator/internal/ui/theme"
)

// states has a screen in each state.
func states(t *testing.T) map[uiState]tuiModel {
	t.Helper()
	m := newMouseTUI(t, 6)
	low := m
	low.height = minHeight - 1
	return map[uiState]tuiModel{
		stateList:    m,
		stateTree:    m.send(t, keys("v")...),
		stateEdit:    m.send(t, keys("i")...),
		stateConfirm: m.send(t, keys("enter")...),
		stateTooLow:  low,
	}
}

// A state added to uiState without a screen here fails, and so does one
// whose screen is not in it.
func TestEveryStateHasAScreenHere(t *testing.T) {
	all := states(t)
	for s := range numUIStates {
		m, ok := all[s]
		switch {
		case !ok:
			t.Errorf("no screen in state %q: add one to states", s)
		case m.uiState() != s:
			t.Errorf("the screen for state %q is in %q", s, m.uiState())
		}
		if s.String() == "" {
			t.Errorf("state %d has no name: add one to uiStateNames", s)
		}
	}
}

// Every state has a golden view.
func TestEveryStateIsPinned(t *testing.T) {
	seen := map[uiState]bool{}
	for _, tc := range goldenCases() {
		t.Run(tc.name, func(t *testing.T) { seen[tc.at(t, true).uiState()] = true })
	}
	for s := range numUIStates {
		if !seen[s] {
			t.Errorf("no golden case is in state %q: add one to goldenCases", s)
		}
	}
}

// Each state says its own keys at the bottom of the screen.
func TestEveryStateHasAKeyLine(t *testing.T) {
	seen := map[string]uiState{}
	for s, m := range states(t) {
		line := ansi.Strip(m.help())
		if strings.TrimSpace(line) == "" {
			t.Errorf("state %q has no key line: give it a case in help", s)
		}
		if other, ok := seen[line]; ok {
			t.Errorf("states %q and %q have the same key line %q", s, other, line)
		}
		seen[line] = s
		lines := m.lines()
		if last := ansi.Strip(lines[len(lines)-1]); !strings.Contains(last, line) {
			t.Errorf("state %q: the last line is %q, not its keys", s, last)
		}
	}
}

// ctrl+c cancels in every state.
func TestCtrlCInEveryState(t *testing.T) {
	for s, m := range states(t) {
		if m = m.send(t, keys("ctrl+c")...); m.result != Canceled {
			t.Errorf("state %q: ctrl+c gives %v", s, m.result)
		}
	}
}

// Over a question, or when the screen is not drawn, no key acts on what is
// under: only y renames, and only over the question.
func TestKeysStayInTheirState(t *testing.T) {
	for _, s := range []uiState{stateConfirm, stateTooLow} {
		for _, k := range []tea.Msg{
			tea.KeyPressMsg{Code: tea.KeyTab}, tea.KeyPressMsg{Code: tea.KeyUp}, tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeyPgDown},
			tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl}, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}, tea.KeyPressMsg{Code: tea.KeyEnter}, tea.KeyPressMsg{Code: tea.KeyEscape},
			keys("i")[0], keys("a")[0], keys("j")[0], keys("G")[0], keys("v")[0], keys("m")[0], keys("n")[0], keys("x")[0],
			tea.MouseWheelMsg{Button: tea.MouseWheelDown}, tea.MouseClickMsg{X: 4, Y: listTop + 2, Button: tea.MouseLeft},
		} {
			m := states(t)[s]
			got := m.send(t, k)
			if got.result != tuiRunning || got.format != m.format || got.view != m.view || got.sel != m.sel || got.insert || got.status() != "" {
				t.Errorf("state %q: %v acted on what is under (result %v, format %q, view %v, sel %d)", s, k, got.result, got.format, got.view, got.sel)
			}
		}
	}
	all := states(t)
	if m := all[stateTooLow].send(t, keys("enter", "y")...); m.result != tuiRunning {
		t.Errorf("too low: y gives %v", m.result)
	}
	// a question asked before the terminal got too low cannot be answered
	m := all[stateConfirm]
	m.height = minHeight - 1
	if m = m.send(t, keys("y")...); m.result != tuiRunning {
		t.Errorf("a question that is not drawn: y gives %v", m.result)
	}
	if m = all[stateTooLow].send(t, keys("q")...); m.result != Quit {
		t.Errorf("too low: q gives %v", m.result)
	}
	if m = all[stateConfirm].send(t, keys("y")...); m.result != Confirmed {
		t.Errorf("confirm: y gives %v", m.result)
	}
}

// The mouse works where nothing is asked and the screen is drawn.
func TestMouseInEveryState(t *testing.T) {
	for s, m := range states(t) {
		if got, want := m.View().MouseMode == tea.MouseModeCellMotion, s.takesMouse(); got != want {
			t.Errorf("state %q: mouse %v, want %v", s, got, want)
		}
	}
}

// The styles suit the terminal's background once it tells.
func TestBackgroundColor(t *testing.T) {
	goldenRoot(t)
	m := newMouseTUI(t, 3)
	for _, tt := range []struct {
		bg   color.Color
		dark bool
	}{{color.White, false}, {color.Black, true}} {
		m = m.send(t, tea.BackgroundColorMsg{Color: tt.bg})
		if got, want := m.st.CursorLine("x", false), theme.For(tt.dark).CursorLine("x", false); got != want {
			t.Errorf("dark %v: %q, want %q", tt.dark, got, want)
		}
	}
}

// What the status line shows goes by itself, with one timer for each.
func TestStatusGoes(t *testing.T) {
	m := newMouseTUI(t, 3)
	now := goldenNow
	m.now = func() time.Time { return now }
	save := tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}

	next, cmd := m.Update(save)
	m, _ = next.(tuiModel)
	if !strings.Contains(m.status(), "saved as the format") || cmd == nil {
		t.Fatalf("status = %q, timer %v", m.status(), cmd != nil)
	}
	// a key leaves it, and sets no other timer
	next, cmd = m.Update(keys("j")[0])
	m, _ = next.(tuiModel)
	if m.status() == "" || cmd != nil {
		t.Errorf("after a key: status = %q, timer %v", m.status(), cmd != nil)
	}
	now = now.Add(statusline.InfoFor)
	m = m.send(t, statusDueMsg{})
	if m.status() != "" || !m.timerAt.IsZero() {
		t.Errorf("at its end: status = %q, timerAt %v", m.status(), m.timerAt)
	}

	// told again before it goes, it stays longer, with a timer for that
	m = m.send(t, save)
	now = now.Add(time.Second)
	next, cmd = m.Update(save)
	m, _ = next.(tuiModel)
	if cmd == nil || !m.timerAt.Equal(now.Add(statusline.InfoFor)) {
		t.Errorf("told again: timer %v at %v", cmd != nil, m.timerAt)
	}
	now = now.Add(statusline.InfoFor - time.Second)
	next, cmd = m.Update(statusDueMsg{}) // the timer of the first
	m, _ = next.(tuiModel)
	if m.status() == "" || cmd == nil {
		t.Errorf("at the end of the first: status = %q, timer %v", m.status(), cmd != nil)
	}

	// a refusal goes at the next key
	m.setFormat("{nope}")
	m = m.send(t, save)
	if !strings.Contains(m.status(), "not saved") {
		t.Fatalf("status = %q", m.status())
	}
	if m = m.send(t, keys("j")...); m.status() != "" {
		t.Errorf("after a key: %q", m.status())
	}
}
