package cli

import (
	"errors"
	"fmt"

	"github.com/babarot/naminator/internal/config"
	"github.com/babarot/naminator/internal/rename"
)

// legacy returns the flags -t, -e, -E and -d.
func (o Option) legacy() rename.LegacyFlags {
	return rename.LegacyFlags{ByDate: o.GroupByDate, ByExt: o.GroupByExt, ExtFirst: o.GroupExtFirst, DestDir: o.DestDir}
}

// legacyPreset names the preset that the flags -t, -e and -E stand for.
func legacyPreset(opt Option) string {
	switch {
	case opt.GroupByDate && opt.GroupByExt && opt.GroupExtFirst:
		return "by extension, then date"
	case opt.GroupByDate && opt.GroupByExt:
		return "by date, then extension"
	case opt.GroupByDate:
		return "by date"
	case opt.GroupByExt:
		return "by extension"
	}
	return ""
}

// chooseNamer returns the namer for the run, and a note to print when the
// run uses the flags that are going away. The template is the first of
// --format, --preset and the configuration's format. Without one, or with
// -t, -e or -E, photos are named by the flags as they always have been
// (rename.NewLegacyNamer), -d included.
func chooseNamer(opt Option, cfg config.Config, args []string) (*rename.Namer, string, error) {
	legacy := opt.GroupByDate || opt.GroupByExt || opt.GroupExtFirst
	if legacy && (opt.Format != "" || opt.Preset != "") {
		return nil, "", errors.New("use either -t, -e and -E, or --format and --preset")
	}
	if opt.Format != "" && opt.Preset != "" {
		return nil, "", errors.New("use either --format or --preset")
	}
	if legacy {
		name := legacyPreset(opt)
		return rename.NewLegacyNamer(opt.legacy()), fmt.Sprintf("-t, -e and -E are going away: use --preset %q instead", name), nil
	}
	format := opt.Format
	if opt.Preset != "" {
		f, err := cfg.FindPreset(opt.Preset)
		if err != nil {
			return nil, "", err
		}
		format = f
	}
	if format == "" {
		format = cfg.Format
	}
	if format == "" {
		return rename.NewLegacyNamer(opt.legacy()), "", nil
	}
	tpl, err := rename.ParseTemplate(format)
	if err != nil {
		return nil, "", fmt.Errorf("--format: %w", err)
	}
	return rename.NewNamer(tpl, args, opt.DestDir), "", nil
}
