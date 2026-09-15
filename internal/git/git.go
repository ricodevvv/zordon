// Package git wraps the git plumbing Zordon needs to manage per-ranger worktrees.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrNotARepository is returned when a command runs outside a git work tree.
var ErrNotARepository = errors.New("not inside a git repository")

// Repo is a git repository rooted at Root.
type Repo struct {
	Root string
}

// Open locates the repository containing dir.
func Open(dir string) (*Repo, error) {
	out, err := run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, ErrNotARepository
	}
	root, err := filepath.Abs(strings.TrimSpace(out))
	if err != nil {
		return nil, err
	}
	return &Repo{Root: root}, nil
}

// Stat summarises the changes a branch carries relative to its base.
type Stat struct {
	Files      int
	Insertions int
	Deletions  int
	Commits    int
}

// Worktree describes one entry of `git worktree list`.
type Worktree struct {
	Path   string
	Branch string
	Head   string
}

// CurrentBranch returns the checked out branch name, or an error on detached HEAD.
func (r *Repo) CurrentBranch() (string, error) {
	out, err := run(r.Root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return "", fmt.Errorf("HEAD is detached")
	}
	return strings.TrimSpace(out), nil
}

// DefaultBranch resolves the repository's integration branch, preferring the
// remote HEAD and falling back to main, master, then the current branch.
func (r *Repo) DefaultBranch() (string, error) {
	if out, err := run(r.Root, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if name := strings.TrimPrefix(strings.TrimSpace(out), "origin/"); name != "" {
			return name, nil
		}
	}
	for _, candidate := range []string{"main", "master"} {
		if r.BranchExists(candidate) {
			return candidate, nil
		}
	}
	return r.CurrentBranch()
}

// BranchExists reports whether a local branch of that name is present.
func (r *Repo) BranchExists(branch string) bool {
	_, err := run(r.Root, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// AddWorktree creates branch from base and checks it out at path.
func (r *Repo) AddWorktree(path, branch, base string) error {
	if r.BranchExists(branch) {
		return fmt.Errorf("branch %q already exists", branch)
	}
	if _, err := run(r.Root, "worktree", "add", "-b", branch, path, base); err != nil {
		return fmt.Errorf("create worktree: %w", err)
	}
	return nil
}

// RemoveWorktree detaches a worktree, discarding local changes when force is set.
func (r *Repo) RemoveWorktree(path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	if _, err := run(r.Root, args...); err != nil {
		// A worktree whose directory was deleted by hand only needs pruning.
		if _, pruneErr := run(r.Root, "worktree", "prune"); pruneErr == nil && !dirTracked(r.Root, path) {
			return nil
		}
		return err
	}
	return nil
}

// DeleteBranch removes a local branch.
func (r *Repo) DeleteBranch(branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := run(r.Root, "branch", flag, branch)
	return err
}

// Worktrees lists the repository's worktrees, main worktree included.
func (r *Repo) Worktrees() ([]Worktree, error) {
	out, err := run(r.Root, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var (
		list    []Worktree
		current Worktree
	)
	flush := func() {
		if current.Path != "" {
			list = append(list, current)
		}
		current = Worktree{}
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			current.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			current.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			current.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		}
	}
	flush()
	return list, nil
}

// Stat reports how far branch has diverged from base.
func (r *Repo) Stat(base, branch string) (Stat, error) {
	var stat Stat
	if !r.BranchExists(branch) {
		return stat, fmt.Errorf("branch %q does not exist", branch)
	}
	out, err := run(r.Root, "diff", "--shortstat", base+"..."+branch)
	if err != nil {
		return stat, err
	}
	stat.Files, stat.Insertions, stat.Deletions = parseShortstat(out)

	if count, err := run(r.Root, "rev-list", "--count", base+".."+branch); err == nil {
		stat.Commits, _ = strconv.Atoi(strings.TrimSpace(count))
	}
	return stat, nil
}

// Diff returns the patch branch introduces on top of base.
func (r *Repo) Diff(base, branch string) (string, error) {
	return run(r.Root, "diff", base+"..."+branch)
}

// WorktreeStat reports everything a ranger has produced in its worktree relative
// to base: committed work, uncommitted edits, and new files it has not added yet.
func (r *Repo) WorktreeStat(worktree, base string) (Stat, error) {
	var stat Stat
	ref := r.forkPoint(worktree, base)

	out, err := run(worktree, "diff", "--shortstat", ref)
	if err != nil {
		return stat, err
	}
	stat.Files, stat.Insertions, stat.Deletions = parseShortstat(out)

	untracked, err := r.untracked(worktree)
	if err != nil {
		return stat, err
	}
	for _, name := range untracked {
		stat.Files++
		stat.Insertions += countLines(filepath.Join(worktree, name))
	}
	if count, err := run(worktree, "rev-list", "--count", ref+"..HEAD"); err == nil {
		stat.Commits, _ = strconv.Atoi(strings.TrimSpace(count))
	}
	return stat, nil
}

// WorktreeDiff returns the full patch a ranger has produced, new files included,
// without touching its index.
func (r *Repo) WorktreeDiff(worktree, base string) (string, error) {
	patch, err := run(worktree, "diff", r.forkPoint(worktree, base))
	if err != nil {
		return "", err
	}
	untracked, err := r.untracked(worktree)
	if err != nil {
		return patch, err
	}

	var b strings.Builder
	b.WriteString(patch)
	for _, name := range untracked {
		// git diff --no-index exits non-zero when the files differ, which is the
		// expected outcome here, so only its output matters.
		out, _ := run(worktree, "diff", "--no-index", "--", os.DevNull, name)
		b.WriteString(out)
	}
	return b.String(), nil
}

// forkPoint resolves where a worktree's branch left base. Comparing against it
// rather than against base itself keeps a ranger's diff meaning "what this
// ranger changed" after someone else's work has already landed on base -
// otherwise every other ranger appears to delete it.
func (r *Repo) forkPoint(worktree, base string) string {
	out, err := run(worktree, "merge-base", base, "HEAD")
	if err != nil {
		return base
	}
	return strings.TrimSpace(out)
}

func (r *Repo) untracked(worktree string) ([]string, error) {
	out, err := run(worktree, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

// maxCountedFileSize bounds the work spent counting lines of a new file.
const maxCountedFileSize = 1 << 20

func countLines(path string) int {
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxCountedFileSize {
		return 0
	}
	raw, err := os.ReadFile(path)
	if err != nil || bytes.IndexByte(raw, 0) >= 0 {
		return 0
	}
	if len(raw) == 0 {
		return 0
	}
	lines := bytes.Count(raw, []byte{'\n'})
	if raw[len(raw)-1] != '\n' {
		lines++
	}
	return lines
}

// IsDirty reports whether the worktree at path has uncommitted changes.
func (r *Repo) IsDirty(path string) (bool, error) {
	out, err := run(path, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// Commit stages everything in the worktree at path and records a commit. It is a
// no-op when the worktree is clean.
func (r *Repo) Commit(path, message string) (bool, error) {
	dirty, err := r.IsDirty(path)
	if err != nil || !dirty {
		return false, err
	}
	if _, err := run(path, "add", "-A"); err != nil {
		return false, err
	}
	if _, err := run(path, "commit", "-m", message); err != nil {
		return false, err
	}
	return true, nil
}

// Merge merges branch into base from the main worktree, leaving base checked out
// as it was found.
func (r *Repo) Merge(base, branch string, squash bool) error {
	original, err := r.CurrentBranch()
	if err != nil {
		return err
	}
	if original != base {
		if _, err := run(r.Root, "checkout", base); err != nil {
			return fmt.Errorf("checkout %s: %w", base, err)
		}
		defer run(r.Root, "checkout", original)
	}

	args := []string{"merge", "--no-ff"}
	if squash {
		args = []string{"merge", "--squash"}
	}
	args = append(args, branch)
	if _, err := run(r.Root, args...); err != nil {
		return fmt.Errorf("merge %s into %s: %w", branch, base, err)
	}
	if squash {
		if _, err := run(r.Root, "commit", "-m", "Merge "+branch); err != nil {
			return err
		}
	}
	return nil
}

func dirTracked(root, path string) bool {
	out, err := run(root, "worktree", "list", "--porcelain")
	if err != nil {
		return false
	}
	return strings.Contains(out, "worktree "+path+"\n")
}

func parseShortstat(out string) (files, insertions, deletions int) {
	for _, part := range strings.Split(strings.TrimSpace(out), ",") {
		part = strings.TrimSpace(part)
		fields := strings.Fields(part)
		if len(fields) < 2 {
			continue
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		switch {
		case strings.HasPrefix(fields[1], "file"):
			files = n
		case strings.HasPrefix(fields[1], "insertion"):
			insertions = n
		case strings.HasPrefix(fields[1], "deletion"):
			deletions = n
		}
	}
	return files, insertions, deletions
}

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), errors.New(msg)
	}
	return stdout.String(), nil
}
