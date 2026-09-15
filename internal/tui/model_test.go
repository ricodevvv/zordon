package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ricodevvv/zordon/internal/fleet"
	"github.com/ricodevvv/zordon/internal/git"
	"github.com/ricodevvv/zordon/internal/state"
)

func TestViewFillsExactlyTheTerminalHeight(t *testing.T) {
	const height = 30

	for _, count := range []int{0, 1, 5, maxListedRangers, maxListedRangers + 4} {
		t.Run(fmt.Sprintf("%d rangers", count), func(t *testing.T) {
			m := newTestModel(count, 100, height)
			m.viewport.SetContent(strings.Repeat("filler\n", 200))

			if got := strings.Count(m.View(), "\n") + 1; got != height {
				t.Errorf("View() rendered %d lines, want %d", got, height)
			}
		})
	}
}

func TestViewFitsShortTerminals(t *testing.T) {
	m := newTestModel(3, 80, 12)
	if got := strings.Count(m.View(), "\n") + 1; got > 12 {
		t.Errorf("View() rendered %d lines, want no more than 12", got)
	}
}

func TestListCapsAtMaxListedRangers(t *testing.T) {
	m := newTestModel(maxListedRangers+3, 100, 40)
	lines := strings.Split(m.list(), "\n")

	if len(lines) != maxListedRangers+1 {
		t.Fatalf("list() rendered %d lines, want %d", len(lines), maxListedRangers+1)
	}
	if !strings.Contains(lines[len(lines)-1], "3 more") {
		t.Errorf("last line = %q, want a count of the hidden rangers", lines[len(lines)-1])
	}
}

func TestMoveStaysInBounds(t *testing.T) {
	m := newTestModel(3, 100, 30)

	m.move(-1)
	if m.cursor != 0 {
		t.Errorf("cursor = %d after moving up from the top, want 0", m.cursor)
	}
	m.move(10)
	if m.cursor != 2 {
		t.Errorf("cursor = %d after moving past the bottom, want 2", m.cursor)
	}
	if got := m.selected().Name; got != "ranger-2" {
		t.Errorf("selected() = %q, want ranger-2", got)
	}
}

func TestSelectedIsNilWithoutRangers(t *testing.T) {
	m := newTestModel(0, 100, 30)
	if m.selected() != nil {
		t.Error("selected() returned a ranger from an empty fleet")
	}
	m.move(1)
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
}

func TestCounts(t *testing.T) {
	m := newTestModel(0, 100, 30)
	m.rangers = []fleet.Ranger{
		{Ranger: &state.Ranger{Status: state.StatusRunning}},
		{Ranger: &state.Ranger{Status: state.StatusRunning}},
		{Ranger: &state.Ranger{Status: state.StatusFailed}},
		{Ranger: &state.Ranger{Status: state.StatusMerged}},
	}
	running, done, failed := m.counts()
	if running != 2 || done != 1 || failed != 1 {
		t.Errorf("counts() = %d, %d, %d; want 2, 1, 1", running, done, failed)
	}
}

func newTestModel(rangers, width, height int) *Model {
	m := &Model{width: width, height: height}
	for i := 0; i < rangers; i++ {
		m.rangers = append(m.rangers, fleet.Ranger{Ranger: &state.Ranger{
			Name:      fmt.Sprintf("ranger-%d", i),
			Task:      "do the thing",
			Branch:    fmt.Sprintf("zordon/ranger-%d", i),
			Status:    state.StatusRunning,
			CreatedAt: time.Now(),
		}})
	}
	m.layout()
	return m
}

func TestListWindowKeepsTheSelectionVisible(t *testing.T) {
	m := newTestModel(20, 100, 40)
	m.cursor = 19

	start, end, indicator := m.listWindow()
	if m.cursor < start || m.cursor >= end {
		t.Errorf("listWindow() = %d..%d, which hides the cursor at %d", start, end, m.cursor)
	}
	if !indicator {
		t.Error("listWindow() reported no indicator line for a truncated list")
	}
	if !strings.Contains(m.list(), "ranger-19") {
		t.Error("list() does not show the selected ranger")
	}
}

func TestListHeightYieldsToTheDetailPane(t *testing.T) {
	m := newTestModel(20, 100, 14)
	if got := m.detailHeight(); got != minDetailHeight {
		t.Errorf("detailHeight() = %d, want %d in a cramped terminal", got, minDetailHeight)
	}
	if got := strings.Count(m.list(), "\n") + 1; got != m.listHeight() {
		t.Errorf("list() rendered %d lines, want listHeight() = %d", got, m.listHeight())
	}
}

func TestListShowsTheDiffBadge(t *testing.T) {
	m := newTestModel(1, 100, 30)
	m.rangers[0].Stat = git.Stat{Files: 2, Insertions: 48, Deletions: 12}

	row := m.list()
	plain := ansi.Strip(row)

	// The badge is styled before it is padded, so a rune-counting pad would eat
	// the escape sequence and drop the counts entirely.
	if !strings.Contains(plain, "+48") || !strings.Contains(plain, "-12") {
		t.Errorf("list() = %q, want the insertion and deletion counts", plain)
	}
	if !strings.Contains(plain, "ranger-0") {
		t.Errorf("list() = %q, want the ranger name", plain)
	}
}

func TestDiffBadgeKeepsItsColumnWidth(t *testing.T) {
	for _, tc := range []struct{ added, removed int }{{0, 0}, {1, 0}, {48, 12}, {123456, 654321}} {
		badge := diffBadge(tc.added, tc.removed, diffBadgeWidth)
		if got := lipgloss.Width(badge); got < diffBadgeWidth {
			t.Errorf("width of diffBadge(%d, %d) = %d, want at least %d", tc.added, tc.removed, got, diffBadgeWidth)
		}
		if ansi.Strip(badge) == "" {
			t.Errorf("diffBadge(%d, %d) rendered nothing", tc.added, tc.removed)
		}
	}
}
