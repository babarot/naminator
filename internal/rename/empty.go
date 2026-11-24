package rename

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// junkFiles are files that the OS creates by itself. A directory that has
// only these left counts as empty.
var junkFiles = map[string]bool{
	".DS_Store":   true, // macOS Finder
	"Thumbs.db":   true, // Windows Explorer
	"desktop.ini": true, // Windows Explorer
}

// PendingMoves records the renames that dry-run would do but has not done.
type PendingMoves struct {
	mu   sync.Mutex
	from map[string]bool // files that would be moved
	to   []string        // paths they would be moved to
}

func NewPendingMoves() *PendingMoves {
	return &PendingMoves{from: map[string]bool{}}
}

// Add records a move from src to dst. It does nothing on a nil receiver.
func (m *PendingMoves) Add(src, dst string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.from[AbsPath(src)] = true
	m.to = append(m.to, AbsPath(dst))
}

// movedAway reports whether the file at path would be moved.
func (m *PendingMoves) movedAway(path string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.from[AbsPath(path)]
}

// movedInto reports whether a file would be moved to somewhere under dir.
func (m *PendingMoves) movedInto(dir string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	prefix := AbsPath(dir) + string(filepath.Separator)
	for _, to := range m.to {
		if strings.HasPrefix(to, prefix) {
			return true
		}
	}
	return false
}

func AbsPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

// EmptyDirs reports whether dir is empty, and returns the empty directories
// to remove: dir itself if it is empty, or else the outermost empty
// directories under it. A directory is empty when it has nothing but junk
// files and empty directories.
//
// With moves, it predicts the result after those moves: files that would be
// moved away count as gone, and a directory that a file would be moved into
// is not empty.
func EmptyDirs(dir string, moves *PendingMoves) (bool, []string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, nil, err
	}
	empty := true
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			subEmpty, subDirs, err := EmptyDirs(filepath.Join(dir, entry.Name()), moves)
			if err != nil {
				return false, nil, err
			}
			empty = empty && subEmpty
			dirs = append(dirs, subDirs...)
			continue
		}
		if !junkFiles[entry.Name()] && !moves.movedAway(filepath.Join(dir, entry.Name())) {
			empty = false
		}
	}
	if empty && moves.movedInto(dir) {
		empty = false
	}
	if empty {
		return true, []string{dir}, nil
	}
	return false, dirs, nil
}
