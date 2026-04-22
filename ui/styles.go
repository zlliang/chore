package ui

import "github.com/charmbracelet/lipgloss"

var (
	styleHeader = lipgloss.NewStyle().Bold(true)

	styleCompleted = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true) // green
	styleFailed    = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true) // red
	styleRunning   = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true) // yellow
	styleSkipped   = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true) // gray
	stylePending   = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true) // gray
	styleVersion   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))            // gray
	styleDim       = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))            // gray, non-bold

	styleErrorLine = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))

	styleSummary = lipgloss.NewStyle().Bold(true).MarginTop(1)

	iconCompleted = styleCompleted.Render("✓")
	iconFailed    = styleFailed.Render("×")
	iconSkipped   = styleSkipped.Render("○")
	iconPending   = stylePending.Render("→")
)
