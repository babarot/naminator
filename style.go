package main

import "github.com/charmbracelet/lipgloss"

var (
	spinnerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Margin(1, 0)
	dotStyle      = helpStyle.UnsetMargins()
	durationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFDD0")) // FFF9B1
	dryrunStyle   = dotStyle
	okStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#BBE896"))
	warnStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFC999"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#D94F70"))
	underStyle    = lipgloss.NewStyle().Underline(true)
	appStyle      = lipgloss.NewStyle().Margin(1, 2, 0, 2)
)
