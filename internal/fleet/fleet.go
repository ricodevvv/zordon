// Package fleet is the orchestration layer: it turns a task into a ranger with
// its own branch, worktree and agent process, and manages it through to a merge.
package fleet

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ricodevvv/zordon/internal/config"
	"github.com/ricodevvv/zordon/internal/git"
	"github.com/ricodevvv/zordon/internal/runner"
	"github.com/ricodevvv/zordon/internal/state"
)

// Fleet is every ranger of a single repository.
type Fleet struct {
	Repo   *git.Repo
	Config *config.Config
	Store  *state.Store

	runner runner.Runner
}

// Ranger pairs stored ranger data with its live diff against the base branch.
type Ranger struct {
	*state.Ranger
	Stat git.Stat
}

// Open loads the fleet of the repository containing dir.
func Open(dir string) (*Fleet, error) {
	repo, err := git.Open(dir)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(repo.Root)
	if err != nil {
		return nil, err
	}
	store, err := state.Load(repo.Root)
	if err != nil {
		return nil, err
	}
	run, err := runner.New(cfg.Runner)
	if err != nil {
		return nil, err
	}
	return &Fleet{Repo: repo, Config: cfg, Store: store, runner: run}, nil
}

// SummonOptions describes the ranger to create.
type SummonOptions struct {
	Task  string
	Agent string
	Name  string
	Base  string
}

// Summon creates a branch and worktree for the task and starts an agent in it.
func (f *Fleet) Summon(opts SummonOptions) (*state.Ranger, error) {
	if strings.TrimSpace(opts.Task) == "" {
		return nil, fmt.Errorf("a task description is required")
	}
	agentName, agent, err := f.Config.Agent(opts.Agent)
	if err != nil {
		return nil, err
	}

	name := opts.Name
	if name == "" {
		name = Slug(opts.Task)
	}
	name = f.uniqueName(name)

	base := opts.Base
	if base == "" {
		base = f.Config.BaseBranch
	}
	if base == "" {
		if base, err = f.Repo.DefaultBranch(); err != nil {
			return nil, err
		}
	}

	branch := f.Config.BranchPrefix + name
	worktree := filepath.Join(f.Repo.Root, f.Config.WorktreeDir, name)
	if err := f.Repo.AddWorktree(worktree, branch, base); err != nil {
		return nil, err
	}

	logDir := filepath.Join(f.Repo.Root, state.Dir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}
	logPath := filepath.Join(logDir, name+".log")

	handle, err := f.runner.Start(runner.Spec{
		Name:    name,
		Dir:     worktree,
		Command: runner.Expand(agent.Command, map[string]string{"task": opts.Task, "name": name}),
		Env:     agent.Env,
		LogPath: logPath,
	})
	if err != nil {
		_ = f.Repo.RemoveWorktree(worktree, true)
		_ = f.Repo.DeleteBranch(branch, true)
		return nil, err
	}

	ranger := &state.Ranger{
		Name:      name,
		Task:      opts.Task,
		Agent:     agentName,
		Branch:    branch,
		Base:      base,
		Worktree:  worktree,
		Runner:    string(f.Config.Runner),
		PID:       handle.PID,
		Session:   handle.Session,
		LogPath:   logPath,
		Status:    state.StatusRunning,
		CreatedAt: time.Now(),
	}
	f.Store.Put(ranger)
	return ranger, f.Store.Save()
}

// Refresh reconciles stored statuses with the agent processes actually running.
func (f *Fleet) Refresh() error {
	changed := false
	for _, r := range f.Store.List() {
		if !r.Status.Active() {
			continue
		}
		if f.runner.Alive(r) {
			continue
		}
		now := time.Now()
		r.FinishedAt = &now
		if code, ok := runner.ExitCode(r); ok {
			r.ExitCode = code
			r.Status = state.StatusDone
			if code != 0 {
				r.Status = state.StatusFailed
			}
		} else {
			r.Status = state.StatusStopped
		}
		changed = true
	}
	if !changed {
		return nil
	}
	return f.Store.Save()
}

// List returns every ranger together with its current diff against its base.
func (f *Fleet) List() []Ranger {
	stored := f.Store.List()
	out := make([]Ranger, 0, len(stored))
	for _, r := range stored {
		stat, _ := f.Repo.WorktreeStat(r.Worktree, r.Base)
		out = append(out, Ranger{Ranger: r, Stat: stat})
	}
	return out
}

// Get returns one ranger by name.
func (f *Fleet) Get(name string) (*state.Ranger, error) {
	r, ok := f.Store.Get(name)
	if !ok {
		return nil, fmt.Errorf("no ranger named %q", name)
	}
	return r, nil
}

// Stop terminates a ranger's agent, leaving its branch and worktree in place.
func (f *Fleet) Stop(name string) error {
	r, err := f.Get(name)
	if err != nil {
		return err
	}
	if err := f.runner.Stop(r); err != nil {
		return err
	}
	now := time.Now()
	r.FinishedAt = &now
	r.Status = state.StatusStopped
	return f.Store.Save()
}

// Dismiss stops a ranger and removes its worktree and branch.
func (f *Fleet) Dismiss(name string, force bool) error {
	r, err := f.Get(name)
	if err != nil {
		return err
	}
	if r.Status.Active() && !force {
		return fmt.Errorf("ranger %q is still running; stop it first or pass --force", name)
	}
	if err := f.runner.Stop(r); err != nil {
		return err
	}
	if err := f.Repo.RemoveWorktree(r.Worktree, true); err != nil {
		return fmt.Errorf("remove worktree: %w", err)
	}
	if err := f.Repo.DeleteBranch(r.Branch, true); err != nil && r.Status != state.StatusMerged {
		return fmt.Errorf("delete branch: %w", err)
	}
	_ = os.Remove(r.LogPath)
	_ = os.Remove(r.LogPath + ".exit")

	f.Store.Remove(name)
	return f.Store.Save()
}

// MergeOptions controls how a ranger's work lands on its base branch.
type MergeOptions struct {
	Squash  bool
	Dismiss bool
	Force   bool
}

// Merge commits whatever the agent left uncommitted, merges the ranger's branch
// into its base, and optionally tears the ranger down afterwards.
func (f *Fleet) Merge(name string, opts MergeOptions) error {
	r, err := f.Get(name)
	if err != nil {
		return err
	}
	if r.Status.Active() && !opts.Force {
		return fmt.Errorf("ranger %q is still running; stop it first or pass --force", name)
	}
	if _, err := f.Repo.Commit(r.Worktree, "zordon("+r.Name+"): "+r.Task); err != nil {
		return fmt.Errorf("commit pending changes: %w", err)
	}

	stat, err := f.Repo.Stat(r.Base, r.Branch)
	if err != nil {
		return err
	}
	if stat.Commits == 0 {
		return fmt.Errorf("ranger %q produced no commits", name)
	}
	if err := f.Repo.Merge(r.Base, r.Branch, opts.Squash); err != nil {
		return err
	}

	r.Status = state.StatusMerged
	if r.FinishedAt == nil {
		now := time.Now()
		r.FinishedAt = &now
	}
	if err := f.Store.Save(); err != nil {
		return err
	}
	if opts.Dismiss {
		return f.Dismiss(name, true)
	}
	return nil
}

// Diff returns the patch a ranger has produced against its base branch,
// including work it has not committed yet.
func (f *Fleet) Diff(name string) (string, error) {
	r, err := f.Get(name)
	if err != nil {
		return "", err
	}
	return f.Repo.WorktreeDiff(r.Worktree, r.Base)
}

// Stat reports what a ranger has produced so far.
func (f *Fleet) Stat(name string) (git.Stat, error) {
	r, err := f.Get(name)
	if err != nil {
		return git.Stat{}, err
	}
	return f.Repo.WorktreeStat(r.Worktree, r.Base)
}

// Logs returns the last n lines of a ranger's output, or all of it when n <= 0.
func (f *Fleet) Logs(name string, n int) (string, error) {
	r, err := f.Get(name)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(r.LogPath)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if n <= 0 {
		return string(raw), nil
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n"), nil
}

// Attach hands the terminal over to a ranger's agent session.
func (f *Fleet) Attach(name string) error {
	r, err := f.Get(name)
	if err != nil {
		return err
	}
	return f.runner.Attach(r)
}

func (f *Fleet) uniqueName(base string) string {
	name := base
	for i := 2; f.Store.Has(name) || f.Repo.BranchExists(f.Config.BranchPrefix+name); i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	return name
}

// Slug turns a task description into a short branch-safe name.
func Slug(task string) string {
	const maxLen = 32

	var b strings.Builder
	lastDash := true
	for _, r := range strings.ToLower(task) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > maxLen {
		slug = slug[:maxLen]
		if i := strings.LastIndexByte(slug, '-'); i > 0 {
			slug = slug[:i]
		}
	}
	if slug == "" {
		slug = "ranger"
	}
	return slug
}
