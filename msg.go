package main

import (
	"fmt"
	"os"
	"strings"
)

type finishMsg struct{}

type resultMsg interface {
	String() string
	Err() error
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

func (r cleanResultMsg) Err() error { return r.err }

func (r cleanResultMsg) String() string {
	if r.err != nil {
		return fmt.Sprintf("%s: %s %v",
			r.dir,
			errorStyle.Render("FAILED"),
			r.err.Error())
	}
	if r.dryrun {
		return fmt.Sprintf("%s: %s Would remove if empty",
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

func (r renameResultMsg) Err() error { return r.err }

func (r renameResultMsg) String() string {
	if r.err != nil {
		return fmt.Sprintf("%s: %s %v",
			r.photo.Name,
			errorStyle.Render("FAILED"),
			r.err.Error())
	}
	renamedPath := strings.ReplaceAll(r.photo.RenamedPath, os.Getenv("HOME"), "~")
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
