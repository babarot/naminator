package rename

import (
	"strings"
	"testing"
	"time"

	"github.com/babarot/naminator/internal/photo"
)

func TestParseTemplate(t *testing.T) {
	tests := []struct {
		format string
		err    string // a part of the error; "" for none
	}{
		{"{date}_{time}", ""},
		{"{ext}/{date}/{date}_{time}", ""},
		{"{year}{month}{day}_{hour}{min}{sec}", ""},
		{"{camera}/{date}_{seq}", ""},
		{"{dir}/{orig}", ""},
		{"photos-{date}", ""},
		{"", "empty"},
		{"  ", "empty"},
		{"{date}/{foo}", "unknown placeholder {foo}"},
		{"{date", "not closed"},
		{"/{date}", "starts with /"},
		{"{date}/", "ends with /"},
		{"../{date}", "above the base"},
		{"{date}/../{time}", "above the base"},
	}
	for _, tt := range tests {
		_, err := ParseTemplate(tt.format)
		switch {
		case tt.err == "" && err != nil:
			t.Errorf("parseTemplate(%q) = %v, want no error", tt.format, err)
		case tt.err != "" && (err == nil || !strings.Contains(err.Error(), tt.err)):
			t.Errorf("parseTemplate(%q) = %v, want an error with %q", tt.format, err, tt.err)
		}
	}
}

func TestTemplateRender(t *testing.T) {
	photo := photo.Photo{
		Name:      "DSC05501.ARW",
		Extension: "arw",
		Camera:    "ILCE-7CM2",
		CreatedAt: time.Date(2026, 10, 4, 14, 22, 9, 0, time.UTC),
	}
	tests := []struct{ format, want string }{
		{"{date}_{time}", "2026-10-04_14-22-09.arw"},
		{"{year}{month}{day}_{hour}{min}{sec}", "20261004_142209.arw"},
		{"{ext}/{date}/{orig}", "arw/2026-10-04/DSC05501.arw"},
		{"{camera}/{date}_{seq}", "ILCE-7CM2/2026-10-04_" + seqMark + ".arw"},
		{"{dir}/{date}", "10551007/2026-10-04.arw"},
	}
	for _, tt := range tests {
		if got := MustParseTemplate(tt.format).render(photo, "10551007"); got != tt.want {
			t.Errorf("render(%q) = %q, want %q", tt.format, got, tt.want)
		}
	}

	// a camera model is one path segment; an unknown one is named so
	for camera, want := range map[string]string{"iPhone 15 Pro": "iPhone_15_Pro", "A/B": "A_B", "": "unknown"} {
		photo.Camera = camera
		if got := Value(photo, "camera", ""); got != want {
			t.Errorf("camera %q = %q, want %q", camera, got, want)
		}
	}
}

func TestPresetsParse(t *testing.T) {
	for _, p := range Presets {
		if _, err := ParseTemplate(p.Format); err != nil {
			t.Errorf("preset %q: %v", p.Name, err)
		}
	}
}
