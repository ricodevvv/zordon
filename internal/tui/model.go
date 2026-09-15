// Package tui renders the Zordon dashboard: every ranger, its diff, and its
// output, in one screen.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ricodevvv/zordon/internal/fleet"
	"github.com/ricodevvv/zordon/internal/state"
)

type mode int

const (
	modeList mode = iota
	modeSummon
	modeConfirm
)

type pane int

const (
	paneLogs pane = iota
	paneDiff
)

type tickMsg time.Time

type loadedMsg struct {
	rangers []fleet.Ranger
	detail  string
	err     error
}

type confirmation struct {
	prompt string
	action func() error
}

// Model is the dashboard state.
type Model struct {
	fleet *fleet.Fleet

	rangers []fleet.Ranger
	cursor  int
	pane    pane
	mode    mode

	viewport viewport.Model
	input    textinput.Model
	confirm  confirmation

	width, height int
	ready         bool
	message       string
	err           error

	// attach is set when the user asked to attach to a ranger; Run acts on it
	// after the dashboard has released the terminal.
	attach string
}

// New builds a dashboard for a fleet.
func New(f *fleet.Fleet) *Model {
	input := textinput.New()
	input.Placeholder = "what should the ranger do?"
	input.CharLimit = 240

	return &Model{fleet: f, input: input}
}

// Run starts the dashboard and blocks until the user quits.
func Run(f *fleet.Fleet) error {
	model := New(f)
	final, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}
	if m, ok := final.(*Model); ok && m.attach != "" {
		return f.Attach(m.attach)
	}
	return nil
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.reload(), tick())
}

func tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) reload() tea.Cmd {
	return func() tea.Msg {
		if err := m.fleet.Refresh(); err != nil {
			return loadedMsg{err: err}
		}
		rangers := m.fleet.List()

		detail := ""
		if len(rangers) > 0 {
			index := m.cursor
			if index >= len(rangers) {
				index = len(rangers) - 1
			}
			detail = m.loadDetail(rangers[index].Name)
		}
		return loadedMsg{rangers: rangers, detail: detail}
	}
}

func (m *Model) loadDetail(name string) string {
	if m.pane == paneDiff {
		patch, err := m.fleet.Diff(name)
		if err != nil {
			return "diff unavailable: " + err.Error()
		}
		if strings.TrimSpace(patch) == "" {
			return "No changes yet."
		}
		return patch
	}
	out, err := m.fleet.Logs(name, 500)
	if err != nil {
		return "logs unavailable: " + err.Error()
	}
	if strings.TrimSpace(out) == "" {
		return "No output yet."
	}
	return out
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.reload(), tick())

	case loadedMsg:
		m.err = msg.err
		if msg.err == nil {
			rowsChanged := len(m.rangers) != len(msg.rangers)
			m.rangers = msg.rangers
			if rowsChanged {
				m.layout()
			}
			if m.cursor >= len(m.rangers) {
				m.cursor = max(0, len(m.rangers)-1)
			}
			atBottom := m.viewport.AtBottom()
			m.viewport.SetContent(highlightDiff(msg.detail, m.pane == paneDiff))
			if atBottom && m.pane == paneLogs {
				m.viewport.GotoBottom()
			}
		}
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case modeSummon:
			return m.updateSummon(msg)
		case modeConfirm:
			return m.updateConfirm(msg)
		default:
			return m.updateList(msg)
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.move(-1)
		return m, m.reload()
	case "down", "j":
		m.move(1)
		return m, m.reload()
	case "tab":
		m.pane = 1 - m.pane
		m.message = ""
		return m, m.reload()
	case "n":
		m.mode = modeSummon
		m.input.SetValue("")
		m.input.Focus()
		return m, textinput.Blink
	case "r":
		m.message = "refreshed"
		return m, m.reload()
	case "a":
		if r := m.selected(); r != nil {
			m.attach = r.Name
			return m, tea.Quit
		}
	case "s":
		if r := m.selected(); r != nil {
			name := r.Name
			return m, m.perform("stopped "+name, func() error { return m.fleet.Stop(name) })
		}
	case "m":
		if r := m.selected(); r != nil {
			name, branch, base := r.Name, r.Branch, r.Base
			m.ask(fmt.Sprintf("merge %s into %s?", branch, base), func() error {
				return m.fleet.Merge(name, fleet.MergeOptions{Force: true})
			})
		}
	case "x":
		if r := m.selected(); r != nil {
			name := r.Name
			m.ask(fmt.Sprintf("dismiss %s, deleting its worktree and branch?", name), func() error {
				return m.fleet.Dismiss(name, true)
			})
		}
	case "g":
		m.viewport.GotoTop()
	case "G":
		m.viewport.GotoBottom()
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *Model) updateSummon(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeList
		m.input.Blur()
		return m, nil
	case "enter":
		task := strings.TrimSpace(m.input.Value())
		m.mode = modeList
		m.input.Blur()
		if task == "" {
			return m, nil
		}
		return m, m.perform("summoned a ranger", func() error {
			_, err := m.fleet.Summon(fleet.SummonOptions{Task: task})
			return err
		})
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		action := m.confirm.action
		m.mode = modeList
		return m, m.perform("done", action)
	default:
		m.mode = modeList
		m.message = "cancelled"
		return m, nil
	}
}

func (m *Model) perform(success string, action func() error) tea.Cmd {
	if err := action(); err != nil {
		m.err = err
		m.message = ""
	} else {
		m.err = nil
		m.message = success
	}
	return m.reload()
}

func (m *Model) ask(prompt string, action func() error) {
	m.mode = modeConfirm
	m.confirm = confirmation{prompt: prompt, action: action}
}

func (m *Model) move(delta int) {
	if len(m.rangers) == 0 {
		return
	}
	m.cursor = clamp(m.cursor+delta, 0, len(m.rangers)-1)
	m.message = ""
}

func (m *Model) selected() *fleet.Ranger {
	if m.cursor < 0 || m.cursor >= len(m.rangers) {
		return nil
	}
	return &m.rangers[m.cursor]
}

func (m *Model) layout() {
	height := m.detailHeight()
	if !m.ready {
		m.viewport = viewport.New(m.width, height)
		m.ready = true
		return
	}
	m.viewport.Width = m.width
	m.viewport.Height = height
}

// listHeight is how many lines the ranger list occupies: one per ranger, capped
// both by maxListedRangers and by what the terminal can spare.
func (m *Model) listHeight() int {
	desired := len(m.rangers)
	if desired == 0 {
		return 1
	}
	if desired > maxListedRangers {
		// One extra line reports the rangers that did not fit.
		desired = maxListedRangers + 1
	}
	available := m.height - headerHeight - detailChromeHeight - footerHeight - minDetailHeight
	if available < 1 {
		available = 1
	}
	return min(desired, available)
}

// listWindow is the slice of rangers the list shows, and whether the remaining
// ones are reported on a trailing line.
func (m *Model) listWindow() (start, end int, indicator bool) {
	rows := m.listHeight()
	if len(m.rangers) <= rows {
		return 0, len(m.rangers), false
	}
	if rows > 1 {
		rows--
		indicator = true
	}
	start = clamp(m.cursor-rows/2, 0, len(m.rangers)-rows)
	return start, start + rows, indicator
}

// detailHeight is the space left for the detail pane once every other part of
// the screen has taken its own.
func (m *Model) detailHeight() int {
	return max(minDetailHeight, m.height-m.listHeight()-headerHeight-detailChromeHeight-footerHeight)
}

func (m *Model) counts() (running, done, failed int) {
	for _, r := range m.rangers {
		switch r.Status {
		case state.StatusRunning:
			running++
		case state.StatusFailed:
			failed++
		default:
			done++
		}
	}
	return running, done, failed
}

func clamp(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
