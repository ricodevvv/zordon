package fleet

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ricodevvv/zordon/internal/config"
	"github.com/ricodevvv/zordon/internal/state"
)

func TestSlug(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Fix the flaky auth test", "fix-the-flaky-auth-test"},
		{"  Add /health endpoint!  ", "add-health-endpoint"},
		{"MIGRATE to Postgres 16", "migrate-to-postgres-16"},
		{"!!!", "ranger"},
		{"", "ranger"},
		{"refactor the entire billing subsystem end to end", "refactor-the-entire-billing"},
	}
	for _, tc := range tests {
		if got := Slug(tc.in); got != tc.want {
			t.Errorf("Slug(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSummonRunsAgentInItsOwnWorktree(t *testing.T) {
	f := newTestFleet(t)

	ranger, err := f.Summon(SummonOptions{Task: "write the greeting"})
	if err != nil {
		t.Fatalf("Summon: %v", err)
	}
	if ranger.Name != "write-the-greeting" {
		t.Errorf("Name = %q, want a slug of the task", ranger.Name)
	}
	if ranger.Branch != "zordon/write-the-greeting" {
		t.Errorf("Branch = %q, want the prefixed slug", ranger.Branch)
	}

	waitForExit(t, f, ranger.Name)
	if got := mustGet(t, f, ranger.Name).Status; got != state.StatusDone {
		t.Fatalf("Status = %q, want %q", got, state.StatusDone)
	}

	body, err := os.ReadFile(filepath.Join(ranger.Worktree, "agent.txt"))
	if err != nil {
		t.Fatalf("agent output missing from the worktree: %v", err)
	}
	if strings.TrimSpace(string(body)) != "write the greeting" {
		t.Errorf("agent.txt = %q, want the task text", strings.TrimSpace(string(body)))
	}
	if _, err := os.Stat(filepath.Join(f.Repo.Root, "agent.txt")); !os.IsNotExist(err) {
		t.Error("the agent's file leaked into the main worktree")
	}
}

func TestSummonThenMergeLandsTheWork(t *testing.T) {
	f := newTestFleet(t)

	ranger, err := f.Summon(SummonOptions{Task: "write the greeting"})
	if err != nil {
		t.Fatalf("Summon: %v", err)
	}
	waitForExit(t, f, ranger.Name)

	list := f.List()
	if len(list) != 1 {
		t.Fatalf("len(List()) = %d, want 1", len(list))
	}
	if list[0].Stat.Insertions != 1 || list[0].Stat.Files != 1 {
		t.Errorf("Stat = %+v, want the agent's uncommitted new file to be counted", list[0].Stat)
	}
	if patch, err := f.Diff(ranger.Name); err != nil || !strings.Contains(patch, "write the greeting") {
		t.Errorf("Diff = %q, %v; want the uncommitted new file to appear", patch, err)
	}

	if err := f.Merge(ranger.Name, MergeOptions{}); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.Repo.Root, "agent.txt")); err != nil {
		t.Errorf("merged file missing from the main worktree: %v", err)
	}
	if got := mustGet(t, f, ranger.Name).Status; got != state.StatusMerged {
		t.Errorf("Status = %q, want %q", got, state.StatusMerged)
	}
}

func TestMergeRefusesWhileRunning(t *testing.T) {
	f := newTestFleet(t)

	ranger, err := f.Summon(SummonOptions{Task: "sleep a while", Agent: "slow"})
	if err != nil {
		t.Fatalf("Summon: %v", err)
	}
	defer f.Dismiss(ranger.Name, true)

	if err := f.Merge(ranger.Name, MergeOptions{}); err == nil {
		t.Fatal("Merge on a running ranger succeeded, want an error")
	}
	if err := f.Dismiss(ranger.Name, false); err == nil {
		t.Fatal("Dismiss on a running ranger succeeded without --force, want an error")
	}
}

func TestFailingAgentIsReportedAsFailed(t *testing.T) {
	f := newTestFleet(t)

	ranger, err := f.Summon(SummonOptions{Task: "break things", Agent: "failing"})
	if err != nil {
		t.Fatalf("Summon: %v", err)
	}
	waitForExit(t, f, ranger.Name)

	got := mustGet(t, f, ranger.Name)
	if got.Status != state.StatusFailed {
		t.Errorf("Status = %q, want %q", got.Status, state.StatusFailed)
	}
	if got.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", got.ExitCode)
	}
}

func TestSummonDisambiguatesNames(t *testing.T) {
	f := newTestFleet(t)

	first, err := f.Summon(SummonOptions{Task: "same task"})
	if err != nil {
		t.Fatalf("Summon: %v", err)
	}
	second, err := f.Summon(SummonOptions{Task: "same task"})
	if err != nil {
		t.Fatalf("second Summon: %v", err)
	}
	waitForExit(t, f, first.Name)
	waitForExit(t, f, second.Name)

	if first.Name == second.Name {
		t.Fatalf("both rangers are named %q", first.Name)
	}
	if second.Name != first.Name+"-2" {
		t.Errorf("second name = %q, want %q", second.Name, first.Name+"-2")
	}
}

func TestDismissRemovesWorktreeAndBranch(t *testing.T) {
	f := newTestFleet(t)

	ranger, err := f.Summon(SummonOptions{Task: "throwaway"})
	if err != nil {
		t.Fatalf("Summon: %v", err)
	}
	waitForExit(t, f, ranger.Name)

	if err := f.Dismiss(ranger.Name, false); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}
	if _, err := os.Stat(ranger.Worktree); !os.IsNotExist(err) {
		t.Error("worktree still present after Dismiss")
	}
	if f.Repo.BranchExists(ranger.Branch) {
		t.Error("branch still present after Dismiss")
	}
	if _, err := f.Get(ranger.Name); err == nil {
		t.Error("ranger still registered after Dismiss")
	}
}

func TestSummonRequiresATask(t *testing.T) {
	f := newTestFleet(t)
	if _, err := f.Summon(SummonOptions{Task: "   "}); err == nil {
		t.Fatal("Summon with a blank task succeeded, want an error")
	}
	if _, err := f.Summon(SummonOptions{Task: "ok", Agent: "missing"}); err == nil {
		t.Fatal("Summon with an unknown agent succeeded, want an error")
	}
}

func newTestFleet(t *testing.T) *Fleet {
	t.Helper()
	root := t.TempDir()

	gitRun(t, root, "init", "-b", "main")
	gitRun(t, root, "config", "user.email", "ranger@example.com")
	gitRun(t, root, "config", "user.name", "Ranger")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "initial")

	cfg := config.Default()
	cfg.BaseBranch = "main"
	cfg.Agents = map[string]config.Agent{
		"echo":    {Command: []string{"/bin/sh", "-c", `echo "$0" > agent.txt`, "{{task}}"}},
		"failing": {Command: []string{"/bin/sh", "-c", "exit 3"}},
		"slow":    {Command: []string{"/bin/sh", "-c", "sleep 30"}},
	}
	cfg.DefaultAgent = "echo"
	if _, err := cfg.Write(root); err != nil {
		t.Fatal(err)
	}

	f, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return f
}

func waitForExit(t *testing.T, f *Fleet, name string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := f.Refresh(); err != nil {
			t.Fatalf("Refresh: %v", err)
		}
		if !mustGet(t, f, name).Status.Active() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("ranger %q did not finish in time", name)
}

func mustGet(t *testing.T, f *Fleet, name string) *state.Ranger {
	t.Helper()
	r, err := f.Get(name)
	if err != nil {
		t.Fatalf("Get(%q): %v", name, err)
	}
	return r
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
