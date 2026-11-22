package main

import (
	"errors"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

// checkInteractive reports flags that cannot be used with -i.
func checkInteractive(opt Option) error {
	if opt.GroupByDate || opt.GroupByExt || opt.GroupExtFirst {
		return errors.New("-i cannot be used with -t, -e and -E: choose a preset in it, or give --preset")
	}
	if opt.Format != "" && opt.Preset != "" {
		return errors.New("use either --format or --preset")
	}
	return nil
}

// startFormat is the format the interactive screen starts with: --format,
// --preset, or, when only files are given, the date and time in place (they
// are renamed where they are, as naminator IMG_*.HEIC is meant to); else
// the configuration's format, or the date and time in place.
func startFormat(opt Option, cfg config, args []string) (string, error) {
	switch {
	case opt.Format != "":
		if _, err := parseTemplate(opt.Format); err != nil {
			return "", fmt.Errorf("--format: %w", err)
		}
		return opt.Format, nil
	case opt.Preset != "":
		return cfg.findPreset(opt.Preset)
	}
	inPlace := presets[0].format
	if onlyFiles(args) || cfg.Format == "" {
		return inPlace, nil
	}
	return cfg.Format, nil
}

// onlyFiles reports whether every argument is a file, not a directory.
func onlyFiles(args []string) bool {
	for _, a := range args {
		if fi, err := os.Stat(a); err != nil || fi.IsDir() {
			return false
		}
	}
	return len(args) > 0
}

// runInteractive runs the interactive screen while the EXIF data is read.
// Once confirmed, the photos are renamed with the format chosen, and the
// screen gives way to the usual display from the rename phase, in the same
// program, so that what it prints stays in order.
func (c CLI) runInteractive(tui tuiModel) error {
	decided := make(chan tuiDecision, 1)
	tui.decided = decided
	tui.runBase, tui.runInto = c.base, c.into
	p := tea.NewProgram(tui, tea.WithoutSignalHandler())
	c.p, c.sender = p, p
	final, err := c.runProgram(p, func() {
		photos := c.extractAll()
		// Stop waiting when the program ends without deciding (it failed),
		// or is canceled just as the display takes over: then the display
		// still needs finishMsg{} to end. Sends to an ended program return
		// at once.
		var d tuiDecision
		select {
		case d = <-decided:
		case <-c.canceled:
			c.sender.Send(finishMsg{})
			return
		}
		if d.result != tuiConfirmed {
			return
		}
		tpl, err := parseTemplate(d.format)
		if err != nil {
			c.sender.Send(finishMsg{})
			return
		}
		c.namer = newNamer(tpl, c.args, c.opt.DestDir)
		c.logger.Debug("format chosen", "format", d.format)
		// The photos the screen could not read are reported at the end
		for _, f := range d.failed {
			c.sender.Send(warnMsg{name: f.name, text: f.err})
		}
		c.renameAll(photos, false)
		c.finish()
	})
	if err != nil {
		return err
	}
	if m, ok := final.(tuiModel); ok && m.result == tuiQuit {
		return nil
	}
	if c.isCanceled() {
		return errCanceled
	}
	if m, ok := final.(tuiModel); ok && m.result != tuiConfirmed {
		return errCanceled
	}
	return nil
}
