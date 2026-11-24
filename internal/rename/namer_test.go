package rename

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/babarot/naminator/internal/photo"
)

func TestNewNamer(t *testing.T) {
	at := time.Date(2026, 10, 4, 14, 22, 9, 0, time.UTC)
	photo := func(path string) photo.Photo {
		return photo.Photo{Name: filepath.Base(path), Path: path, Dir: filepath.Dir(path), Extension: strings.ToLower(strings.TrimPrefix(filepath.Ext(path), ".")), CreatedAt: at}
	}
	tests := []struct {
		name   string
		args   []string
		dest   string
		format string
		path   string
		want   string
	}{
		// card folders copied to the desktop: the base is their parent
		{"by extension, then date", []string{"Desktop/10551007", "Desktop/10651008"}, "", "{ext}/{date}/{date}_{time}", "Desktop/10551007/DSC05501.ARW", "Desktop/arw/2026-10-04/2026-10-04_14-22-09.arw"},
		{"in place", []string{"Desktop/10551007"}, "", "{dir}/{date}_{time}", "Desktop/10551007/DSC05501.ARW", "Desktop/10551007/2026-10-04_14-22-09.arw"},
		{"folders inside", []string{"Pictures/import"}, "", "{dir}/{date}_{time}", "Pictures/import/DCIM/100MSDCF/a.jpg", "Pictures/import/DCIM/100MSDCF/2026-10-04_14-22-09.jpg"},
		{"a trailing slash", []string{"Desktop/10551007/"}, "", "{date}/{date}_{time}", "Desktop/10551007/DSC05501.ARW", "Desktop/2026-10-04/2026-10-04_14-22-09.arw"},
		// files given: the base is their directory
		{"files given", []string{"iPhone/IMG_8101.HEIC"}, "", "{dir}/{date}_{time}", "iPhone/IMG_8101.HEIC", "iPhone/2026-10-04_14-22-09.heic"},
		// -d moves the output base only: {dir} is still from the input base
		{"dest", []string{"in"}, "/out", "{date}_{time}", "in/a.jpg", "/out/2026-10-04_14-22-09.jpg"},
		{"dest with dir", []string{"in"}, "/out", "{dir}/{date}_{time}", "in/a.jpg", "/out/in/2026-10-04_14-22-09.jpg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewNamer(MustParseTemplate(tt.format), tt.args, tt.dest)
			if got := n.Target(photo(tt.path)); got != tt.want {
				t.Errorf("target = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNamerSeq(t *testing.T) {
	n := NewNamer(MustParseTemplate("{camera}/{date}_{seq}"), []string{"card"}, "")
	day := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	var got []string
	for i, camera := range []string{"A", "A", "B", "A"} {
		got = append(got, n.Target(photo.Photo{Name: fmt.Sprintf("%d.jpg", i), Path: fmt.Sprintf("card/%d.jpg", i), Dir: "card", Extension: "jpg", Camera: camera, CreatedAt: day.Add(time.Duration(i) * time.Hour)}))
	}
	// numbered per path: each camera and day from 001, in the order given
	want := []string{"A/2026-10-04_001.jpg", "A/2026-10-04_002.jpg", "B/2026-10-04_001.jpg", "A/2026-10-04_003.jpg"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("targets = %v, want %v", got, want)
	}
}
