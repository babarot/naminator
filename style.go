package main

import (
	"os"

	"charm.land/lipgloss/v2"
)

// The styles follow gh-infra's: the terminal's own ANSI colors, so they suit
// its theme.
var (
	Green   lipgloss.Style
	Red     lipgloss.Style
	Yellow  lipgloss.Style
	Cyan    lipgloss.Style
	Blue    lipgloss.Style
	Magenta lipgloss.Style
	Bold    lipgloss.Style
	Dim     lipgloss.Style
)

func init() {
	enableStyles()
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		disableStyles()
	}
}

func enableStyles() {
	Green = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	Red = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	Yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	Cyan = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	Blue = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	Magenta = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	Bold = lipgloss.NewStyle().Bold(true)
	Dim = lipgloss.NewStyle().Faint(true)
}

// disableStyles makes every style plain text, for NO_COLOR and for output
// that is not a terminal.
func disableStyles() {
	Green = lipgloss.NewStyle()
	Red = lipgloss.NewStyle()
	Yellow = lipgloss.NewStyle()
	Cyan = lipgloss.NewStyle()
	Blue = lipgloss.NewStyle()
	Magenta = lipgloss.NewStyle()
	Bold = lipgloss.NewStyle()
	Dim = lipgloss.NewStyle()
}

const (
	iconSuccess  = "✓"
	iconError    = "✗"
	iconChange   = "~"
	iconRemove   = "-"
	iconQueued   = "·"
	iconEllipsis = "…"
)
