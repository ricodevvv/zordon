package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenOutsideRepository(t *testing.T) {
	if _, err := Open(t.TempDir()); err != ErrNotARepository {
		t.Fatalf("Open outside a repo = %v, want ErrNotARepository", err)
	}
}

func TestWorktreeLifecycle(t *testing.T) {
	repo := newTestRepo(t)

	worktree := filepath.Join(t.TempDir(), "ranger")
	if err := repo.AddWorktree(worktree, "zordon/task", "main"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	if !repo.BranchExists("zordon/task") {
		t.Fatal("BranchExists(\"zordon/task\") = false after AddWorktree")
	}
	if err := repo.AddWorktree(worktree+"2", "zordon/task", "main"); err == nil {
		t.Fatal("AddWorktree on an existing branch succeeded, want an error")
	}

	trees, err := repo.Worktrees()
	if err != nil {
		t.Fatalf("Worktrees: %v", err)
	}
	if len(trees) != 2 {
		t.Fatalf("len(Worktrees()) = %d, want 2", len(trees))
	}

	if err := repo.RemoveWorktree(worktree, true); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if err := repo.DeleteBranch("zordon/task", true); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
}

func TestStatAndMerge(t *testing.T) {
	repo := newTestRepo(t)
	worktree := filepath.Join(t.TempDir(), "ranger")
	if err := repo.AddWorktree(worktree, "zordon/task", "main"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	if dirty, err := repo.IsDirty(worktree); err != nil || dirty {
		t.Fatalf("IsDirty on a fresh worktree = %v, %v; want false, nil", dirty, err)
	}
	writeFile(t, worktree, "feature.txt", "one\ntwo\n")

	committed, err := repo.Commit(worktree, "add feature")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !committed {
		t.Fatal("Commit reported nothing to commit")
	}
	if again, err := repo.Commit(worktree, "noop"); err != nil || again {
		t.Fatalf("Commit on a clean worktree = %v, %v; want false, nil", again, err)
	}

	stat, err := repo.Stat("main", "zordon/task")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if stat.Files != 1 || stat.Insertions != 2 || stat.Commits != 1 {
		t.Errorf("Stat = %+v, want 1 file, 2 insertions, 1 commit", stat)
	}

	patch, err := repo.Diff("main", "zordon/task")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if patch == "" {
		t.Error("Diff returned an empty patch")
	}

	if err := repo.Merge("main", "zordon/task", false); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo.Root, "feature.txt")); err != nil {
		t.Errorf("merged file missing from the main worktree: %v", err)
	}
}

func TestMergeRestoresOriginalBranch(t *testing.T) {
	repo := newTestRepo(t)
	worktree := filepath.Join(t.TempDir(), "ranger")
	if err := repo.AddWorktree(worktree, "zordon/task", "main"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	writeFile(t, worktree, "feature.txt", "hello\n")
	if _, err := repo.Commit(worktree, "add feature"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	gitRun(t, repo.Root, "checkout", "-b", "scratch")
	if err := repo.Merge("main", "zordon/task", true); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	branch, err := repo.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "scratch" {
		t.Errorf("CurrentBranch() = %q, want the branch checked out before the merge", branch)
	}
}

func TestDefaultBranchPrefersMain(t *testing.T) {
	repo := newTestRepo(t)
	branch, err := repo.DefaultBranch()
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	if branch != "main" {
		t.Errorf("DefaultBranch() = %q, want main", branch)
	}
}

func TestParseShortstat(t *testing.T) {
	tests := []struct {
		in                           string
		files, insertions, deletions int
	}{
		{" 3 files changed, 42 insertions(+), 7 deletions(-)\n", 3, 42, 7},
		{" 1 file changed, 1 insertion(+)\n", 1, 1, 0},
		{" 2 files changed, 5 deletions(-)\n", 2, 0, 5},
		{"", 0, 0, 0},
	}
	for _, tc := range tests {
		files, insertions, deletions := parseShortstat(tc.in)
		if files != tc.files || insertions != tc.insertions || deletions != tc.deletions {
			t.Errorf("parseShortstat(%q) = %d, %d, %d; want %d, %d, %d",
				tc.in, files, insertions, deletions, tc.files, tc.insertions, tc.deletions)
		}
	}
}

func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	dir := t.TempDir()

	gitRun(t, dir, "init", "-b", "main")
	gitRun(t, dir, "config", "user.email", "ranger@example.com")
	gitRun(t, dir, "config", "user.name", "Ranger")
	writeFile(t, dir, "README.md", "# test\n")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "initial")

	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return repo
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestWorktreeStatCountsUncommittedWork(t *testing.T) {
	repo := newTestRepo(t)
	worktree := filepath.Join(t.TempDir(), "ranger")
	if err := repo.AddWorktree(worktree, "zordon/task", "main"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	writeFile(t, worktree, "committed.txt", "a\nb\n")
	if _, err := repo.Commit(worktree, "committed work"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	writeFile(t, worktree, "README.md", "# test\nedited\n")
	writeFile(t, worktree, "brand-new.txt", "x\ny\nz\n")

	stat, err := repo.WorktreeStat(worktree, "main")
	if err != nil {
		t.Fatalf("WorktreeStat: %v", err)
	}
	if stat.Commits != 1 {
		t.Errorf("Commits = %d, want 1", stat.Commits)
	}
	if stat.Files != 3 {
		t.Errorf("Files = %d, want 3 (committed, edited, untracked)", stat.Files)
	}
	if stat.Insertions != 6 {
		t.Errorf("Insertions = %d, want 6", stat.Insertions)
	}
}

func TestWorktreeDiffIncludesUntrackedFiles(t *testing.T) {
	repo := newTestRepo(t)
	worktree := filepath.Join(t.TempDir(), "ranger")
	if err := repo.AddWorktree(worktree, "zordon/task", "main"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	writeFile(t, worktree, "brand-new.txt", "hello from the agent\n")

	patch, err := repo.WorktreeDiff(worktree, "main")
	if err != nil {
		t.Fatalf("WorktreeDiff: %v", err)
	}
	if !strings.Contains(patch, "hello from the agent") {
		t.Errorf("WorktreeDiff = %q, want the untracked file's contents", patch)
	}

	// The index must be untouched, or the agent would see staged changes.
	if dirty, err := repo.IsDirty(worktree); err != nil || !dirty {
		t.Errorf("IsDirty = %v, %v; want the file to still be untracked", dirty, err)
	}
	staged, err := run(worktree, "diff", "--cached", "--name-only")
	if err != nil || strings.TrimSpace(staged) != "" {
		t.Errorf("staged files = %q, %v; want nothing staged", staged, err)
	}
}

func TestCountLines(t *testing.T) {
	dir := t.TempDir()
	tests := map[string]struct {
		body string
		want int
	}{
		"trailing newline": {"a\nb\n", 2},
		"no newline":       {"a\nb", 2},
		"empty":            {"", 0},
		"binary":           {"a\x00b\n", 0},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name)
			writeFile(t, dir, name, tc.body)
			if got := countLines(path); got != tc.want {
				t.Errorf("countLines(%q) = %d, want %d", tc.body, got, tc.want)
			}
		})
	}
	if got := countLines(filepath.Join(dir, "missing")); got != 0 {
		t.Errorf("countLines on a missing file = %d, want 0", got)
	}
}
