package ui

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
