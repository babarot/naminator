package main

import (
	"time"

	"github.com/babarot/naminator/internal/photo"

	tea "charm.land/bubbletea/v2"
)

// sender takes the messages of the work: a program, or the plain display.
type sender interface {
	Send(msg tea.Msg)
}

// Tracker tells a display what the work does: the work calls its methods
// and knows nothing of the messages they send.
type Tracker struct {
	to sender
}

// ReadStart tells that a worker began reading the EXIF data of image.
func (t Tracker) ReadStart(image string) {
	t.to.Send(exifStartMsg{image: image})
}

// ReadDone tells that the EXIF data of image was read, or why not.
func (t Tracker) ReadDone(image string, p photo.Photo, took time.Duration, err error) {
	t.to.Send(exifResultMsg{image: image, photo: p, duration: took, err: err})
}

// RenameStart starts the phase that renames photos (see RenamePhase).
func (t Tracker) RenameStart(photos []photo.Photo, base string, into, dry bool) {
	t.to.Send(RenamePhase(photos, base, into, dry).msg)
}

// Renamed tells that p was renamed (would be, when dry), or why not.
func (t Tracker) Renamed(p photo.Photo, dry bool, err error) {
	t.to.Send(renameResultMsg{photo: p, dryrun: dry, err: err})
}

// Warn tells of something left undone, reported at the end.
func (t Tracker) Warn(name, text string) {
	t.to.Send(warnMsg{name: name, text: text})
}

// CleanStart starts the phase that removes the empty directories.
func (t Tracker) CleanStart(dry bool) {
	t.to.Send(phaseStartMsg{kind: phaseClean, dry: dry})
}

// Cleaned tells that dir was removed (would be, when dry) as it was empty,
// or kept, or why it could not be.
func (t Tracker) Cleaned(dir string, dry, empty bool, err error) {
	t.to.Send(cleanResultMsg{dir: dir, dryrun: dry, empty: empty, err: err})
}

// Finish tells that the work is done: the display ends.
func (t Tracker) Finish() {
	t.to.Send(finishMsg{})
}
