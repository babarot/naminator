package rename

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/babarot/naminator/internal/photo"
)

func TestFindSidecars(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root,
		"card/DSC05501.ARW", "card/DSC05501.JPG", "card/DSC05501.xmp", // shared: the raw file owns it
		"card/DSC05502.ARW", "card/DSC05502.ARW.xmp", // named after the file name
		"card/DSC05503.JPG", "card/DSC05503.HEIC", "card/DSC05503.xmp", // no raw file: left
		"card/DSC05504.JPG", "card/DSC05504.XMP", // upper case extension
		"card/notes.xmp",    // no photo of that name: not a sidecar
		"card/DSC05501.txt", // not a sidecar extension
		"iphone/IMG_2033.HEIC", "iphone/IMG_2033.AAE",
	)
	var photos []photo.Photo
	for _, p := range []string{"card/DSC05501.ARW", "card/DSC05501.JPG", "card/DSC05502.ARW", "card/DSC05503.JPG", "card/DSC05503.HEIC", "card/DSC05504.JPG", "iphone/IMG_2033.HEIC"} {
		photos = append(photos, photo.Photo{Path: filepath.Join(root, p)})
	}
	owned, warnings := FindSidecars(osFS{}, photos)
	got := map[string]string{}
	for photo, scs := range owned {
		for _, sc := range scs {
			got[filepath.Base(sc)] = filepath.Base(photo)
		}
	}
	want := map[string]string{
		"DSC05501.xmp":     "DSC05501.ARW",
		"DSC05502.ARW.xmp": "DSC05502.ARW",
		"DSC05504.XMP":     "DSC05504.JPG",
		"IMG_2033.AAE":     "IMG_2033.HEIC",
	}
	if len(got) != len(want) {
		t.Errorf("owned = %v, want %v", got, want)
	}
	for sc, photo := range want {
		if got[sc] != photo {
			t.Errorf("%s belongs to %q, want %q", sc, got[sc], photo)
		}
	}
	if len(warnings) != 1 || filepath.Base(warnings[0].Path) != "DSC05503.xmp" || !strings.Contains(warnings[0].Reason, "DSC05503.HEIC") {
		t.Errorf("warnings = %+v, want DSC05503.xmp left", warnings)
	}
}

func TestSidecarTarget(t *testing.T) {
	tests := []struct{ sidecar, want string }{
		{"/c/DSC05501.xmp", "/o/2026-10-04_14-22-09.xmp"},
		{"/c/DSC05501.XMP", "/o/2026-10-04_14-22-09.XMP"},
		{"/c/DSC05501.ARW.xmp", "/o/2026-10-04_14-22-09.arw.xmp"},
	}
	for _, tt := range tests {
		if got := SidecarTarget("/c/DSC05501.ARW", "/o/2026-10-04_14-22-09.arw", tt.sidecar); got != tt.want {
			t.Errorf("sidecarTarget(%q) = %q, want %q", tt.sidecar, got, tt.want)
		}
	}
}

// A raw file that is not renamed (its EXIF data cannot be read, or it was
// not given) still owns the sidecar it shares: the JPEG does not take it.
func TestFindSidecarsKeepsTheRawFilesSidecar(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, "card/DSC05501.JPG", "card/DSC05501.xmp")
	writeTIFF(t, filepath.Join(root, "card", "DSC05501.ARW"))
	owned, warnings := FindSidecars(osFS{}, []photo.Photo{{Path: filepath.Join(root, "card", "DSC05501.JPG")}})
	if len(owned) != 0 || len(warnings) != 0 {
		t.Errorf("owned = %v, warnings = %v: the sidecar should stay with the raw file", owned, warnings)
	}
}

// The movie of a Live Photo is not an image, so it does not share the
// photo's sidecar.
func TestFindSidecarsIgnoresLivePhotoMovies(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, "iphone/IMG_2033.HEIC", "iphone/IMG_2033.MOV", "iphone/IMG_2033.AAE")
	heic := filepath.Join(root, "iphone", "IMG_2033.HEIC")
	owned, warnings := FindSidecars(osFS{}, []photo.Photo{{Path: heic}})
	if len(owned[heic]) != 1 || len(warnings) != 0 {
		t.Errorf("owned = %v, warnings = %v, want the AAE with the HEIC", owned, warnings)
	}
}
