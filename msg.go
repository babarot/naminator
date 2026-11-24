package main

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/babarot/naminator/internal/photo"
)

// The work reports to the display (Tracker) with these messages. A run is
// made of phases: reading EXIF, renaming, and removing empty directories
// (--clean). phaseStartMsg starts each one but the first, which the display
// is made with; finishMsg ends the run.

type finishMsg struct{}

type phaseKind int

const (
	phaseExif phaseKind = iota
	phaseRename
	phaseClean
)

type phaseStartMsg struct {
	kind phaseKind
	// keys are the paths the phase works on, in order; each row is named
	// after the base name of its key. Clean has none: its rows are added as
	// the directories are found.
	keys []string
	// limit is how many exiftool processes run at once, for the gauge.
	limit int
	// base is where the rename targets are shown from: the destination
	// directory when into is true (--dest-dir), else the only directory
	// given. Empty when there is none.
	base string
	into bool
	dry  bool
}

// exifStartMsg reports that a worker began reading the EXIF data of image.
type exifStartMsg struct {
	image string
}

type exifResultMsg struct {
	// image is the path the photo was read from; photo.Path is the one
	// exiftool reports, which can be empty on an error.
	image    string
	photo    photo.Photo
	duration time.Duration
	err      error
}

type renameResultMsg struct {
	photo  photo.Photo
	dryrun bool
	err    error
}

// warnMsg is something left undone, reported with the failures at the end.
type warnMsg struct {
	name string
	text string
}

type cleanResultMsg struct {
	dir    string
	dryrun bool
	empty  bool
	err    error
}

// abbrevHome replaces home at the start of path with "~".
func abbrevHome(path, home string) string {
	if home == "" {
		return path
	}
	home = filepath.Clean(home)
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return filepath.Join("~", rest)
	}
	return path
}
