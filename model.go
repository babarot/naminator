package main

import (
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"
)

// model draws the run in the terminal, after gh-infra's spinner display. A
// phase with few rows is drawn whole (fullView). One with many, or that does
// not fit, is compact: its title and finished rows are printed above the
// view with tea.Println, where they stay in the scrollback, and the view
// keeps a fixed height. A view taller than the terminal cannot be redrawn by
// the inline renderer: its top lines scroll away and leave broken copies.
//
// When a phase is over, what the view showed of it is printed above, and
// the next phase is drawn below it. At the end the failures are printed in
// full and the view is left empty.
type model struct {
	phase    *phase
	failures []failure
	spinner  spinner.Model
	width    int
	height   int // 0 when unknown
	queue    []string
	printing bool // a batch of queue is being printed
	finished bool // finishMsg came: the view is empty and quits once printed
	stopped  bool // Ctrl+C was pressed
	cancel   func()
	now      func() time.Time
}

// printedMsg reports that a batch of queued lines has been printed.
type printedMsg struct{}

// interruptMsg reports a SIGINT or SIGTERM from outside (kill, or ^C when
// the input is not a terminal); the model stops as on Ctrl+C. The program
// runs without bubbletea's signal handler, which would end it at once,
// before the model could show what was canceled.
type interruptMsg struct{}

// newModel returns the model showing first, the first phase. cancel is
// called on Ctrl+C to stop the work.
func newModel(first phaseStartMsg, cancel func()) model {
	width, height := 80, 0
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		width, height = w, h
	}
	m := model{
		spinner: spinner.New(
			spinner.WithSpinner(spinner.Jump),
			spinner.WithStyle(Cyan),
		),
		width:  width,
		height: height,
		cancel: cancel,
		now:    time.Now,
	}
	m.phase = newPhase(first, m.now())
	return m
}

// startCompact makes the first phase compact when it should be from the
// start, and returns its title to print before the program runs: a
// tea.Println sent before the first frame is drawn lands at the top of the
// screen, and that frame then erases what is below it.
func (m *model) startCompact() string {
	if !m.phase.wantCompact(m.height) {
		return ""
	}
	m.phase.compact = true
	return m.phase.title() + "\n"
}

func (m model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, m.settle()

	case phaseStartMsg:
		m.endPhase()
		m.phase = newPhase(msg, m.now())
		if m.phase.wantCompact(m.height) {
			m.phase.compact = true
			m.queue = append(m.queue, m.phase.title(), "")
		}
		return m, m.settle()

	case exifStartMsg, exifResultMsg, renameResultMsg, cleanResultMsg:
		if _, f := m.phase.apply(msg, m.now()); f != nil {
			m.failures = append(m.failures, *f)
		}
		return m, m.settle()

	case finishMsg:
		m.endPhase()
		m.queue = append(m.queue, errorReport(m.failures)...)
		m.finished = true
		return m, m.flush()

	case printedMsg:
		m.printing = false
		return m, m.flush()

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, m.stop()
		}
		return m, nil

	case interruptMsg:
		return m, m.stop()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

// stop shows the running rows as canceled and stops the work. The program
// ends once the work reports finishMsg and everything is printed.
func (m *model) stop() tea.Cmd {
	if m.stopped {
		return nil
	}
	m.stopped = true
	m.phase.cancel(m.now())
	if m.cancel != nil {
		m.cancel()
	}
	return m.settle()
}

// endPhase queues what the view showed of the phase, to keep it above.
func (m *model) endPhase() {
	p := m.phase
	if p.compact {
		m.queuePrinted()
		m.queue = append(m.queue, p.compactEnd()...)
	} else {
		m.queue = append(m.queue, p.fullView(true, "", m.now(), m.width)...)
	}
	m.queue = append(m.queue, "")
}

// queuePrinted queues the finished rows of a compact phase not printed yet,
// and the canceled ones once stopped.
func (m *model) queuePrinted() {
	p := m.phase
	for _, it := range p.finished {
		if !it.printed && shownAbove(it) {
			it.printed = true
			m.queue = append(m.queue, p.row(it, "", m.now(), m.width))
		}
	}
	if p.canceled {
		for _, it := range p.items {
			if it.status == itemCanceled && !it.printed {
				it.printed = true
				m.queue = append(m.queue, p.row(it, "", m.now(), m.width))
			}
		}
	}
}

// settle makes the phase compact when it no longer fits, queues what the
// compact view does not hold, and prints it.
func (m *model) settle() tea.Cmd {
	p := m.phase
	if !p.compact && p.wantCompact(m.height) {
		p.compact = true
		m.queue = append(m.queue, p.title(), "")
	}
	if p.compact {
		m.queuePrinted()
	}
	return m.flush()
}

// flush prints the queued lines, or quits once the run has finished and
// everything is printed.
//
// Commands returned by separate Updates run concurrently, so two batches of
// tea.Println could come out of order, and tea.Quit could overtake a batch
// and drop its lines. One batch is in flight at a time: a tea.Sequence that
// ends with printedMsg, which sends the next batch, or the quit.
func (m *model) flush() tea.Cmd {
	if m.printing {
		return nil
	}
	if len(m.queue) > 0 {
		// One Println for the batch: an empty line printed on its own is
		// dropped, but kept inside a text.
		text := strings.Join(m.queue, "\n")
		m.queue = nil
		m.printing = true
		return tea.Sequence(tea.Println(text), func() tea.Msg { return printedMsg{} })
	}
	if m.finished {
		return tea.Quit
	}
	return nil
}

func (m model) View() tea.View {
	if m.finished {
		return tea.NewView("")
	}
	p, spin, now := m.phase, m.spinner.View(), m.now()
	var lines []string
	if p.compact {
		lines = p.compactView(spin, now, m.width, m.height)
	} else {
		lines = p.fullView(false, spin, now, m.width)
	}
	return tea.NewView(strings.Join(lines, "\n"))
}
