package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		c, err := loadConfig(filepath.Join(t.TempDir(), "none.yaml"))
		if err != nil || c.Format != "" || c.Clean || len(c.Presets) != 0 {
			t.Errorf("config = %+v, %v, want empty", c, err)
		}
	})
	t.Run("valid", func(t *testing.T) {
		c, err := loadConfig(writeConfig(t, "format: \"{ext}/{date}/{date}_{time}\"\nclean: true\npresets:\n  - name: for the blog\n    format: \"blog/{date}/{date}_{orig}\"\n"))
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
			_, err := loadConfig(writeConfig(t, tt.body))
			if err == nil || !strings.Contains(err.Error(), tt.err) {
				t.Errorf("err = %v, want one with %q", err, tt.err)
			}
		})
	}
}

func TestConfigPresets(t *testing.T) {
	c := config{Presets: []configPreset{{Name: "by date", Format: "days/{date}/{time}"}, {Name: "for the blog", Format: "blog/{date}_{orig}"}}}
	all := c.allPresets()
	if len(all) != len(presets)+1 || all[1].name != "by date" || all[1].format != "days/{date}/{time}" || all[len(all)-1].name != "for the blog" {
		t.Errorf("presets = %+v", all)
	}
	if f, err := c.findPreset("for the blog"); err != nil || f != "blog/{date}_{orig}" {
		t.Errorf("findPreset = %q, %v", f, err)
	}
	if _, err := c.findPreset("nope"); err == nil || !strings.Contains(err.Error(), `"by extension, then date"`) {
		t.Errorf("err = %v, want the names listed", err)
	}
}

func TestChooseNamer(t *testing.T) {
	photo := Photo{Name: "DSC05501.ARW", Path: "Desktop/10551007/DSC05501.ARW", Dir: "Desktop/10551007", Extension: "arw", CreatedAt: time.Date(2026, 10, 4, 14, 22, 9, 0, time.UTC)}
	args := []string{"Desktop/10551007"}
	withConfig := config{Format: "{ext}/{date}/{date}_{time}", Presets: []configPreset{{Name: "flat", Format: "{date}_{time}"}}}
	tests := []struct {
		name string
		opt  Option
		cfg  config
		want string // the target, or the error with "error: "
		note bool
	}{
		{"nothing: as before", Option{}, config{}, "Desktop/10551007/2026-10-04_14-22-09.arw", false},
		{"-d alone: as before", Option{DestDir: "/out"}, config{}, "/out/2026-10-04_14-22-09.arw", false},
		{"-tecE: as before, with a note", Option{GroupByDate: true, GroupByExt: true, GroupExtFirst: true}, config{}, "Desktop/arw/2026-10-04/2026-10-04_14-22-09.arw", true},
		{"--format", Option{Format: "{ext}/{date}/{date}_{time}"}, config{}, "Desktop/arw/2026-10-04/2026-10-04_14-22-09.arw", false},
		{"--preset", Option{Preset: "by date"}, config{}, "Desktop/2026-10-04/2026-10-04_14-22-09.arw", false},
		{"--preset from the config", Option{Preset: "flat"}, withConfig, "Desktop/2026-10-04_14-22-09.arw", false},
		{"the config's format", Option{}, withConfig, "Desktop/arw/2026-10-04/2026-10-04_14-22-09.arw", false},
		{"the config's format with -d", Option{DestDir: "/out"}, withConfig, "/out/arw/2026-10-04/2026-10-04_14-22-09.arw", false},
		{"--format over the config", Option{Format: "{date}"}, withConfig, "Desktop/2026-10-04.arw", false},
		{"-t over the config", Option{GroupByDate: true}, withConfig, "Desktop/2026-10-04/2026-10-04_14-22-09.arw", true},
		{"-t with --format", Option{GroupByDate: true, Format: "{date}"}, config{}, "error: either -t", false},
		{"--format with --preset", Option{Format: "{date}", Preset: "by date"}, config{}, "error: either --format", false},
		{"a bad --format", Option{Format: "{nope}"}, config{}, "error: --format: unknown placeholder", false},
		{"an unknown --preset", Option{Preset: "nope"}, config{}, "error: no preset named", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, note, err := chooseNamer(tt.opt, tt.cfg, args)
			if want, ok := strings.CutPrefix(tt.want, "error: "); ok {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("err = %v, want one with %q", err, want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := n.target(photo); got != tt.want {
				t.Errorf("target = %q, want %q", got, tt.want)
			}
			if (note != "") != tt.note {
				t.Errorf("note = %q, want one: %v", note, tt.note)
			}
		})
	}
}
