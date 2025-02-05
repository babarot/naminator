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
	files        map[string]state
	total        int
	height       int
}

type state uint8

const (
	succeeded state = iota
	failed
)

type status struct {
	succeeded int
	failed    int
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
		files:        map[string]state{},
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
		switch v := msg.(type) {
		case exifResultMsg, renameResultMsg:
			if v.Err() != nil {
				m.files[msg.Path()] = failed
			} else {
				m.files[msg.Path()] = succeeded
			}
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

	var successes, failures int
	for _, state := range m.files {
		switch state {
		case succeeded:
			successes++
		case failed:
			failures++
		}
	}

	if m.quitting {
		s += "Renaming done. Time: " +
			duration(time.Since(m.startTime)).String()
		s += fmt.Sprintf(" (%d OK, %d failed, %d total)", successes, failures, m.total)
		if failures > 0 {
			s += "\n"
			s += fmt.Sprintf("%d %s detected. See %s for more details.",
				failures,
				map[bool]string{true: "issue", false: "issues"}[failures == 1],
				underStyle.Render("debug.log"))
		}
	} else {
		s += m.spinner.View() + " Processing photos... "
		s += fmt.Sprintf("(%d/%d)", successes+failures, m.total)
	}

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

	percent := float64(successes+failures) / float64(m.total)
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
