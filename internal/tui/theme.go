package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/ricodevvv/zordon/internal/state"
)

// Palette is Zordon's colour scheme, readable on both light and dark terminals.
var (
	ColorPrimary = lipgloss.AdaptiveColor{Light: "#5B3DF5", Dark: "#A78BFA"}
	ColorRunning = lipgloss.AdaptiveColor{Light: "#0369A1", Dark: "#38BDF8"}
	ColorDone    = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"}
	ColorFailed  = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
	ColorStopped = lipgloss.AdaptiveColor{Light: "#A16207", Dark: "#FBBF24"}
	ColorMuted   = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9CA3AF"}
	ColorAdded   = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"}
	ColorRemoved = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
)

// StatusStyle is the colour a status is rendered in.
func StatusStyle(s state.Status) lipgloss.Style {
	switch s {
	case state.StatusRunning:
		return lipgloss.NewStyle().Foreground(ColorRunning)
	case state.StatusDone, state.StatusMerged:
		return lipgloss.NewStyle().Foreground(ColorDone)
	case state.StatusFailed:
		return lipgloss.NewStyle().Foreground(ColorFailed)
	default:
		return lipgloss.NewStyle().Foreground(ColorStopped)
	}
}

// StatusIcon is the single-width glyph shown next to a ranger.
func StatusIcon(s state.Status) string {
	switch s {
	case state.StatusRunning:
		return "●"
	case state.StatusDone:
		return "✓"
	case state.StatusMerged:
		return "⇡"
	case state.StatusFailed:
		return "✗"
	default:
		return "○"
	}
}

// FormatDuration renders an elapsed time in a fixed, compact form.
func FormatDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd%02dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}

// Truncate shortens s to width, ending with an ellipsis when it was cut.
func Truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

// Pad right-pads s with spaces to exactly width, truncating when too long.
func Pad(s string, width int) string {
	s = Truncate(s, width)
	for lipgloss.Width(s) < width {
		s += " "
	}
	return s
}
