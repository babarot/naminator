package rename

import "os"

// FileSystem abstracts file system operations for testability.
type FileSystem interface {
	Rename(oldpath, newpath string) error
	MkdirAll(path string, perm os.FileMode) error
	RemoveAll(path string) error
	Stat(name string) (os.FileInfo, error)
	ReadDir(name string) ([]os.DirEntry, error)
}
