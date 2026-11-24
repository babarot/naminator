package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

// config is the configuration file, ~/.config/naminator/config.yaml
// ($XDG_CONFIG_HOME/naminator/config.yaml when it is set):
//
//	format: "{ext}/{date}/{date}_{time}"
//	clean: true
//	presets:
//	  - name: for the blog
//	    format: "blog/{date}/{date}_{orig}"
type config struct {
	// Format is the template used when none is given (--format, --preset)
	Format string `yaml:"format"`
	// Clean removes the directories left empty, as -c does
	Clean bool `yaml:"clean"`
	// Presets are added to the built-in ones; one with the name of a
	// built-in preset replaces it
	Presets []configPreset `yaml:"presets"`
}

type configPreset struct {
	Name   string `yaml:"name"`
	Format string `yaml:"format"`
}

// configPath returns where the configuration file is.
func configPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "naminator", "config.yaml"), nil
}

// loadConfig reads the configuration file at path. A missing file is an
// empty configuration.
func loadConfig(path string) (config, error) {
	var c config
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := yaml.UnmarshalWithOptions(b, &c, yaml.Strict()); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	if c.Format != "" {
		if _, err := parseTemplate(c.Format); err != nil {
			return c, fmt.Errorf("%s: format: %w", path, err)
		}
	}
	for i, p := range c.Presets {
		if p.Name == "" {
			return c, fmt.Errorf("%s: presets[%d]: the name is empty", path, i)
		}
		if _, err := parseTemplate(p.Format); err != nil {
			return c, fmt.Errorf("%s: presets[%d] (%s): %w", path, i, p.Name, err)
		}
	}
	return c, nil
}

// allPresets returns the built-in presets, then those of the configuration:
// one with the name of a built-in preset takes its place.
func (c config) allPresets() []preset {
	all := make([]preset, len(presets))
	for i, p := range presets {
		all[i] = preset{name: p.name, format: p.format}
	}
next:
	for _, p := range c.Presets {
		for i := range all {
			if all[i].name == p.Name {
				all[i].format = p.Format
				continue next
			}
		}
		all = append(all, preset{name: p.Name, format: p.Format})
	}
	return all
}

// findPreset returns the format of the preset named name.
func (c config) findPreset(name string) (string, error) {
	all := c.allPresets()
	names := make([]string, len(all))
	for i, p := range all {
		if p.name == name {
			return p.format, nil
		}
		names[i] = fmt.Sprintf("%q", p.name)
	}
	return "", fmt.Errorf("no preset named %q: use one of %s", name, strings.Join(names, ", "))
}

// legacy returns the flags -t, -e, -E and -d.
func (o Option) legacy() legacyFlags {
	return legacyFlags{byDate: o.GroupByDate, byExt: o.GroupByExt, extFirst: o.GroupExtFirst, destDir: o.DestDir}
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
// (legacyNamer), -d included.
func chooseNamer(opt Option, cfg config, args []string) (*namer, string, error) {
	legacy := opt.GroupByDate || opt.GroupByExt || opt.GroupExtFirst
	if legacy && (opt.Format != "" || opt.Preset != "") {
		return nil, "", errors.New("use either -t, -e and -E, or --format and --preset")
	}
	if opt.Format != "" && opt.Preset != "" {
		return nil, "", errors.New("use either --format or --preset")
	}
	if legacy {
		name := legacyPreset(opt)
		return legacyNamer(opt.legacy()), fmt.Sprintf("-t, -e and -E are going away: use --preset %q instead", name), nil
	}
	format := opt.Format
	if opt.Preset != "" {
		f, err := cfg.findPreset(opt.Preset)
		if err != nil {
			return nil, "", err
		}
		format = f
	}
	if format == "" {
		format = cfg.Format
	}
	if format == "" {
		return legacyNamer(opt.legacy()), "", nil
	}
	tpl, err := parseTemplate(format)
	if err != nil {
		return nil, "", fmt.Errorf("--format: %w", err)
	}
	return newNamer(tpl, args, opt.DestDir), "", nil
}

// saveFormat sets format in the configuration file at path: it replaces
// the line of format, or adds one at the top, and keeps the rest, comments
// included.
func saveFormat(path, format string) error {
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	line := "format: " + strconv.Quote(format)
	lines := strings.Split(string(b), "\n")
	done := false
	for i, l := range lines {
		if strings.HasPrefix(l, "format:") {
			lines[i], done = line, true
			break
		}
	}
	if !done {
		if len(b) == 0 {
			lines = []string{line, ""}
		} else {
			lines = append([]string{line}, lines...)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}
