package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ricodevvv/zordon/internal/fleet"
	"github.com/ricodevvv/zordon/internal/state"
	"github.com/ricodevvv/zordon/internal/tui"
)

var (
	styleOK     = lipgloss.NewStyle().Foreground(tui.ColorDone).Bold(true)
	styleName   = lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	styleDim    = lipgloss.NewStyle().Foreground(tui.ColorMuted)
	styleCmd    = lipgloss.NewStyle().Foreground(tui.ColorPrimary)
	styleHeader = lipgloss.NewStyle().Foreground(tui.ColorMuted).Bold(true)
	styleAdd    = lipgloss.NewStyle().Foreground(tui.ColorAdded)
	styleDel    = lipgloss.NewStyle().Foreground(tui.ColorRemoved)
)

func renderStatus(s state.Status) string {
	return tui.StatusStyle(s).Render(tui.StatusIcon(s) + " " + string(s))
}

func renderTable(rangers []fleet.Ranger) string {
	const (
		nameWidth   = 24
		agentWidth  = 10
		statusWidth = 9
		timeWidth   = 8
		diffWidth   = 14
	)

	var b strings.Builder
	b.WriteString(styleHeader.Render(strings.Join([]string{
		tui.Pad("RANGER", nameWidth),
		tui.Pad("AGENT", agentWidth),
		tui.Pad("STATUS", statusWidth),
		tui.Pad("ELAPSED", timeWidth),
		tui.Pad("DIFF", diffWidth),
		"TASK",
	}, "  ")))
	b.WriteByte('\n')

	for _, r := range rangers {
		status := tui.StatusStyle(r.Status).Render(tui.Pad(tui.StatusIcon(r.Status)+" "+string(r.Status), statusWidth))
		b.WriteString(strings.Join([]string{
			styleName.Render(tui.Pad(r.Name, nameWidth)),
			tui.Pad(r.Agent, agentWidth),
			status,
			styleDim.Render(tui.Pad(tui.FormatDuration(r.Duration()), timeWidth)),
			renderDiffStat(r.Stat.Insertions, r.Stat.Deletions, diffWidth),
			tui.Truncate(r.Task, 48),
		}, "  "))
		b.WriteByte('\n')
	}
	return b.String()
}

func renderDiffStat(added, removed, width int) string {
	if added == 0 && removed == 0 {
		return styleDim.Render(tui.Pad("—", width))
	}
	text := styleAdd.Render(fmt.Sprintf("+%d", added)) + " " + styleDel.Render(fmt.Sprintf("-%d", removed))
	for lipgloss.Width(text) < width {
		text += " "
	}
	return text
}
