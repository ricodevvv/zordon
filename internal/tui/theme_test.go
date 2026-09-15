package tui

import (
	"os"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ricodevvv/zordon/internal/state"
)

// Styles collapse to plain text when the tests run without a terminal, so the
// colour-dependent assertions below pin the profile.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	os.Exit(m.Run())
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{5 * time.Second, "5s"},
		{90 * time.Second, "1m30s"},
		{2*time.Hour + 5*time.Minute, "2h05m"},
		{50 * time.Hour, "2d02h"},
	}
	for _, tc := range tests {
		if got := FormatDuration(tc.in); got != tc.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		in    string
		width int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 4, "hel…"},
		{"hello", 1, "…"},
		{"hello", 0, ""},
		{"área", 3, "ár…"},
	}
	for _, tc := range tests {
		if got := Truncate(tc.in, tc.width); got != tc.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tc.in, tc.width, got, tc.want)
		}
	}
}

func TestPadReachesExactWidth(t *testing.T) {
	for _, in := range []string{"", "ab", "a much longer string than the width", "área"} {
		if got := lipgloss.Width(Pad(in, 8)); got != 8 {
			t.Errorf("width of Pad(%q, 8) = %d, want 8", in, got)
		}
	}
}

func TestStatusIconIsSingleWidth(t *testing.T) {
	statuses := []state.Status{
		state.StatusRunning, state.StatusDone, state.StatusFailed,
		state.StatusStopped, state.StatusMerged,
	}
	for _, s := range statuses {
		if got := lipgloss.Width(StatusIcon(s)); got != 1 {
			t.Errorf("width of StatusIcon(%q) = %d, want 1", s, got)
		}
		if StatusStyle(s).GetForeground() == nil {
			t.Errorf("StatusStyle(%q) has no colour", s)
		}
	}
}

func TestHighlightDiffLeavesLogsAlone(t *testing.T) {
	logs := "line one\n+ not a diff\n"
	if got := highlightDiff(logs, false); got != logs {
		t.Errorf("highlightDiff(logs, false) = %q, want the input unchanged", got)
	}
	patch := "@@ -1 +1 @@\n+added\n-removed\n"
	if got := highlightDiff(patch, true); got == patch {
		t.Error("highlightDiff(patch, true) did not colour the patch")
	}
}
