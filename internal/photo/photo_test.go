package photo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWalkDir(t *testing.T) {
	dir := t.TempDir()

	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(subDir, "c.png"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	files, err := walkDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Errorf("walkDir() returned %d files, want 3: %v", len(files), files)
	}

	for _, f := range files {
		fi, _ := os.Stat(f)
		if fi.IsDir() {
			t.Errorf("walkDir() should not include directories, got %s", f)
		}
	}
}

func TestWalkDirEmpty(t *testing.T) {
	dir := t.TempDir()
	files, err := walkDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestWalkDirNonExistent(t *testing.T) {
	_, err := walkDir("/nonexistent/path")
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}
