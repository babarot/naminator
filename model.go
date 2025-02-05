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

// replaceFirstNilOrAppend finds the first nil element in m.results and replaces it with msg.
// If no nil element is found, it removes the oldest element and appends msg at the end.
// This ensures that messages fit within the allocated space while keeping recent ones visible.
func (m *model) replaceFirstNilOrAppend(msg resultMsg) {
	found := false
	for i, result := range m.results {
		if result == nil {
			m.results[i] = msg
			found = true
			break
		}
	}
	// If no nil element was found, remove the oldest message and append the new one.
	if !found {
		m.results = append(m.results[1:], msg)
	}
}

// trimNonErrorMessageAndAppend removes the first non-error message (or nil) from m.results
// while ensuring error messages are retained. If all messages contain errors, it removes
// the oldest one instead. This function maintains a balance between highlighting errors
// and allowing new messages to be displayed.
func (m *model) trimNonErrorMessageAndAppend(msg resultMsg) {
	var newResults []resultMsg
	met := false

	// Iterate through m.results and remove the first non-error message (or nil).
	for _, result := range m.results {
		if !met && (result == nil || result.Err() == nil) {
			// Skip the first encountered non-error message (or nil) to make space for new messages.
			met = true
			continue
		}
		newResults = append(newResults, result)
	}

	// Ensure the number of messages does not exceed the allowed height.
	if len(newResults) >= m.height {
		// If the list exceeds the allowed height, trim the oldest messages before appending the new one.
		newResults = append(newResults[len(newResults)-m.height+1:], msg)
	} else {
		// Otherwise, simply append the new message.
		newResults = append(newResults, msg)
	}
	m.results = newResults
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case finishMsg:
		m.quitting = true
		return m, tea.Quit

	case resultMsg:
		// If msg contains an error, add it to the errorResults list.
		if msg.Err() != nil {
			m.errorResults = append(m.errorResults, msg)
		}

		// If any errors exist, prioritize retaining error messages while managing space.
		if len(m.errorResults) > 0 {
			m.trimNonErrorMessageAndAppend(msg)
		} else {
			// Otherwise, maintain a fixed-length message history.
			m.replaceFirstNilOrAppend(msg)
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
		// if failures > 0 {
		// 	s += "\n"
		// 	s += fmt.Sprintf("%d %s detected. See %s for more details.",
		// 		failures,
		// 		map[bool]string{true: "issue", false: "issues"}[failures == 1],
		// 		underStyle.Render("debug.log"))
		// }
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
