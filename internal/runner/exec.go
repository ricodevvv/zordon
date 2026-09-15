package runner

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/ricodevvv/zordon/internal/state"
)

// execRunner detaches the agent into its own session and captures its output to
// a log file, so the agent outlives the Zordon command that started it.
type execRunner struct{}

func (e *execRunner) Name() string { return "exec" }

func (e *execRunner) Start(spec Spec) (Handle, error) {
	log, err := os.OpenFile(spec.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return Handle{}, err
	}
	defer log.Close()

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return Handle{}, err
	}
	defer devNull.Close()

	args := append([]string{"-c", wrapper, "zordon"}, spec.Command...)
	cmd := exec.Command("/bin/sh", args...)
	cmd.Dir = spec.Dir
	cmd.Env = environ(spec)
	cmd.Stdin = devNull
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return Handle{}, fmt.Errorf("start %s: %w", spec.Command[0], err)
	}
	// Reaping keeps a long-lived Zordon process (the dashboard) from accumulating
	// zombies, which would otherwise still answer signal 0 and look alive forever.
	// When Zordon exits first the agent is simply reparented to init.
	go func() { _ = cmd.Wait() }()

	return Handle{PID: cmd.Process.Pid}, nil
}

func (e *execRunner) Alive(r *state.Ranger) bool {
	if r.PID <= 0 {
		return false
	}
	if _, finished := ExitCode(r); finished {
		return false
	}
	proc, err := os.FindProcess(r.PID)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func (e *execRunner) Stop(r *state.Ranger) error {
	if r.PID <= 0 {
		return nil
	}
	// The agent was started with Setsid, so its whole group can be signalled.
	if err := syscall.Kill(-r.PID, syscall.SIGTERM); err != nil {
		if err == syscall.ESRCH {
			return nil
		}
		return syscall.Kill(r.PID, syscall.SIGTERM)
	}
	return nil
}

func (e *execRunner) Attach(r *state.Ranger) error {
	return fmt.Errorf("the exec runner cannot be attached to; use `zordon logs -f %s`, or set runner: tmux", r.Name)
}
