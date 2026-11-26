package ui

import (
	"time"

	"github.com/babarot/naminator/internal/photo"
	"github.com/babarot/naminator/internal/rename"
	"github.com/babarot/naminator/internal/ui/theme"

	tea "charm.land/bubbletea/v2"
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
	p       *tea.Program
	results *gatherer
	cancel  func()
}

// ReadStart tells nothing: the screen shows which photos are read, not
// which are being read.
func (s *Screen) ReadStart(string) {}

// ReadDone tells that the EXIF data of image was read, or why not.
func (s *Screen) ReadDone(image string, p photo.Photo, took time.Duration, err error) {
	s.results.add(exifResultMsg{image: image, photo: p, duration: took, err: err})
}

// NewScreen returns the interactive screen, which is drawn on the alternate
// screen.
func NewScreen(o ScreenOptions) *Screen {
	m := newTUIModel(o.Args, o.Images, o.Format, o.DestDir, o.Dry, o.Clean, o.Presets, o.FS, o.ConfigPath, o.Cancel)
	p := tea.NewProgram(m, tea.WithoutSignalHandler())
	return &Screen{Tracker: Tracker{p}, p: p, cancel: o.Cancel}
}

// Do runs the screen while work runs, and returns what was decided.
func (s *Screen) Do(work func()) (Decision, error) {
	s.results = newGatherer(s.p, gatherEvery)
	final, err := runProgram(s.p, s.cancel, func() {
		work()
		s.results.close()
	})
	if err != nil {
		return Decision{}, err
	}
	m, _ := final.(tuiModel)
	return m.decision(), nil
}

// FormatLine is the line that tells the format decided, above the run.
func FormatLine(d Decision) string {
	st := theme.For(true)
	return st.Dim.Render("Format: ") + st.Path.Render(d.Base) + d.Format
}
