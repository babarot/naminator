package rename

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Reserver hands out destination paths so that no two photos in a run,
// and no photo and an existing file, end up with the same path.
type Reserver struct {
	mu    sync.Mutex
	paths map[string]bool
}

func NewReserver() *Reserver {
	return &Reserver{paths: map[string]bool{}}
}

// Reserve returns want if it is free, or else the first free path made by
// adding _1, _2, ... before the extension. A path that src itself already
// occupies counts as free, so renaming a photo to its current name is a no-op.
func (r *Reserver) Reserve(fsys FileSystem, src, want string) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	ext := filepath.Ext(want)
	base := strings.TrimSuffix(want, ext)
	for i := 0; ; i++ {
		path := want
		if i > 0 {
			path = fmt.Sprintf("%s_%d%s", base, i, ext)
		}
		if r.paths[path] || IsOtherFile(fsys, path, src) {
			continue
		}
		r.paths[path] = true
		return path
	}
}

// Claim reserves path for src when it is free: not handed out yet, and no
// file other than src is there.
func (r *Reserver) Claim(fsys FileSystem, src, path string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.paths[path] || IsOtherFile(fsys, path, src) {
		return false
	}
	r.paths[path] = true
	return true
}

// IsOtherFile reports whether path exists and is not the file at src.
func IsOtherFile(fsys FileSystem, path, src string) bool {
	fi, err := fsys.Stat(path)
	if err != nil {
		return false
	}
	srcFi, err := fsys.Stat(src)
	return err != nil || !os.SameFile(fi, srcFi)
}
