package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/babarot/naminator/internal/config"
	"github.com/babarot/naminator/internal/photo"
	"github.com/babarot/naminator/internal/rename"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// interactive reports whether the run opens the interactive screen: in a
// terminal, unless --yes or the flags going away (-t, -e, -E) are given,
// which rename without asking as before; or when -i asks for it.
func interactive(opt Option, terminal bool) bool {
	if opt.Interactive {
		return true
	}
	legacy := opt.GroupByDate || opt.GroupByExt || opt.GroupExtFirst
	return terminal && !opt.Yes && !legacy
}

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
func startFormat(opt Option, cfg config.Config, args []string) (string, error) {
	switch {
	case opt.Format != "":
		if _, err := rename.ParseTemplate(opt.Format); err != nil {
			return "", fmt.Errorf("--format: %w", err)
		}
		return opt.Format, nil
	case opt.Preset != "":
		return cfg.FindPreset(opt.Preset)
	}
	inPlace := rename.Presets[0].Format
	switch {
	case onlyFiles(args):
		return inPlace, nil
	case cfg.Format != "":
		return cfg.Format, nil
	case opt.DestDir != "":
		// into the destination as they are, as -d alone has always done
		return "{date}_{time}", nil
	}
	return inPlace, nil
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

// runInteractive runs the interactive screen, on the alternate screen,
// while the EXIF data is read. Once confirmed, the screen ends, and the
// photos are renamed with the format chosen by runDecided, with the usual
// display from the rename phase in the shell's scrollback.
func (c CLI) runInteractive(tui tuiModel) error {
	// the color of the cursor's line suits the terminal's background
	tui.dark = lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	p := tea.NewProgram(tui, tea.WithoutSignalHandler())
	c.p, c.sender = p, p
	// The screen ends once decided, and the EXIF data is all read before
	// it can be confirmed: the work is the reading only.
	final, err := c.runProgram(p, func() { c.extractAll() })
	if err != nil {
		return err
	}
	m, _ := final.(tuiModel)
	switch {
	case m.result == tuiQuit:
		// q also cancels, to stop the reading: quitting is not canceling
		return nil
	case m.result != tuiConfirmed || c.isCanceled():
		return errCanceled
	}
	return c.runDecided(m)
}

// runDecided renames the photos as the screen m decided, with the run's
// display, a program of its own drawn below the shell's prompt.
func (c CLI) runDecided(m tuiModel) error {
	d := m.decision()
	tpl, err := rename.ParseTemplate(d.format)
	if err != nil {
		return err
	}
	c.namer = rename.NewNamer(tpl, c.args, c.opt.DestDir)
	// the photos go where the screen showed them going
	c.planned = d.targets
	c.logger.Debug("format chosen", "format", d.format)

	// The photos given the file's time are among those of the screen
	photos := m.photos()
	photo.SortByTime(photos)
	fmt.Println(Dim.Render("Format: ") + Blue.Render(m.base()) + d.format)
	run := newModel(phaseStartMsg{kind: phaseRename, keys: photo.Paths(photos), base: c.base, into: c.into, dry: c.opt.Dryrun}, c.cancel)
	if title := run.startCompact(); title != "" {
		fmt.Println(title)
	}
	// the photos were read on the screen: the run can end at once
	run.awaitReply = true
	c.p = tea.NewProgram(run, tea.WithoutSignalHandler())
	c.sender = c.p
	return c.runWork(func() {
		// The photos the screen could not read are reported at the end
		for _, f := range d.failed {
			c.sender.Send(warnMsg{name: f.name, text: f.err})
		}
		c.renameAll(photos, false)
		c.finish()
	})
}
