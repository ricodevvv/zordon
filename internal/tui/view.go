package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	headerHeight = 3
	// detailChromeHeight covers the detail pane's separator and tab line.
	detailChromeHeight = 2
	footerHeight       = 3
	// minDetailHeight is the detail pane's share of a cramped terminal.
	minDetailHeight = 3
	// maxListedRangers caps the ranger list so the detail pane keeps its room.
	maxListedRangers = 8
)

var (
	styleLogo     = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	styleMuted    = lipgloss.NewStyle().Foreground(ColorMuted)
	styleSelected = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	styleError    = lipgloss.NewStyle().Foreground(ColorFailed)
	styleOK       = lipgloss.NewStyle().Foreground(ColorDone)
	styleKey      = lipgloss.NewStyle().Foreground(ColorPrimary)
	styleRule     = lipgloss.NewStyle().Foreground(ColorMuted)
	styleAdded    = lipgloss.NewStyle().Foreground(ColorAdded)
	styleRemoved  = lipgloss.NewStyle().Foreground(ColorRemoved)
	styleTab      = lipgloss.NewStyle().Foreground(ColorMuted)
	styleTabOn    = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
)

// View implements tea.Model.
func (m *Model) View() string {
	if !m.ready {
		return "summoning…"
	}
	return strings.Join([]string{
		m.header(),
		m.list(),
		m.detail(),
		m.footer(),
	}, "\n")
}

func (m *Model) header() string {
	running, done, failed := m.counts()
	summary := fmt.Sprintf("%d running · %d idle · %d failed", running, done, failed)

	left := styleLogo.Render("ZORDON") + styleMuted.Render("  command center")
	right := styleMuted.Render(summary)
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return "\n" + left + strings.Repeat(" ", gap) + right + "\n" + rule(m.width)
}

func (m *Model) list() string {
	if len(m.rangers) == 0 {
		return styleMuted.Render("  No rangers. Press n to summon one.")
	}

	nameWidth := clamp(m.width/5, 12, 28)
	taskWidth := max(10, m.width-nameWidth-38)
	start, end, indicator := m.listWindow()

	rows := make([]string, 0, end-start+1)
	for i := start; i < end; i++ {
		r := m.rangers[i]
		cursor := "  "
		name := Pad(r.Name, nameWidth)
		if i == m.cursor {
			cursor = styleSelected.Render("▸ ")
			name = styleSelected.Render(name)
		}
		rows = append(rows, strings.Join([]string{
			cursor + StatusStyle(r.Status).Render(StatusIcon(r.Status)),
			name,
			Pad(diffBadge(r.Stat.Insertions, r.Stat.Deletions), 14),
			styleMuted.Render(Pad(FormatDuration(r.Duration()), 7)),
			styleMuted.Render(Truncate(r.Task, taskWidth)),
		}, " "))
	}
	if indicator {
		rows = append(rows, styleMuted.Render(fmt.Sprintf("  … %d more", len(m.rangers)-(end-start))))
	}
	return strings.Join(rows, "\n")
}

func (m *Model) detail() string {
	tabs := []string{tab("output", m.pane == paneLogs), tab("diff", m.pane == paneDiff)}
	title := "  " + strings.Join(tabs, styleMuted.Render(" · "))
	if r := m.selected(); r != nil {
		title += styleMuted.Render("  " + r.Branch)
	}
	return rule(m.width) + "\n" + title + "\n" + m.viewport.View()
}

func (m *Model) footer() string {
	switch m.mode {
	case modeSummon:
		return rule(m.width) + "\n  " + m.input.View() + "\n" + styleMuted.Render("  enter to summon · esc to cancel")
	case modeConfirm:
		return rule(m.width) + "\n  " + m.confirm.prompt + "\n" + styleMuted.Render("  y to confirm · any other key to cancel")
	}

	keys := []string{
		key("n", "summon"), key("m", "merge"), key("s", "stop"),
		key("x", "dismiss"), key("a", "attach"), key("tab", "output/diff"), key("q", "quit"),
	}
	status := ""
	switch {
	case m.err != nil:
		status = styleError.Render("  " + m.err.Error())
	case m.message != "":
		status = styleOK.Render("  " + m.message)
	}
	return rule(m.width) + "\n  " + strings.Join(keys, styleMuted.Render(" · ")) + "\n" + status
}

func tab(label string, active bool) string {
	if active {
		return styleTabOn.Render(label)
	}
	return styleTab.Render(label)
}

func key(k, label string) string {
	return styleKey.Render(k) + styleMuted.Render(" "+label)
}

func rule(width int) string {
	if width < 1 {
		width = 1
	}
	return styleRule.Render(strings.Repeat("─", width))
}

func diffBadge(added, removed int) string {
	if added == 0 && removed == 0 {
		return styleMuted.Render("—")
	}
	return styleAdded.Render(fmt.Sprintf("+%d", added)) + " " + styleRemoved.Render(fmt.Sprintf("-%d", removed))
}

// highlightDiff colours patch lines; plain log output is returned untouched.
func highlightDiff(content string, isDiff bool) string {
	if !isDiff {
		return content
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"), strings.HasPrefix(line, "diff "):
			lines[i] = styleMuted.Render(line)
		case strings.HasPrefix(line, "@@"):
			lines[i] = styleKey.Render(line)
		case strings.HasPrefix(line, "+"):
			lines[i] = styleAdded.Render(line)
		case strings.HasPrefix(line, "-"):
			lines[i] = styleRemoved.Render(line)
		}
	}
	return strings.Join(lines, "\n")
}
