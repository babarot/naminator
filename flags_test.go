package main

import (
	"strings"
	"testing"
	"time"

	"github.com/babarot/naminator/internal/config"
	"github.com/babarot/naminator/internal/photo"
)

func TestChooseNamer(t *testing.T) {
	photo := photo.Photo{Name: "DSC05501.ARW", Path: "Desktop/10551007/DSC05501.ARW", Dir: "Desktop/10551007", Extension: "arw", CreatedAt: time.Date(2026, 10, 4, 14, 22, 9, 0, time.UTC)}
	args := []string{"Desktop/10551007"}
	withConfig := config.Config{Format: "{ext}/{date}/{date}_{time}", Presets: []config.Preset{{Name: "flat", Format: "{date}_{time}"}}}
	tests := []struct {
		name string
		opt  Option
		cfg  config.Config
		want string // the target, or the error with "error: "
		note bool
	}{
		{"nothing: as before", Option{}, config.Config{}, "Desktop/10551007/2026-10-04_14-22-09.arw", false},
		{"-d alone: as before", Option{DestDir: "/out"}, config.Config{}, "/out/2026-10-04_14-22-09.arw", false},
		{"-tecE: as before, with a note", Option{GroupByDate: true, GroupByExt: true, GroupExtFirst: true}, config.Config{}, "Desktop/arw/2026-10-04/2026-10-04_14-22-09.arw", true},
		{"--format", Option{Format: "{ext}/{date}/{date}_{time}"}, config.Config{}, "Desktop/arw/2026-10-04/2026-10-04_14-22-09.arw", false},
		{"--preset", Option{Preset: "by date"}, config.Config{}, "Desktop/2026-10-04/2026-10-04_14-22-09.arw", false},
		{"--preset from the config", Option{Preset: "flat"}, withConfig, "Desktop/2026-10-04_14-22-09.arw", false},
		{"the config's format", Option{}, withConfig, "Desktop/arw/2026-10-04/2026-10-04_14-22-09.arw", false},
		{"the config's format with -d", Option{DestDir: "/out"}, withConfig, "/out/arw/2026-10-04/2026-10-04_14-22-09.arw", false},
		{"--format over the config", Option{Format: "{date}"}, withConfig, "Desktop/2026-10-04.arw", false},
		{"-t over the config", Option{GroupByDate: true}, withConfig, "Desktop/2026-10-04/2026-10-04_14-22-09.arw", true},
		{"-t with --format", Option{GroupByDate: true, Format: "{date}"}, config.Config{}, "error: either -t", false},
		{"--format with --preset", Option{Format: "{date}", Preset: "by date"}, config.Config{}, "error: either --format", false},
		{"a bad --format", Option{Format: "{nope}"}, config.Config{}, "error: --format: unknown placeholder", false},
		{"an unknown --preset", Option{Preset: "nope"}, config.Config{}, "error: no preset named", false},
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
			if got := n.Target(photo); got != tt.want {
				t.Errorf("target = %q, want %q", got, tt.want)
			}
			if (note != "") != tt.note {
				t.Errorf("note = %q, want one: %v", note, tt.note)
			}
		})
	}
}
