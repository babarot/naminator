package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type finishMsg struct{}

type resultMsg interface {
	Path() string
	Err() error
	String() string
}

type exifResultMsg struct {
	photo    Photo
	duration duration
	err      error
}

type renameResultMsg struct {
	photo  Photo
	dryrun bool
	err    error
}

type cleanResultMsg struct {
	dir    string
	dryrun bool
	empty  bool
	err    error
}

func (r cleanResultMsg) Path() string { return r.dir }

func (r cleanResultMsg) Err() error { return r.err }

func (r cleanResultMsg) String() string {
	if r.err != nil {
		return fmt.Sprintf("%s: %s %v",
			r.dir,
			errorStyle.Render("FAILED"),
			r.err.Error())
	}
	if r.dryrun {
		if r.empty {
			return fmt.Sprintf("%s: %s Would remove because empty",
				r.dir,
				dryrunStyle.Render("DRY-RUN"))
		}
		return fmt.Sprintf("%s: %s Would not remove because NOT empty",
			r.dir,
			dryrunStyle.Render("DRY-RUN"))
	}
	if r.empty {
		return fmt.Sprintf("%s: %s Removed because empty",
			r.dir,
			okStyle.Render("OK"))
	} else {
		return fmt.Sprintf("%s: %s Do not remove because NOT empty",
			r.dir,
			warnStyle.Render("SKIP"))
	}
}

func (r renameResultMsg) Path() string { return r.photo.Path }

func (r renameResultMsg) Err() error { return r.err }

func (r renameResultMsg) String() string {
	if r.err != nil {
		return fmt.Sprintf("%s: %s %v",
			r.photo.Name,
			errorStyle.Render("FAILED"),
			r.err.Error())
	}
	renamedPath := abbrevHome(r.photo.RenamedPath, os.Getenv("HOME"))
	if r.dryrun {
		return fmt.Sprintf("%s: %s Would rename %s",
			r.photo.Name,
			dryrunStyle.Render("DRY-RUN"),
			dryrunStyle.Render("-> "+renamedPath),
		)
	}
	return fmt.Sprintf("%s: %s Renamed to %s",
		r.photo.Name,
		okStyle.Render("OK"),
		renamedPath)
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

func (r exifResultMsg) Path() string { return r.photo.Path }

func (r exifResultMsg) Err() error { return r.err }

func (r exifResultMsg) String() string {
	if r.err != nil {
		return fmt.Sprintf("%s: %s %v",
			r.photo.Name,
			errorStyle.Render("FAILED"),
			r.err.Error())
	}
	return fmt.Sprintf("%s: %s Got exif data %s",
		r.photo.Name,
		okStyle.Render("OK"),
		durationStyle.Render(r.duration.String()))
}
