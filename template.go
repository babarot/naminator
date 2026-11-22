package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// A template is the path a photo is renamed to, as in
// "{ext}/{date}/{date}_{time}". Each {part} is replaced with the photo's
// value, "/" separates directories, and "." and the extension are added at
// the end. The path is under a base directory (see namer).
type template struct {
	raw   string
	parts []templatePart
}

// templatePart is literal text, or a placeholder when key is set.
type templatePart struct {
	text string
	key  string
}

// placeholders are the parts a template can use.
var placeholders = []string{"date", "time", "year", "month", "day", "hour", "min", "sec", "orig", "ext", "camera", "seq", "dir"}

// seqMark stands for {seq} until the number is known: photos whose paths
// are the same but for it are numbered in the order they were taken.
const seqMark = "\x00"

// parseTemplate parses s, and returns an error saying what to fix when it
// cannot be used.
func parseTemplate(s string) (template, error) {
	t := template{raw: s}
	switch {
	case strings.TrimSpace(s) == "":
		return t, errors.New("the format is empty")
	case strings.HasPrefix(s, "/"):
		return t, errors.New("the format starts with /: it is a path under the base")
	case strings.HasSuffix(s, "/"):
		return t, errors.New("the format ends with /: the file name is missing")
	}
	if slices.Contains(strings.Split(s, "/"), "..") {
		return t, errors.New("the format goes above the base (..)")
	}
	rest := s
	for rest != "" {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			t.parts = append(t.parts, templatePart{text: rest})
			break
		}
		if open > 0 {
			t.parts = append(t.parts, templatePart{text: rest[:open]})
		}
		end := strings.IndexByte(rest[open:], '}')
		if end < 0 {
			return t, errors.New("a placeholder is not closed: add }")
		}
		key := rest[open+1 : open+end]
		if !slices.Contains(placeholders, key) {
			return t, fmt.Errorf("unknown placeholder {%s}: use {%s}", key, strings.Join(placeholders, "} {"))
		}
		t.parts = append(t.parts, templatePart{key: key})
		rest = rest[open+end+1:]
	}
	return t, nil
}

// mustParseTemplate is parseTemplate for the built-in templates.
func mustParseTemplate(s string) template {
	t, err := parseTemplate(s)
	if err != nil {
		panic(err)
	}
	return t
}

func (t template) String() string { return t.raw }

// render returns the path for photo with "." and its extension, {seq} left
// as seqMark. dir is the photo's directory relative to the input base, for
// {dir}.
func (t template) render(photo Photo, dir string) string {
	var b strings.Builder
	for _, p := range t.parts {
		if p.key == "" {
			b.WriteString(p.text)
			continue
		}
		b.WriteString(photo.value(p.key, dir))
	}
	return b.String() + "." + photo.Extension
}

// value is the photo's value of a placeholder.
func (p Photo) value(key, dir string) string {
	at := p.CreatedAt
	switch key {
	case "date":
		return at.Format("2006-01-02")
	case "time":
		return at.Format("15-04-05")
	case "year":
		return at.Format("2006")
	case "month":
		return at.Format("01")
	case "day":
		return at.Format("02")
	case "hour":
		return at.Format("15")
	case "min":
		return at.Format("04")
	case "sec":
		return at.Format("05")
	case "orig":
		return strings.TrimSuffix(p.Name, filepath.Ext(p.Name))
	case "ext":
		return p.Extension
	case "camera":
		if p.Camera == "" {
			return "unknown"
		}
		// a path segment: no separators or spaces
		return strings.NewReplacer("/", "_", " ", "_").Replace(p.Camera)
	case "seq":
		return seqMark
	case "dir":
		return dir
	}
	return ""
}

// presets are the built-in templates by name, in the order the interactive
// screen goes through them. The flags -t, -e and -E name the ones marked.
var presets = []struct {
	name   string
	format string
	flags  string // the flags it stands for
}{
	{"date and time", "{dir}/{date}_{time}", ""},
	{"by date", "{date}/{date}_{time}", "-t"},
	{"by extension", "{ext}/{date}_{time}", "-e"},
	{"by date, then extension", "{date}/{ext}/{date}_{time}", "-t -e"},
	{"by extension, then date", "{ext}/{date}/{date}_{time}", "-t -e -E"},
	{"keep the original name", "{date}/{time}_{orig}", ""},
	{"numbered by camera", "{camera}/{date}_{seq}", ""},
}

// namer gives each photo its path: the template rendered under the output
// base. Photos are given their paths in the order they were taken, which
// numbers {seq}.
type namer struct {
	tpl template
	// output is the directory the template's path starts from.
	output func(Photo) string
	// input is the directory {dir} is relative to.
	input func(Photo) string
	seq   map[string]int
}

// target returns the path photo is to be renamed to, before it is reserved
// against the other photos and existing files (pathReserver).
func (n *namer) target(photo Photo) string {
	dir := ""
	if rel, err := filepath.Rel(n.input(photo), photo.Dir); err == nil && rel != "." {
		dir = rel
	}
	path := filepath.Join(n.output(photo), n.tpl.render(photo, dir))
	if strings.Contains(path, seqMark) {
		if n.seq == nil {
			n.seq = map[string]int{}
		}
		n.seq[path]++
		path = strings.ReplaceAll(path, seqMark, fmt.Sprintf("%03d", n.seq[path]))
	}
	return path
}

// newNamer returns the namer for tpl over args, the files and directories
// given. Both bases are the directory an argument is in (for a directory
// given, its parent); destDir, when set, is the output base instead.
func newNamer(tpl template, args []string, destDir string) *namer {
	input := func(p Photo) string { return argParent(args, p.Path) }
	output := input
	if destDir != "" {
		output = func(Photo) string { return destDir }
	}
	return &namer{tpl: tpl, output: output, input: input}
}

// argParent returns the directory the argument path came from is in: for a
// directory given, its parent; for a file given, its directory.
func argParent(args []string, path string) string {
	abs := absPath(path)
	best := ""
	for _, arg := range args {
		a := absPath(arg)
		if abs == a || strings.HasPrefix(abs, a+string(filepath.Separator)) {
			// the longest match, for arguments inside one another
			if len(arg) > len(best) {
				best = arg
			}
		}
	}
	if best == "" || absPath(best) == abs {
		return filepath.Dir(path)
	}
	return filepath.Dir(filepath.Clean(best))
}

// legacyNamer returns the namer for the flags -t, -e, -E and -d, as they
// have always named photos: the date and time in the photo's directory, or
// in the directory above it (-t, -e) or the destination (-d), in a
// directory per date and extension.
func legacyNamer(opt Option) *namer {
	format := "{date}_{time}"
	switch {
	case opt.GroupByDate && opt.GroupByExt && opt.GroupExtFirst:
		format = "{ext}/{date}/{date}_{time}"
	case opt.GroupByDate && opt.GroupByExt:
		format = "{date}/{ext}/{date}_{time}"
	case opt.GroupByDate:
		format = "{date}/{date}_{time}"
	case opt.GroupByExt:
		format = "{ext}/{date}_{time}"
	}
	output := func(p Photo) string {
		switch {
		case opt.DestDir != "":
			return opt.DestDir
		case opt.GroupByDate || opt.GroupByExt:
			return filepath.Dir(p.Dir)
		}
		return p.Dir
	}
	return &namer{tpl: mustParseTemplate(format), output: output, input: func(p Photo) string { return p.Dir }}
}
