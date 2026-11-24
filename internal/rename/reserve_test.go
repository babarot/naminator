package rename

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathReserver(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, nil, 0644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	want := filepath.Join(dir, "2024-03-15_14-30-45.jpg")

	t.Run("skips a path taken by another file", func(t *testing.T) {
		write("2024-03-15_14-30-45.jpg")
		src := write("other.jpg")
		got := NewReserver().Reserve(osFS{}, src, want)
		if exp := filepath.Join(dir, "2024-03-15_14-30-45_1.jpg"); got != exp {
			t.Errorf("reserve() = %q, want %q", got, exp)
		}
	})

	t.Run("keeps the path the source already has", func(t *testing.T) {
		src := write("2024-03-15_14-30-45.jpg")
		got := NewReserver().Reserve(osFS{}, src, want)
		if got != want {
			t.Errorf("reserve() = %q, want %q", got, want)
		}
	})
}
