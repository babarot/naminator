package rename

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEmptyDirs(t *testing.T) {
	tests := []struct {
		name      string
		tree      []string
		wantEmpty bool
		wantDirs  []string
	}{
		{"empty", nil, true, []string{"."}},
		{"only junk files", []string{".DS_Store", "Thumbs.db", "desktop.ini"}, true, []string{"."}},
		{"a file", []string{"a.jpg"}, false, nil},
		{"nested empty directories", []string{"a/b/", "a/.DS_Store", "c/"}, true, []string{"."}},
		{
			"empty directories next to a file",
			[]string{"100MSDCF/.DS_Store", "2024-03-15/a.jpg", "2024-03-15/old/", "misc/x/", "misc/y/"},
			false,
			[]string{"100MSDCF", "2024-03-15/old", "misc"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			makeTree(t, root, tt.tree...)
			empty, dirs, err := EmptyDirs(root, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if empty != tt.wantEmpty {
				t.Errorf("empty = %v, want %v", empty, tt.wantEmpty)
			}
			var got []string
			for _, dir := range dirs {
				rel, _ := filepath.Rel(root, dir)
				got = append(got, rel)
			}
			if strings.Join(got, ",") != strings.Join(tt.wantDirs, ",") {
				t.Errorf("dirs = %v, want %v", got, tt.wantDirs)
			}
		})
	}

	t.Run("predicts with pending moves", func(t *testing.T) {
		tests := []struct {
			name      string
			moves     [][2]string // src, dst relative to the parent of root
			wantEmpty bool
			wantDirs  []string
		}{
			{"nothing moved", nil, false, nil},
			{
				"all moved out",
				[][2]string{{"DCIM/100MSDCF/a.jpg", "2024-03-15/a.jpg"}, {"DCIM/100MSDCF/b.jpg", "2024-03-15/b.jpg"}},
				true, []string{"."},
			},
			{
				"moved into a new directory under root",
				[][2]string{{"DCIM/100MSDCF/a.jpg", "DCIM/2024-03-15/a.jpg"}, {"DCIM/100MSDCF/b.jpg", "DCIM/2024-03-15/b.jpg"}},
				false, []string{"100MSDCF"},
			},
			{
				"renamed in place",
				[][2]string{{"DCIM/100MSDCF/a.jpg", "DCIM/100MSDCF/x.jpg"}, {"DCIM/100MSDCF/b.jpg", "DCIM/100MSDCF/y.jpg"}},
				false, nil,
			},
			{
				"one file left",
				[][2]string{{"DCIM/100MSDCF/a.jpg", "2024-03-15/a.jpg"}},
				false, nil,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				parent := t.TempDir()
				root := filepath.Join(parent, "DCIM")
				makeTree(t, root, "100MSDCF/a.jpg", "100MSDCF/b.jpg", "100MSDCF/.DS_Store")
				moves := NewPendingMoves()
				for _, m := range tt.moves {
					moves.Add(filepath.Join(parent, m[0]), filepath.Join(parent, m[1]))
				}
				empty, dirs, err := EmptyDirs(root, moves)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if empty != tt.wantEmpty {
					t.Errorf("empty = %v, want %v", empty, tt.wantEmpty)
				}
				var got []string
				for _, dir := range dirs {
					rel, _ := filepath.Rel(root, dir)
					got = append(got, rel)
				}
				if strings.Join(got, ",") != strings.Join(tt.wantDirs, ",") {
					t.Errorf("dirs = %v, want %v", got, tt.wantDirs)
				}
			})
		}
	})

	t.Run("non-existent dir", func(t *testing.T) {
		if _, _, err := EmptyDirs("/nonexistent/path", nil); err == nil {
			t.Error("expected error for non-existent dir")
		}
	})
}
