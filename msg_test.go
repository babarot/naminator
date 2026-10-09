package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExifResultMsgString(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		msg := exifResultMsg{
			photo:    Photo{Name: "photo.jpg", Path: "/tmp/photo.jpg"},
			duration: duration(1500 * time.Millisecond),
		}
		s := msg.String()
		if !strings.Contains(s, "photo.jpg") {
			t.Errorf("should contain filename, got %q", s)
		}
		if !strings.Contains(s, "Got exif data") {
			t.Errorf("should contain 'Got exif data', got %q", s)
		}
		if !strings.Contains(s, "1.50s") {
			t.Errorf("should contain duration, got %q", s)
		}
	})

	t.Run("error", func(t *testing.T) {
		msg := exifResultMsg{
			photo: Photo{Name: "bad.jpg", Path: "/tmp/bad.jpg"},
			err:   errors.New("no exif"),
		}
		s := msg.String()
		if !strings.Contains(s, "bad.jpg") {
			t.Errorf("should contain filename, got %q", s)
		}
		if !strings.Contains(s, "no exif") {
			t.Errorf("should contain error message, got %q", s)
		}
	})
}

func TestRenameResultMsgString(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		msg := renameResultMsg{
			photo: Photo{
				Name:        "photo.jpg",
				Path:        "/tmp/photo.jpg",
				RenamedPath: "/tmp/renamed/2024-01-01_12-00-00.jpg",
			},
		}
		s := msg.String()
		if !strings.Contains(s, "photo.jpg") {
			t.Errorf("should contain filename, got %q", s)
		}
		if !strings.Contains(s, "Renamed to") {
			t.Errorf("should contain 'Renamed to', got %q", s)
		}
	})

	t.Run("dryrun", func(t *testing.T) {
		msg := renameResultMsg{
			photo: Photo{
				Name:        "photo.jpg",
				Path:        "/tmp/photo.jpg",
				RenamedPath: "/tmp/renamed/2024-01-01_12-00-00.jpg",
			},
			dryrun: true,
		}
		s := msg.String()
		if !strings.Contains(s, "Would rename") {
			t.Errorf("should contain 'Would rename', got %q", s)
		}
	})

	t.Run("error", func(t *testing.T) {
		msg := renameResultMsg{
			photo: Photo{Name: "photo.jpg", Path: "/tmp/photo.jpg"},
			err:   errors.New("permission denied"),
		}
		s := msg.String()
		if !strings.Contains(s, "permission denied") {
			t.Errorf("should contain error message, got %q", s)
		}
	})
}

func TestCleanResultMsgString(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		msg := cleanResultMsg{dir: "mydir", err: errors.New("rm failed")}
		s := msg.String()
		if !strings.Contains(s, "mydir") || !strings.Contains(s, "rm failed") {
			t.Errorf("unexpected output: %q", s)
		}
	})

	t.Run("dryrun", func(t *testing.T) {
		msg := cleanResultMsg{dir: "mydir", dryrun: true}
		s := msg.String()
		if !strings.Contains(s, "Would remove if empty") {
			t.Errorf("unexpected output: %q", s)
		}
	})

	t.Run("empty removed", func(t *testing.T) {
		msg := cleanResultMsg{dir: "mydir", empty: true}
		s := msg.String()
		if !strings.Contains(s, "Removed because empty") {
			t.Errorf("unexpected output: %q", s)
		}
	})

	t.Run("not empty skip", func(t *testing.T) {
		msg := cleanResultMsg{dir: "mydir", empty: false}
		s := msg.String()
		if !strings.Contains(s, "NOT empty") {
			t.Errorf("unexpected output: %q", s)
		}
	})
}

func TestResultMsgInterface(t *testing.T) {
	// Verify all message types implement resultMsg
	var _ resultMsg = exifResultMsg{}
	var _ resultMsg = renameResultMsg{}
	var _ resultMsg = cleanResultMsg{}
}

func TestAbbrevHome(t *testing.T) {
	tests := []struct {
		path, home, want string
	}{
		{"/home/user/photos/a.jpg", "/home/user", "~/photos/a.jpg"},
		{"/home/user/photos/a.jpg", "/home/user/", "~/photos/a.jpg"},
		{"/home/user", "/home/user", "~"},
		// Only a whole leading directory is replaced
		{"/home/username/a.jpg", "/home/user", "/home/username/a.jpg"},
		{"/backup/home/user/a.jpg", "/home/user", "/backup/home/user/a.jpg"},
		// An unset HOME leaves the path as is
		{"/photos/a.jpg", "", "/photos/a.jpg"},
	}
	for _, tt := range tests {
		if got := abbrevHome(tt.path, tt.home); got != tt.want {
			t.Errorf("abbrevHome(%q, %q) = %q, want %q", tt.path, tt.home, got, tt.want)
		}
	}
}
