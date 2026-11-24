package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The mouse only points and moves, as in gomi: the wheel moves the cursor,
// a click puts it on a line (a second click on a directory folds or opens
// it), a click on the format edits it there, on the preset goes to the
// next one, and on the view's name switches the list and the tree.
// Confirming and renaming stay on keys, and the mouse does nothing while
// the screen asks y/N.

// mouseOn reports whether the screen takes the mouse.
func (m tuiModel) mouseOn() bool {
	return m.result == tuiRunning && m.uiState().takesMouse()
}

// The rows of the screen: the header, the format's box, the parts or what
// is wrong with the format, then the list.
const (
	rowHeader = 0
	rowFormat = 2 // the box's line with the format
	listTop   = 5
)

// boxHint and boxTail are what the format's box shows right of the format:
// the hint of the mode, then the preset and "tab ›".
func (m tuiModel) boxHint() string {
	if m.uiState() == stateEdit {
		return "editing  "
	}
	return "i edit  "
}

func (m tuiModel) boxTail() string {
	return m.presetLabel() + "  tab ›"
}

// mouse handles a click or a turn of the wheel.
func (m tuiModel) mouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if !m.mouseOn() {
		return m, nil
	}
	mouse := msg.Mouse()
	x, y := mouse.X, mouse.Y
	m.notice = ""

	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.sel = max(0, m.sel-1)
		case tea.MouseWheelDown:
			m.sel = min(len(m.items())-1, m.sel+1)
		}
		return m, nil

	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return m, nil
		}
		switch {
		case y == rowHeader && m.onViewLabel(x):
			m.toggleView()
		case y == rowFormat:
			m.clickFormat(x)
		case y >= listTop && y < listTop+m.listHeight():
			win := m.window()
			i := y - listTop - btoi(win.up)
			switch {
			case i < 0:
				m.page(-1) // the line of those above
			case i >= win.rows:
				m.page(1) // the line of those below
			default:
				m.clickList(win.top + i)
			}
		}
	}
	return m, nil
}

// onViewLabel reports whether column x of the header is on the view's
// name, drawn last on the header and ending a column before its edge.
// While EXIF is read, the progress is there instead.
func (m tuiModel) onViewLabel(x int) bool {
	if m.nRead < len(m.rows) {
		return false
	}
	label := "view: " + m.view.String()
	start := m.width - 1 - ansi.StringWidth(label)
	return x >= start && x < m.width-1
}

// clickFormat handles a click on the box's line with the format: on the
// preset (and "tab ›"), the next preset; on the hint or the format, edit it,
// the cursor where the click was.
func (m *tuiModel) clickFormat(x int) {
	inner := m.width - 2
	tailStart := 1 + inner - 1 - ansi.StringWidth(m.boxTail())
	hintStart := tailStart - ansi.StringWidth(m.boxHint())
	switch {
	case x >= tailStart && x < 1+inner:
		m.cyclePreset(true)
	case x >= hintStart && x < tailStart:
		m.insert = true
	case x >= 1 && x < hintStart:
		// the format starts after the border, a space and the base
		start := 2 + ansi.StringWidth(m.base())
		m.insert = true
		m.cursor = max(0, min(x-start, len([]rune(m.format))))
	}
}

// clickList puts the cursor on line i of the list or the tree. A second
// click on a directory folds or opens it, and on "⋯ N more files" shows
// them. Editing the format ends.
func (m *tuiModel) clickList(i int) {
	items := m.items()
	if i < 0 || i >= len(items) {
		return
	}
	m.insert = false
	if i != m.sel {
		m.sel = i
		return
	}
	it := items[i]
	switch it.kind {
	case itemRoot:
		if m.folded[it.path] {
			clear(m.folded)
		} else {
			m.folded[it.path] = true
		}
	case itemDir:
		if m.folded[it.path] {
			delete(m.folded, it.path)
		} else {
			m.folded[it.path] = true
		}
	case itemMore:
		m.expanded[it.path] = true
	}
}
