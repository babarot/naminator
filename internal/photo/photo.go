// Package photo is what naminator knows of a photo: its EXIF data, read
// with exiftool, and the files that are photos.
package photo

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
)

// SortByTime sorts photos in the order they were taken, the order they are
// renamed in: photos taken in the same second get _1, _2, ... in that
// order, and the files of one shot (e.g. RAW and HEIF, with the same
// original name) get the same number.
func SortByTime(photos []Photo) {
	slices.SortStableFunc(photos, func(a, b Photo) int {
		if n := a.CreatedAt.Compare(b.CreatedAt); n != 0 {
			return n
		}
		if n := strings.Compare(a.Name, b.Name); n != 0 {
			return n
		}
		return strings.Compare(a.Path, b.Path)
	})
}

// Paths returns the paths of photos, the rows of the rename phase.
func Paths(photos []Photo) []string {
	keys := make([]string, len(photos))
	for i, photo := range photos {
		keys[i] = photo.Path
	}
	return keys
}

type Photo struct {
	Name        string
	Path        string
	RenamedPath string
	Dir         string
	Extension   string
	// Camera is the model of the camera, from EXIF (empty if unknown)
	Camera string
	// Sidecars are the files that go with the photo (see rename.FindSidecars), and
	// RenamedSidecars where they went
	Sidecars        []string
	RenamedSidecars []string
	// CreatedAt is the clock time where the photo was taken (see parseExifTime)
	CreatedAt time.Time
}

func walkDir(root string) ([]string, error) {
	files := []string{}
	err := filepath.WalkDir(root, func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func Images(dirs []string) ([]string, error) {
	var images []string
	for _, dir := range dirs {
		files, err := walkDir(dir)
		if err != nil {
			return []string{}, err
		}
		for _, file := range files {
			mime, _ := mimetype.DetectFile(file)
			if !strings.Contains(mime.String(), "image") {
				continue
			}
			images = append(images, file)
		}
	}
	return images, nil
}

// IsImage reports whether the file at path is an image, by its content.
func IsImage(path string) bool {
	mime, err := mimetype.DetectFile(path)
	return err == nil && strings.HasPrefix(mime.String(), "image/")
}
