package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

const maxHeight = 30

var (
	showProgressSet bool
	showProgress    bool
)

type model struct {
	progress     progress.Model
	spinner      spinner.Model
	quitting     bool
	results      []resultMsg
	errorResults []resultMsg
	startTime    time.Time
	processed    int
	total        int
	height       int
}

func newModel(total int) model {
	s := spinner.New(
		spinner.WithSpinner(spinner.Line),
		spinner.WithStyle(spinnerStyle),
	)
	_, termHeight, _ := term.GetSize(int(os.Stdout.Fd()))
	minHeight := termHeight - 9
	if minHeight < 0 {
		minHeight = 1
	}
	height := min(total, minHeight, maxHeight)
	return model{
		progress:     progress.New(progress.WithDefaultGradient()),
		spinner:      s,
		quitting:     false,
		results:      make([]resultMsg, height),
		errorResults: nil,
		startTime:    time.Now(),
		processed:    0,
		total:        total,
		height:       height,
	}
}

func (m model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case finishMsg:
		m.quitting = true
		return m, tea.Quit
	case resultMsg:
		if msg.Err() != nil {
			// If the message contains an error, add it to the errorResults slice.
			m.errorResults = append(m.errorResults, msg)
		}
		if len(m.errorResults) > 0 {
			// If there are any recorded errors, rebuild the results slice.
			// The goal is to highlight error messages (retain them in the slice)
			// while receiving other new messages.
			var met bool
			var results []resultMsg
			for _, result := range m.results {
				if (result == nil || result.Err() == nil) && !met {
					// Skip the first non-error message encountered.
					// This ensures that when an error occurs, a regular message is removed
					// to make space while keeping error messages visible.
					met = true
				} else {
					// Retain all other messages.
					results = append(results, result)
				}
			}
			if n := len(results); n >= m.height {
				// If the number of messages exceeds the allowed height, trim the oldest ones.
				// This prevents the list from growing indefinitely while keeping recent messages.
				m.results = append(results[n-m.height+1:], msg)
			} else {
				// Otherwise, simply append the new message.
				m.results = append(results, msg)
			}
		} else {
			// If there are no errors, maintain a fixed-length message history.
			// Remove the oldest message (first element) and append the new message.
			// This ensures the message list remains within the allowed height.
			m.results = append(m.results[1:], msg)
		}
		if _, ok := msg.(exifResultMsg); ok {
			m.processed++
		}
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m model) View() string {
	var s string

	if m.quitting {
		s += "Renaming completed. Total elapsed time: " +
			duration(time.Since(m.startTime)).String()
	} else {
		s += m.spinner.View() + " Processing photos..."
	}
	s += fmt.Sprintf(" (%d/%d)", m.processed, m.total)

	s += "\n\n"

	for _, res := range m.results {
		switch res.(type) {
		case exifResultMsg, renameResultMsg, cleanResultMsg:
			s += res.String()
		default:
			// default is resultMsg (interface)
			s += dotStyle.Render(strings.Repeat(".", 30))
		}
		s += "\n"
	}

	s += "\n"

	percent := float64(m.processed) / float64(m.total)
	if time.Since(m.startTime).Seconds() > 3.0 && !showProgressSet {
		showProgress = percent < 0.25
		showProgressSet = true
	}
	if percent < 1 && (m.total > 100 || showProgress) {
		s += m.progress.ViewAs(percent)
		s += "\n"
	}

	return appStyle.Render(s)
}
