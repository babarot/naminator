package photo

import (
	"errors"
	"testing"

	"github.com/barasher/go-exiftool"
)

func TestParseExifTime(t *testing.T) {
	tests := []struct {
		value string
		// want is the clock time the name is made of, and offset the UTC
		// offset in seconds the result should keep
		want   string
		offset int
	}{
		{"2024:01:02 15:04:05", "2024-01-02 15:04:05", 0},
		{"2024:01:02 15:04:05+09:00", "2024-01-02 15:04:05", 9 * 60 * 60},
		{"2024:01:02 15:04:05.123+09:00", "2024-01-02 15:04:05.123", 9 * 60 * 60},
		{"2024:01:02 15:04:05.12+09:00", "2024-01-02 15:04:05.12", 9 * 60 * 60},
		{"2024:01:02 15:04:05.1234-05:00", "2024-01-02 15:04:05.1234", -5 * 60 * 60},
		{"2024:01:02 15:04:05.5Z", "2024-01-02 15:04:05.5", 0},
		{"2024:01:02 15:04:05.123", "2024-01-02 15:04:05.123", 0},
		// Taken in Paris: kept as the local time there, not converted
		{"2024:07:14 23:30:00+02:00", "2024-07-14 23:30:00", 2 * 60 * 60},
		// A clock time skipped by daylight saving time in many zones
		{"2024:03:10 02:30:00", "2024-03-10 02:30:00", 0},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := parseExifTime(tt.value)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s := got.Format("2006-01-02 15:04:05.999999999"); s != tt.want {
				t.Errorf("parseExifTime(%q) = %q, want %q", tt.value, s, tt.want)
			}
			if _, offset := got.Zone(); offset != tt.offset {
				t.Errorf("parseExifTime(%q) offset = %d, want %d", tt.value, offset, tt.offset)
			}
		})
	}

	if _, err := parseExifTime("not a time"); err == nil {
		t.Error("expected an error for an invalid value")
	}
}

// A photo without a shooting time says so, rather than the keys missing.
func TestPhotoOfNoShootingTime(t *testing.T) {
	info := exiftool.FileMetadata{Fields: map[string]any{"FileName": "screenshot.jpg", "SourceFile": "a/screenshot.jpg"}}
	if _, err := photoOf(Photo{}, info); !errors.Is(err, ErrNoShootingTime) || err.Error() != "no shooting time in its EXIF; skipped" {
		t.Errorf("err = %v", err)
	}
	info.Fields["DateTimeOriginal"] = "not a date"
	if _, err := photoOf(Photo{}, info); err == nil || errors.Is(err, ErrNoShootingTime) {
		t.Errorf("a date that cannot be read: err = %v", err)
	}
}
