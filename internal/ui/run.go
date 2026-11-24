// Package ui is what naminator draws: the interactive screen (Screen) and
// the display of a run (Run). The work tells them what it does (Tracker);
// they move nothing.
package ui

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/babarot/naminator/internal/photo"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-isatty"
)

// Phase is the part of a run a display starts with.
type Phase struct {
	msg phaseStartMsg
}

// ReadPhase is reading the EXIF data of images, limit at once.
func ReadPhase(images []string, limit int) Phase {
	return Phase{phaseStartMsg{kind: phaseExif, keys: images, limit: limit}}
}

// RenamePhase is renaming photos. base is where the paths they get are
// shown from: the destination directory when into is true (--dest-dir),
// else the only directory given. Empty when there is none.
func RenamePhase(photos []photo.Photo, base string, into, dry bool) Phase {
	return Phase{phaseStartMsg{kind: phaseRename, keys: photo.Paths(photos), base: base, into: into, dry: dry}}
}

// Run is the display of a run, which the work reports to (Tracker).
type Run struct {
	Tracker
	p      *tea.Program // nil without a terminal
	cancel func()
}

// NewPlainRun returns the display of a run without a terminal: plain lines
// written to w.
func NewPlainRun(w io.Writer, first Phase) *Run {
	return &Run{Tracker: Tracker{newPlainSender(w, first.msg)}}
}

// NewRun returns the display of a run in a terminal, drawn below the
// prompt. cancel is called on Ctrl+C or a signal to stop the work.
func NewRun(first Phase, cancel func()) *Run {
	m := newModel(first.msg, cancel)
	m.awaitReply = true
	if title := m.startCompact(); title != "" {
		fmt.Println(title)
	}
	p := tea.NewProgram(m, tea.WithoutSignalHandler())
	return &Run{Tracker: Tracker{p}, p: p, cancel: cancel}
}

// Do runs work with the display, and returns once both have ended: the
// display ends once work has called Finish and everything is printed.
func (r *Run) Do(work func()) error {
	if r.p == nil {
		work()
		return nil
	}
	_, err := runProgram(r.p, r.cancel, work)
	return err
}

// runProgram runs p while work runs, and returns p's final model once both
// have ended. A signal reaches p as interruptMsg, as Ctrl+C does, and
// calls cancel; so does a failure of p, to stop the work.
func runProgram(p *tea.Program, cancel func(), work func()) (tea.Model, error) {
	// A signal stops the run as Ctrl+C does, from before the work starts
	// until this returns: the work can end long before p, as the reading
	// under the screen does. It also cancels the run, so that a signal as
	// p ends starts no next program. After the first, the default handling
	// is back, so a second one ends the process at once.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	ran := make(chan struct{})
	forwarded := make(chan struct{})
	go func() {
		defer close(forwarded)
		select {
		case <-sig:
			signal.Stop(sig)
			cancel()
			p.Send(interruptMsg{})
		case <-ran:
		}
	}()
	defer func() {
		close(ran)
		<-forwarded
		// a signal that came as p ended, before it was forwarded
		select {
		case <-sig:
			cancel()
		default:
		}
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		work()
	}()
	final, err := p.Run()
	if err != nil {
		// The program failed: stop the work too.
		cancel()
	}
	// Wait for the work, so that no rename starts after this and the
	// exiftool processes are closed only once nothing uses them. Sends to
	// the ended program return at once.
	<-done
	// FIXME(bubbletea): drain the terminal's replies to the queries
	// bubbletea v2 sends, which it fails to read during shutdown; they
	// would leak into the shell. Only a terminal replies: draining
	// anything else (</dev/null, a pipe) would wait forever on Linux.
	if isatty.IsTerminal(os.Stdin.Fd()) {
		drainStdinAfterBubbletea()
	}
	return final, err
}
