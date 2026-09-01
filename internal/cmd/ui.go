package cmd

import "github.com/charmbracelet/lipgloss"

var (
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	greenStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	yellowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	redStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

func dim(s string) string    { return dimStyle.Render(s) }
func green(s string) string  { return greenStyle.Render(s) }
func yellow(s string) string { return yellowStyle.Render(s) }
func red(s string) string    { return redStyle.Render(s) }
