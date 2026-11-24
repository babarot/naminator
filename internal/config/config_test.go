package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/babarot/naminator/internal/rename"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfig(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		c, err := Load(filepath.Join(t.TempDir(), "none.yaml"))
		if err != nil || c.Format != "" || c.Clean || len(c.Presets) != 0 {
			t.Errorf("config = %+v, %v, want empty", c, err)
		}
	})
	t.Run("valid", func(t *testing.T) {
		c, err := Load(writeConfig(t, "format: \"{ext}/{date}/{date}_{time}\"\nclean: true\npresets:\n  - name: for the blog\n    format: \"blog/{date}/{date}_{orig}\"\n"))
		if err != nil {
			t.Fatal(err)
		}
		if c.Format != "{ext}/{date}/{date}_{time}" || !c.Clean || len(c.Presets) != 1 || c.Presets[0].Name != "for the blog" {
			t.Errorf("config = %+v", c)
		}
	})
	for name, tt := range map[string]struct{ body, err string }{
		"unknown key":     {"formt: \"{date}\"\n", "formt"},
		"bad format":      {"format: \"{date}/{foo}\"\n", "format: unknown placeholder {foo}"},
		"preset no name":  {"presets:\n  - format: \"{date}\"\n", "the name is empty"},
		"preset bad form": {"presets:\n  - name: x\n    format: \"/{date}\"\n", "presets[0] (x)"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(writeConfig(t, tt.body))
			if err == nil || !strings.Contains(err.Error(), tt.err) {
				t.Errorf("err = %v, want one with %q", err, tt.err)
			}
		})
	}
}

func TestConfigPresets(t *testing.T) {
	c := Config{Presets: []Preset{{Name: "by date", Format: "days/{date}/{time}"}, {Name: "for the blog", Format: "blog/{date}_{orig}"}}}
	all := c.AllPresets()
	if len(all) != len(rename.Presets)+1 || all[1].Name != "by date" || all[1].Format != "days/{date}/{time}" || all[len(all)-1].Name != "for the blog" {
		t.Errorf("presets = %+v", all)
	}
	if f, err := c.FindPreset("for the blog"); err != nil || f != "blog/{date}_{orig}" {
		t.Errorf("findPreset = %q, %v", f, err)
	}
	if _, err := c.FindPreset("nope"); err == nil || !strings.Contains(err.Error(), `"by extension, then date"`) {
		t.Errorf("err = %v, want the names listed", err)
	}
}
