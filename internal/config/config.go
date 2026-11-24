// Package config reads and writes naminator's configuration file.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/babarot/naminator/internal/rename"
)

// Config is the configuration file, ~/.config/naminator/config.yaml
// ($XDG_CONFIG_HOME/naminator/config.yaml when it is set):
//
//	format: "{ext}/{date}/{date}_{time}"
//	clean: true
//	presets:
//	  - name: for the blog
//	    format: "blog/{date}/{date}_{orig}"
type Config struct {
	// Format is the template used when none is given (--format, --preset)
	Format string `yaml:"format"`
	// Clean removes the directories left empty, as -c does
	Clean bool `yaml:"clean"`
	// Presets are added to the built-in ones; one with the name of a
	// built-in preset replaces it
	Presets []Preset `yaml:"presets"`
}

type Preset struct {
	Name   string `yaml:"name"`
	Format string `yaml:"format"`
}

// Path returns where the configuration file is.
func Path() (string, error) {
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

// Load reads the configuration file at path. A missing file is an
// empty configuration.
func Load(path string) (Config, error) {
	var c Config
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
		if _, err := rename.ParseTemplate(c.Format); err != nil {
			return c, fmt.Errorf("%s: format: %w", path, err)
		}
	}
	for i, p := range c.Presets {
		if p.Name == "" {
			return c, fmt.Errorf("%s: presets[%d]: the name is empty", path, i)
		}
		if _, err := rename.ParseTemplate(p.Format); err != nil {
			return c, fmt.Errorf("%s: presets[%d] (%s): %w", path, i, p.Name, err)
		}
	}
	return c, nil
}

// AllPresets returns the built-in presets, then those of the configuration:
// one with the name of a built-in preset takes its place.
func (c Config) AllPresets() []rename.Preset {
	all := make([]rename.Preset, len(rename.Presets))
	for i, p := range rename.Presets {
		all[i] = rename.Preset{Name: p.Name, Format: p.Format}
	}
next:
	for _, p := range c.Presets {
		for i := range all {
			if all[i].Name == p.Name {
				all[i].Format = p.Format
				continue next
			}
		}
		all = append(all, rename.Preset{Name: p.Name, Format: p.Format})
	}
	return all
}

// FindPreset returns the format of the preset named name.
func (c Config) FindPreset(name string) (string, error) {
	all := c.AllPresets()
	names := make([]string, len(all))
	for i, p := range all {
		if p.Name == name {
			return p.Format, nil
		}
		names[i] = fmt.Sprintf("%q", p.Name)
	}
	return "", fmt.Errorf("no preset named %q: use one of %s", name, strings.Join(names, ", "))
}

// SaveFormat sets format in the configuration file at path: it replaces
// the line of format, or adds one at the top, and keeps the rest, comments
// included.
func SaveFormat(path, format string) error {
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
