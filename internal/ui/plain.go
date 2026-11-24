package ui

import (
	"fmt"
	"io"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

// plainSender shows the run without a terminal (a pipe, a file, CI): each
// phase's title, the rows with something to show as they finish, and the
// outcome, as plain lines without a spinner or a progress bar.
type plainSender struct {
	mu       sync.Mutex
	w        io.Writer
	phase    *phase
	failures []Failure
	now      func() time.Time
}

func newPlainSender(w io.Writer, first phaseStartMsg) *plainSender {
	s := &plainSender{w: w, now: time.Now}
	s.phase = newPhase(first, s.now())
	_, _ = fmt.Fprintln(w, s.phase.title())
	return s
}

func (s *plainSender) Send(msg tea.Msg) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch msg := msg.(type) {
	case phaseStartMsg:
		s.endPhase()
		s.phase = newPhase(msg, s.now())
		_, _ = fmt.Fprintln(s.w, s.phase.title())
	case finishMsg:
		s.endPhase()
		for _, line := range errorReport(s.failures) {
			_, _ = fmt.Fprintln(s.w, line)
		}
	default:
		it, f := s.phase.apply(msg, s.now())
		if f != nil {
			s.failures = append(s.failures, *f)
		}
		if it != nil && shownAbove(it) {
			_, _ = fmt.Fprintln(s.w, s.phase.row(it, "", s.now(), 0))
		}
	}
}

func (s *plainSender) endPhase() {
	_, _ = fmt.Fprintln(s.w, s.phase.outcome())
	_, _ = fmt.Fprintln(s.w)
}
