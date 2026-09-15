package runner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/ricodevvv/zordon/internal/state"
)

// tmuxRunner keeps each agent in its own tmux session, so interactive agents get
// a real terminal and can be attached to at any time.
type tmuxRunner struct{}

func (t *tmuxRunner) Name() string { return "tmux" }

// Available reports whether tmux is installed.
func Available(kind string) bool {
	if kind != "tmux" {
		return true
	}
	_, err := exec.LookPath("tmux")
	return err == nil
}

func (t *tmuxRunner) Start(spec Spec) (Handle, error) {
	if _, err := exec.LookPath("tmux"); err != nil {
		return Handle{}, fmt.Errorf("runner is tmux but tmux is not installed")
	}
	session := "zordon-" + spec.Name

	args := append([]string{"new-session", "-d", "-s", session, "-c", spec.Dir, "/bin/sh", "-c", wrapper, "zordon"}, spec.Command...)
	cmd := exec.Command("tmux", args...)
	cmd.Env = environ(spec)
	if out, err := cmd.CombinedOutput(); err != nil {
		return Handle{}, fmt.Errorf("tmux new-session: %s", strings.TrimSpace(string(out)))
	}

	// Mirror the pane into the ranger's log so `zordon logs` works for tmux too.
	pipe := exec.Command("tmux", "pipe-pane", "-o", "-t", session, "cat >> "+spec.LogPath)
	_ = pipe.Run()

	return Handle{Session: session}, nil
}

func (t *tmuxRunner) Alive(r *state.Ranger) bool {
	if r.Session == "" {
		return false
	}
	return exec.Command("tmux", "has-session", "-t", r.Session).Run() == nil
}

func (t *tmuxRunner) Stop(r *state.Ranger) error {
	if r.Session == "" {
		return nil
	}
	if !t.Alive(r) {
		return nil
	}
	return exec.Command("tmux", "kill-session", "-t", r.Session).Run()
}

func (t *tmuxRunner) Attach(r *state.Ranger) error {
	if !t.Alive(r) {
		return fmt.Errorf("ranger %q has no live tmux session", r.Name)
	}
	path, err := exec.LookPath("tmux")
	if err != nil {
		return err
	}
	verb := "attach-session"
	if os.Getenv("TMUX") != "" {
		verb = "switch-client"
	}
	return syscall.Exec(path, []string{"tmux", verb, "-t", r.Session}, os.Environ())
}
