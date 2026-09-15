package runner

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ricodevvv/zordon/internal/config"
	"github.com/ricodevvv/zordon/internal/state"
)

func TestExpand(t *testing.T) {
	argv := []string{"claude", "-p", "{{task}}", "--session", "{{name}}", "--plain"}
	got := Expand(argv, map[string]string{"task": "fix the parser", "name": "fix-the-parser"})

	want := []string{"claude", "-p", "fix the parser", "--session", "fix-the-parser", "--plain"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Expand()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExpandLeavesUnknownPlaceholders(t *testing.T) {
	got := Expand([]string{"{{unknown}}"}, map[string]string{"task": "x"})
	if got[0] != "{{unknown}}" {
		t.Errorf("Expand() = %q, want the placeholder left alone", got[0])
	}
}

func TestNewRejectsUnknownRunner(t *testing.T) {
	if _, err := New(config.Runner("screen")); err == nil {
		t.Fatal("New(\"screen\") succeeded, want an error")
	}
	for _, kind := range []config.Runner{config.RunnerExec, config.RunnerTmux} {
		if _, err := New(kind); err != nil {
			t.Errorf("New(%q): %v", kind, err)
		}
	}
}

func TestExitCode(t *testing.T) {
	log := filepath.Join(t.TempDir(), "ranger.log")
	r := &state.Ranger{LogPath: log}

	if _, ok := ExitCode(r); ok {
		t.Fatal("ExitCode reported a result before the agent finished")
	}
	if err := os.WriteFile(log+".exit", []byte("7"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, ok := ExitCode(r)
	if !ok || code != 7 {
		t.Errorf("ExitCode = %d, %v; want 7, true", code, ok)
	}
}

func TestExecRunnerLifecycle(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "ranger.log")
	run := &execRunner{}

	handle, err := run.Start(Spec{
		Name:    "probe",
		Dir:     dir,
		Command: []string{"/bin/sh", "-c", "echo hello; exit 0"},
		LogPath: log,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if handle.PID <= 0 {
		t.Fatalf("Start returned PID %d, want a real process id", handle.PID)
	}

	r := &state.Ranger{Name: "probe", PID: handle.PID, LogPath: log}
	waitFor(t, func() bool { _, ok := ExitCode(r); return ok })

	if run.Alive(r) {
		t.Error("Alive() = true after the agent exited")
	}
	body, err := os.ReadFile(log)
	if err != nil || string(body) != "hello\n" {
		t.Errorf("log = %q, %v; want the agent's stdout", string(body), err)
	}
	if err := run.Attach(r); err == nil {
		t.Error("Attach on the exec runner succeeded, want an explanatory error")
	}
}

func TestExecRunnerStop(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "ranger.log")
	run := &execRunner{}

	handle, err := run.Start(Spec{Name: "sleeper", Dir: dir, Command: []string{"sleep", "60"}, LogPath: log})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	r := &state.Ranger{Name: "sleeper", PID: handle.PID, LogPath: log}
	if !run.Alive(r) {
		t.Fatal("Alive() = false right after Start")
	}
	if err := run.Stop(r); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	waitFor(t, func() bool { return !run.Alive(r) })
}

func TestStopOnAMissingProcessIsNotAnError(t *testing.T) {
	run := &execRunner{}
	if err := run.Stop(&state.Ranger{PID: 0}); err != nil {
		t.Errorf("Stop on an unstarted ranger: %v", err)
	}
}

func waitFor(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition was not met in time")
}
