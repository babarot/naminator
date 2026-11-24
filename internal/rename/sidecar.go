package rename

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/babarot/naminator/internal/photo"
)

// Sidecars are files that go with a photo and are named after it, such as
// the edits of a raw developer (DSC05501.xmp, or DSC05501.ARW.xmp) or of
// iOS (IMG_2033.AAE). They are renamed with the photo they belong to, so
// that they keep matching it.

// sidecarExts are the extensions of the files taken as sidecars.
var sidecarExts = []string{".xmp", ".aae", ".dop", ".pp3"}

// rasterExts are the extensions of the photos that are not raw files: when
// a raw file and one of these share a sidecar, the raw file owns it.
var rasterExts = []string{".jpg", ".jpeg", ".heic", ".heif", ".hif", ".png", ".tif", ".tiff", ".webp"}

// SidecarWarning is a sidecar left where it is, and why.
type SidecarWarning struct {
	Path   string
	Reason string
}

// FindSidecars returns the sidecars of photos by photo path, and the
// sidecars that are left because no single photo owns them. A sidecar is in
// the directory of its photo and named after its file name (DSC05501.ARW.xmp)
// or after its name without the extension (DSC05501.xmp). The latter can be
// shared by several photos (DSC05501.ARW and DSC05501.JPG): the raw file
// owns it when there is one; otherwise it is left.
//
// The owner is chosen among all the images in the directory, whether they
// are renamed or not, so that a sidecar is not taken from a raw file whose
// EXIF data cannot be read, or which was not given. A sidecar whose owner
// is not renamed stays where it is.
func FindSidecars(fsys FileSystem, photos []photo.Photo) (map[string][]string, []SidecarWarning) {
	renamed := map[string]bool{}
	var dirs []string
	seen := map[string]bool{}
	for _, p := range photos {
		renamed[p.Path] = true
		if dir := filepath.Dir(p.Path); !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	owned := map[string][]string{}
	var warnings []SidecarWarning
	for _, dir := range dirs {
		entries, err := fsys.ReadDir(dir)
		if err != nil {
			continue
		}
		var images, sidecars []string
		for _, e := range entries {
			name := e.Name()
			switch {
			case e.IsDir():
			case slices.Contains(sidecarExts, strings.ToLower(filepath.Ext(name))):
				sidecars = append(sidecars, name)
			case renamed[filepath.Join(dir, name)] || photo.IsImage(filepath.Join(dir, name)):
				images = append(images, name)
			}
		}
		for _, name := range sidecars {
			path := filepath.Join(dir, name)
			owner, reason := sidecarOwner(images, strings.TrimSuffix(name, filepath.Ext(name)))
			switch {
			case reason != "":
				warnings = append(warnings, SidecarWarning{Path: path, Reason: reason})
			case owner != "" && renamed[filepath.Join(dir, owner)]:
				owned[filepath.Join(dir, owner)] = append(owned[filepath.Join(dir, owner)], path)
			}
		}
	}
	return owned, warnings
}

// sidecarOwner returns the name of the image a sidecar named stem plus an
// extension belongs to, or why it belongs to none of them when several
// match. Both are empty when no image matches: the file is not a sidecar.
func sidecarOwner(images []string, stem string) (owner, reason string) {
	var matches []string
	for _, name := range images {
		if name == stem {
			// DSC05501.ARW.xmp names a single photo
			return name, ""
		}
		if strings.TrimSuffix(name, filepath.Ext(name)) == stem {
			matches = append(matches, name)
		}
	}
	if len(matches) == 1 {
		return matches[0], ""
	}
	var raws []string
	for _, name := range matches {
		if !slices.Contains(rasterExts, strings.ToLower(filepath.Ext(name))) {
			raws = append(raws, name)
		}
	}
	if len(raws) == 1 {
		return raws[0], ""
	}
	if len(matches) == 0 {
		return "", ""
	}
	return "", fmt.Sprintf("not moved: it is named after %s, and none of them is the only raw file", strings.Join(matches, " and "))
}

// SidecarTarget returns the path the sidecar at path goes to when its
// photo is renamed from photo to renamed: named after the new name as it
// was named after the old one.
func SidecarTarget(photo, renamed, path string) string {
	name := filepath.Base(path)
	ext := filepath.Ext(name)
	if strings.TrimSuffix(name, ext) == filepath.Base(photo) {
		// DSC05501.ARW.xmp goes to 2026-10-04_14-22-09.arw.xmp
		return renamed + ext
	}
	return strings.TrimSuffix(renamed, filepath.Ext(renamed)) + ext
}
