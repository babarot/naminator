package main

import "testing"

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
