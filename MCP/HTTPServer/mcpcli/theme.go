package main

import "github.com/charmbracelet/lipgloss"

// Minimum window dimensions enforced across the UI.
const (
	minWindowWidth  = 80
	minWindowHeight = 12
)

// Theme centralizes colors and styles to avoid scattered globals.
type Theme struct {
	ColorNormal    lipgloss.Color
	ColorHighlight lipgloss.Color
	ColorSuccess   lipgloss.Color
	ColorError     lipgloss.Color
	ColorBorder    lipgloss.Color

	HeaderActive  lipgloss.Style
	StatusSuccess lipgloss.Style
	StatusError   lipgloss.Style
	ModalBorder   lipgloss.Style
}

func NewTheme() *Theme {
	t := &Theme{
		ColorNormal:    lipgloss.Color("15"),
		ColorHighlight: lipgloss.Color("4"),
		ColorSuccess:   lipgloss.Color("2"),
		ColorError:     lipgloss.Color("1"),
		ColorBorder:    lipgloss.Color("8"),
	}

	t.HeaderActive = lipgloss.NewStyle().Bold(true).Foreground(t.ColorHighlight)
	t.StatusSuccess = lipgloss.NewStyle().Foreground(t.ColorSuccess)
	t.StatusError = lipgloss.NewStyle().Foreground(t.ColorError)
	t.ModalBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.ColorHighlight).
		Padding(1).
		Align(lipgloss.Center)

	return t
}
