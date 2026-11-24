package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/babarot/naminator/internal/config"
	"github.com/babarot/naminator/internal/rename"
	"github.com/babarot/naminator/internal/ui"
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

// runInteractive runs the interactive screen while the EXIF data is read.
// Once confirmed, the screen ends, and the photos are renamed as decided
// (runDecided).
func (c CLI) runInteractive(screen *ui.Screen) error {
	c.tracker = screen
	// The screen ends once decided, and the EXIF data is all read before
	// it can be confirmed: the work is the reading only.
	d, err := screen.Do(func() { c.extractAll() })
	if err != nil {
		return err
	}
	switch {
	case d.Result == ui.Quit:
		// q also cancels, to stop the reading: quitting is not canceling
		return nil
	case d.Result != ui.Confirmed || c.isCanceled():
		return errCanceled
	}
	return c.runDecided(d)
}

// runDecided renames the photos as the screen decided, with the display of
// a run in the shell's scrollback.
func (c CLI) runDecided(d ui.Decision) error {
	tpl, err := rename.ParseTemplate(d.Format)
	if err != nil {
		return err
	}
	c.namer = rename.NewNamer(tpl, c.args, c.opt.DestDir)
	// the photos go where the screen showed them going
	c.planned = d.Targets
	c.logger.Debug("format chosen", "format", d.Format)

	fmt.Println(ui.FormatLine(d))
	display := ui.NewRun(ui.RenamePhase(d.Photos, c.base, c.into, c.opt.Dryrun), c.cancel)
	c.tracker = display
	return c.runWork(display, func() {
		// The photos the screen could not read are reported at the end
		for _, f := range d.Failed {
			c.tracker.Warn(f.Name, f.Err)
		}
		c.renameAll(d.Photos, false)
		c.finish()
	})
}
