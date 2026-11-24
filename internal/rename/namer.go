package rename

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/babarot/naminator/internal/photo"
)

// Namer gives each photo its path: the template rendered under the output
// base. Photos are given their paths in the order they were taken, which
// numbers {seq}.
type Namer struct {
	tpl Template
	// Output is the directory the template's path starts from.
	Output func(photo.Photo) string
	// Input is the directory {dir} is relative to.
	Input func(photo.Photo) string
	seq   map[string]int
}

// Target returns the path photo is to be renamed to, before it is reserved
// against the other photos and existing files (Reserver).
func (n *Namer) Target(photo photo.Photo) string {
	dir := ""
	if rel, err := filepath.Rel(n.Input(photo), photo.Dir); err == nil && rel != "." {
		dir = rel
	}
	path := filepath.Join(n.Output(photo), n.tpl.render(photo, dir))
	if strings.Contains(path, seqMark) {
		if n.seq == nil {
			n.seq = map[string]int{}
		}
		n.seq[path]++
		path = strings.ReplaceAll(path, seqMark, fmt.Sprintf("%03d", n.seq[path]))
	}
	return path
}

// NewNamer returns the namer for tpl over args, the files and directories
// given. Both bases are the directory an argument is in (for a directory
// given, its parent); destDir, when set, is the output base instead.
func NewNamer(tpl Template, args []string, destDir string) *Namer {
	input := func(p photo.Photo) string { return ArgParent(args, p.Path) }
	output := input
	if destDir != "" {
		output = func(photo.Photo) string { return destDir }
	}
	return &Namer{tpl: tpl, Output: output, Input: input}
}

// ArgParent returns the directory the argument path came from is in: for a
// directory given, its parent; for a file given, its directory.
func ArgParent(args []string, path string) string {
	abs := AbsPath(path)
	best := ""
	for _, arg := range args {
		a := AbsPath(arg)
		if abs == a || strings.HasPrefix(abs, a+string(filepath.Separator)) {
			// the longest match, for arguments inside one another
			if len(arg) > len(best) {
				best = arg
			}
		}
	}
	if best == "" || AbsPath(best) == abs {
		return filepath.Dir(path)
	}
	return filepath.Dir(filepath.Clean(best))
}

// LegacyFlags are the flags -t, -e, -E and -d.
type LegacyFlags struct {
	ByDate, ByExt, ExtFirst bool
	DestDir                 string
}

// NewLegacyNamer returns the namer for the flags -t, -e, -E and -d, as they
// have always named photos: the date and time in the photo's directory, or
// in the directory above it (-t, -e) or the destination (-d), in a
// directory per date and extension.
func NewLegacyNamer(f LegacyFlags) *Namer {
	format := "{date}_{time}"
	switch {
	case f.ByDate && f.ByExt && f.ExtFirst:
		format = "{ext}/{date}/{date}_{time}"
	case f.ByDate && f.ByExt:
		format = "{date}/{ext}/{date}_{time}"
	case f.ByDate:
		format = "{date}/{date}_{time}"
	case f.ByExt:
		format = "{ext}/{date}_{time}"
	}
	output := func(p photo.Photo) string {
		switch {
		case f.DestDir != "":
			return f.DestDir
		case f.ByDate || f.ByExt:
			return filepath.Dir(p.Dir)
		}
		return p.Dir
	}
	return &Namer{tpl: MustParseTemplate(format), Output: output, Input: func(p photo.Photo) string { return p.Dir }}
}
