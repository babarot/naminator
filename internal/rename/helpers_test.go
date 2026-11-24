package rename

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeTree creates files (and their directories) under root, and directories
// for paths ending with "/".
func makeTree(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		path := filepath.Join(root, p)
		if strings.HasSuffix(p, "/") {
			if err := os.MkdirAll(path, 0755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// writeTIFF writes the start of a TIFF file, which is enough to be read as
// an image (raw files are TIFF based).
func writeTIFF(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("II*\x00\x08\x00\x00\x00\x00\x00"), 0644); err != nil {
		t.Fatal(err)
	}
}

// osFS is the real filesystem, as main has it.
type osFS struct{}

func (osFS) Rename(oldpath, newpath string) error         { return os.Rename(oldpath, newpath) }
func (osFS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (osFS) RemoveAll(path string) error                  { return os.RemoveAll(path) }
func (osFS) Stat(name string) (os.FileInfo, error)        { return os.Stat(name) }
func (osFS) ReadDir(name string) ([]os.DirEntry, error)   { return os.ReadDir(name) }
