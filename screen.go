package main

import (
	"os"

	"github.com/babarot/naminator/internal/rename"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ScreenOptions are what the interactive screen is made with.
type ScreenOptions struct {
	// Args are the arguments, and Images the photos found in them.
	Args   []string
	Images []string
	// Format is the format the screen starts with, under DestDir when it
	// is set.
	Format  string
	DestDir string
	Dry     bool
	Clean   bool
	Presets []rename.Preset
	FS      rename.FileSystem
	// ConfigPath is where the format is saved as the default.
	ConfigPath string
	// Cancel stops the reading, when the screen ends without confirming.
	Cancel func()
}

// Screen is the interactive screen: it shows the path each photo would get
// as the work reads their EXIF data (Tracker), and ends once decided.
type Screen struct {
	Tracker
	p      *tea.Program
	cancel func()
}

// NewScreen returns the interactive screen, which is drawn on the alternate
// screen.
func NewScreen(o ScreenOptions) *Screen {
	m := newTUIModel(o.Args, o.Images, o.Format, o.DestDir, o.Dry, o.Clean, o.Presets, o.FS, o.ConfigPath, o.Cancel)
	// the color of the cursor's line suits the terminal's background
	m.dark = lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	p := tea.NewProgram(m, tea.WithoutSignalHandler())
	return &Screen{Tracker: Tracker{p}, p: p, cancel: o.Cancel}
}

// Do runs the screen while work runs, and returns what was decided.
func (s *Screen) Do(work func()) (tuiDecision, error) {
	final, err := runProgram(s.p, s.cancel, work)
	if err != nil {
		return tuiDecision{}, err
	}
	m, _ := final.(tuiModel)
	return m.decision(), nil
}

// FormatLine is the line that tells the format decided, above the run.
func FormatLine(d tuiDecision) string {
	return Dim.Render("Format: ") + Blue.Render(d.Base) + d.Format
}
