// Package theme is the colors of the screens, by what they are for, and the
// styles made of them. A color is written nowhere else.
package theme

import (
	"fmt"
	"image/color"
	"os"
	"regexp"

	"charm.land/lipgloss/v2"
)

// Palette is the colors of a theme, named by what they are for.
type Palette struct {
	Success color.Color // done
	Failure color.Color // failed, removed, or to fix
	Warn    color.Color // to look at: a change, a number, a limit, a question
	Active  color.Color // going on, and where the cursor is
	Path    color.Color // directories
	Part    color.Color // the parts of a format, and its box while edited

	// CursorLine is the background of the cursor's line, and
	// CursorLineEdit while the keys go to the format.
	CursorLine     color.Color
	CursorLineEdit color.Color
}

// base is what the palettes share, as gh-infra's: the terminal's own ANSI
// colors, so they suit its theme.
var base = Palette{
	Success: lipgloss.Color("2"),
	Failure: lipgloss.Color("1"),
	Warn:    lipgloss.Color("3"),
	Active:  lipgloss.Color("6"),
	Path:    lipgloss.Color("4"),
	Part:    lipgloss.Color("5"),
}

// Dark is the palette of a dark background.
func Dark() Palette {
	p := base
	p.CursorLine, p.CursorLineEdit = lipgloss.Color("#283457"), lipgloss.Color("#222436")
	return p
}

// Light is the palette of a light background.
func Light() Palette {
	p := base
	p.CursorLine, p.CursorLineEdit = lipgloss.Color("#dbe4ff"), lipgloss.Color("#eef1fb")
	return p
}

// Styles are the styles of a palette.
type Styles struct {
	Success    lipgloss.Style
	Failure    lipgloss.Style
	Warn       lipgloss.Style
	Active     lipgloss.Style
	Path       lipgloss.Style
	Part       lipgloss.Style
	PartStrong lipgloss.Style
	Strong     lipgloss.Style
	Dim        lipgloss.Style

	// Plain is set when the styles are plain text, and the cursor's line
	// has no background.
	Plain bool

	cursorLine, cursorLineEdit color.Color
}

// New returns the styles of p.
func New(p Palette) Styles {
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	return Styles{
		Success:        fg(p.Success),
		Failure:        fg(p.Failure),
		Warn:           fg(p.Warn),
		Active:         fg(p.Active),
		Path:           fg(p.Path),
		Part:           fg(p.Part),
		PartStrong:     fg(p.Part).Bold(true),
		Strong:         lipgloss.NewStyle().Bold(true),
		Dim:            lipgloss.NewStyle().Faint(true),
		cursorLine:     p.CursorLine,
		cursorLineEdit: p.CursorLineEdit,
	}
}

// Plain returns styles that are plain text, for NO_COLOR and for output
// that is not a terminal.
func Plain() Styles {
	s := lipgloss.NewStyle()
	return Styles{Success: s, Failure: s, Warn: s, Active: s, Path: s, Part: s, PartStrong: s, Strong: s, Dim: s, Plain: true}
}

// For returns the styles of a terminal with a dark or light background,
// plain when NO_COLOR is set.
func For(dark bool) Styles {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return Plain()
	}
	if dark {
		return New(Dark())
	}
	return New(Light())
}

// resetSGR matches the resets of the styles, after which the background of
// the cursor's line is set again.
var resetSGR = regexp.MustCompile(`\x1b\[0?m`)

// CursorLine draws line on the background of the cursor's line. line is
// padded to the width it is to cover.
func (s Styles) CursorLine(line string, editing bool) string {
	c := s.cursorLine
	if editing {
		c = s.cursorLineEdit
	}
	if c == nil {
		return line
	}
	r, g, b, _ := c.RGBA()
	bg := fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r>>8, g>>8, b>>8)
	return bg + resetSGR.ReplaceAllStringFunc(line, func(reset string) string { return reset + bg }) + "\x1b[49m"
}
