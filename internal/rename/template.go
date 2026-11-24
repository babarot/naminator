// Package rename decides where photos go: the templates of their paths,
// the paths no two photos share, their sidecars, and the directories left
// empty. It moves nothing.
package rename

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/babarot/naminator/internal/photo"
)

// A Template is the path a photo is renamed to, as in
// "{ext}/{date}/{date}_{time}". Each {part} is replaced with the photo's
// value, "/" separates directories, and "." and the extension are added at
// the end. The path is under a base directory (see Namer).
type Template struct {
	raw   string
	parts []templatePart
}

// templatePart is literal text, or a placeholder when key is set.
type templatePart struct {
	text string
	key  string
}

// Placeholders are the parts a template can use.
var Placeholders = []string{"date", "time", "year", "month", "day", "hour", "min", "sec", "orig", "ext", "camera", "seq", "dir"}

// seqMark stands for {seq} until the number is known: photos whose paths
// are the same but for it are numbered in the order they were taken.
const seqMark = "\x00"

// ParseTemplate parses s, and returns an error saying what to fix when it
// cannot be used.
func ParseTemplate(s string) (Template, error) {
	t := Template{raw: s}
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
		if !slices.Contains(Placeholders, key) {
			return t, fmt.Errorf("unknown placeholder {%s}: use {%s}", key, strings.Join(Placeholders, "} {"))
		}
		t.parts = append(t.parts, templatePart{key: key})
		rest = rest[open+end+1:]
	}
	return t, nil
}

// MustParseTemplate is ParseTemplate for the built-in templates.
func MustParseTemplate(s string) Template {
	t, err := ParseTemplate(s)
	if err != nil {
		panic(err)
	}
	return t
}

func (t Template) String() string { return t.raw }

// render returns the path for photo with "." and its extension, {seq} left
// as seqMark. dir is the photo's directory relative to the input base, for
// {dir}.
func (t Template) render(photo photo.Photo, dir string) string {
	var b strings.Builder
	for _, p := range t.parts {
		if p.key == "" {
			b.WriteString(p.text)
			continue
		}
		b.WriteString(Value(photo, p.key, dir))
	}
	return b.String() + "." + photo.Extension
}

// Value is the photo's value of a placeholder.
func Value(p photo.Photo, key, dir string) string {
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

// Preset is a template with a name.
type Preset struct {
	Name   string
	Format string
	Flags  string // the flags it stands for, for a built-in one
}

// Presets are the built-in templates by name, in the order the interactive
// screen goes through them. The flags -t, -e and -E name the ones marked.
var Presets = []Preset{
	{"date and time", "{dir}/{date}_{time}", ""},
	{"by date", "{date}/{date}_{time}", "-t"},
	{"by extension", "{ext}/{date}_{time}", "-e"},
	{"by date, then extension", "{date}/{ext}/{date}_{time}", "-t -e"},
	{"by extension, then date", "{ext}/{date}/{date}_{time}", "-t -e -E"},
	{"keep the original name", "{date}/{time}_{orig}", ""},
	{"numbered by camera", "{camera}/{date}_{seq}", ""},
}
