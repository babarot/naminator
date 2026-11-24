package main

import (
	"io"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// A signal stops the program for as long as it runs, also once the work
// has ended (as the reading under the screen does long before the screen
// is decided), and cancels the run.
func TestRunProgramSignalAfterWork(t *testing.T) {
	var canceled atomic.Bool
	p := tea.NewProgram(waitModel{}, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutSignalHandler())
	workDone := make(chan struct{})
	go func() {
		<-workDone
		time.Sleep(50 * time.Millisecond)
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
		// a signal that does not reach the program fails the test rather
		// than leaving it running
		time.AfterFunc(3*time.Second, p.Quit)
	}()
	final, err := runProgram(p, func() { canceled.Store(true) }, func() { close(workDone) })
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := final.(waitModel); !m.interrupted {
		t.Error("the program did not get the signal")
	}
	if !canceled.Load() {
		t.Error("the run was not canceled")
	}
}

// waitModel runs until a signal reaches it.
type waitModel struct{ interrupted bool }

func (m waitModel) Init() tea.Cmd { return nil }

func (m waitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(interruptMsg); ok {
		m.interrupted = true
		return m, tea.Quit
	}
	return m, nil
}

func (m waitModel) View() tea.View { return tea.NewView("") }
