package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorRunning = lipgloss.Color("42")  // green
	colorFailed  = lipgloss.Color("196") // red
	colorPending = lipgloss.Color("220") // yellow
	colorMuted   = lipgloss.Color("240") // gray

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15"))





	styleRunning = lipgloss.NewStyle().Foreground(colorRunning).Bold(true)
	styleFailed  = lipgloss.NewStyle().Foreground(colorFailed).Bold(true)
	stylePending = lipgloss.NewStyle().Foreground(colorPending)
	styleMuted   = lipgloss.NewStyle().Foreground(colorMuted)

	styleHelp = lipgloss.NewStyle().
			Foreground(colorMuted).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("238"))

	styleWorkspaceBar = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("12"))

	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleDataModeLive   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))  // green
	styleDataModeCached = lipgloss.NewStyle().Foreground(lipgloss.Color("220")) // yellow
)

// StateStyle returns a lipgloss style appropriate for the given lifecycle state.
func StateStyle(state string) lipgloss.Style {
	switch state {
	case "RUNNING", "SUCCESS":
		return styleRunning
	case "FAILED", "ERROR", "INTERNAL_ERROR":
		return styleFailed
	case "PENDING", "STARTING", "DEPLOYING":
		return stylePending
	default:
		return styleMuted
	}
}
