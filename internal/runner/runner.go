// Package runner starts and supervises the agent process behind each ranger.
package runner

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ricodevvv/zordon/internal/config"
	"github.com/ricodevvv/zordon/internal/state"
)

// Spec describes one agent launch.
type Spec struct {
	Name    string
	Dir     string
	Command []string
	Env     map[string]string
	LogPath string
}

// Handle identifies a started agent so it can be found again later.
type Handle struct {
	PID     int
	Session string
}

// Runner starts agents and reports on them after the Zordon process has exited.
type Runner interface {
	// Name is the identifier used in configuration.
	Name() string
	// Start launches the agent and returns a handle to it.
	Start(spec Spec) (Handle, error)
	// Alive reports whether the agent process is still running.
	Alive(r *state.Ranger) bool
	// Stop terminates the agent.
	Stop(r *state.Ranger) error
	// Attach hands the terminal over to the agent, replacing the current process.
	Attach(r *state.Ranger) error
}

// New returns the runner for the configured backend.
func New(kind config.Runner) (Runner, error) {
	switch kind {
	case config.RunnerExec:
		return &execRunner{}, nil
	case config.RunnerTmux:
		return &tmuxRunner{}, nil
	default:
		return nil, fmt.Errorf("unknown runner %q", kind)
	}
}

// Expand substitutes {{task}} and {{name}} placeholders in an agent command.
func Expand(argv []string, vars map[string]string) []string {
	out := make([]string, len(argv))
	for i, arg := range argv {
		for key, value := range vars {
			arg = strings.ReplaceAll(arg, "{{"+key+"}}", value)
		}
		out[i] = arg
	}
	return out
}

// exitPath is the file a finished agent writes its exit status to.
func exitPath(logPath string) string { return logPath + ".exit" }

// ExitCode reads the recorded exit status of a finished agent.
func ExitCode(r *state.Ranger) (int, bool) {
	raw, err := os.ReadFile(exitPath(r.LogPath))
	if err != nil {
		return 0, false
	}
	code, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		return 0, false
	}
	return code, true
}

// wrapper runs the agent, records its exit status, and is the only shell Zordon
// relies on so that agent commands never have to be quoted into a string.
const wrapper = `"$@"; printf '%s' "$?" > "$ZORDON_EXIT_FILE"`

func environ(spec Spec) []string {
	env := append(os.Environ(),
		"ZORDON_EXIT_FILE="+exitPath(spec.LogPath),
		"ZORDON_RANGER="+spec.Name,
		"ZORDON_LOG="+spec.LogPath,
	)
	for key, value := range spec.Env {
		env = append(env, key+"="+value)
	}
	return env
}
