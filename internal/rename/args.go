package rename

import (
	"os"
	"path/filepath"
	"strings"
)

// Args are the files and directories given, with the working directory they
// are looked up from, so that a path can be told under them without asking
// for the working directory each time.
type Args struct {
	given []string
	abs   []string
	cwd   string
}

// NewArgs returns the Args of given.
func NewArgs(given []string) Args {
	a := Args{given: given}
	a.cwd, _ = os.Getwd()
	for _, g := range given {
		a.abs = append(a.abs, a.Abs(g))
	}
	return a
}

// Abs is AbsPath, from the working directory of a.
func (a Args) Abs(path string) string {
	if filepath.IsAbs(path) || a.cwd == "" {
		return AbsPath(path)
	}
	return filepath.Join(a.cwd, path)
}

// Parent returns the directory the argument path came from is in: for a
// directory given, its parent; for a file given, its directory.
func (a Args) Parent(path string) string {
	abs := a.Abs(path)
	best := -1
	for i, g := range a.abs {
		if abs == g || strings.HasPrefix(abs, g+string(filepath.Separator)) {
			// the longest match, for arguments inside one another
			if best < 0 || len(a.given[i]) > len(a.given[best]) {
				best = i
			}
		}
	}
	if best < 0 || a.abs[best] == abs {
		return filepath.Dir(path)
	}
	return filepath.Dir(filepath.Clean(a.given[best]))
}
