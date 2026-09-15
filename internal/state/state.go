// Package state persists the fleet of rangers across Zordon invocations.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Dir is the per-repository directory holding Zordon's state and logs.
const Dir = ".zordon"

// Status is the lifecycle stage of a ranger.
type Status string

const (
	// StatusRunning means the agent process is alive.
	StatusRunning Status = "running"
	// StatusDone means the agent exited successfully.
	StatusDone Status = "done"
	// StatusFailed means the agent exited with a non-zero status.
	StatusFailed Status = "failed"
	// StatusStopped means the agent was terminated by the user.
	StatusStopped Status = "stopped"
	// StatusMerged means the ranger's branch was merged into its base.
	StatusMerged Status = "merged"
)

// Active reports whether the status represents work still in flight.
func (s Status) Active() bool { return s == StatusRunning }

// Ranger is a single agent working on one task in its own worktree.
type Ranger struct {
	Name       string     `json:"name"`
	Task       string     `json:"task"`
	Agent      string     `json:"agent"`
	Branch     string     `json:"branch"`
	Base       string     `json:"base"`
	Worktree   string     `json:"worktree"`
	Runner     string     `json:"runner"`
	PID        int        `json:"pid,omitempty"`
	Session    string     `json:"session,omitempty"`
	LogPath    string     `json:"log_path"`
	Status     Status     `json:"status"`
	ExitCode   int        `json:"exit_code"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// Duration is how long the ranger has been, or was, working.
func (r *Ranger) Duration() time.Duration {
	if r.FinishedAt != nil {
		return r.FinishedAt.Sub(r.CreatedAt)
	}
	return time.Since(r.CreatedAt)
}

// Store is the on-disk fleet registry for one repository.
type Store struct {
	mu      sync.Mutex
	path    string
	rangers map[string]*Ranger
}

type file struct {
	Version int       `json:"version"`
	Rangers []*Ranger `json:"rangers"`
}

// Load reads the store for the repository rooted at root, creating an empty one
// when no state file exists yet.
func Load(root string) (*Store, error) {
	path := filepath.Join(root, Dir, "state.json")
	s := &Store{path: path, rangers: map[string]*Ranger{}}

	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for _, r := range f.Rangers {
		s.rangers[r.Name] = r
	}
	return s, nil
}

// Get returns the ranger with that name.
func (s *Store) Get(name string) (*Ranger, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rangers[name]
	return r, ok
}

// List returns every ranger, newest first.
func (s *Store) List() []*Ranger {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Ranger, 0, len(s.rangers))
	for _, r := range s.rangers {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// Put inserts or replaces a ranger.
func (s *Store) Put(r *Ranger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rangers[r.Name] = r
}

// Remove drops a ranger from the registry.
func (s *Store) Remove(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rangers, name)
}

// Has reports whether a ranger of that name is registered.
func (s *Store) Has(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.rangers[name]
	return ok
}

// Save writes the registry atomically.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	f := file{Version: 1, Rangers: make([]*Ranger, 0, len(s.rangers))}
	for _, r := range s.rangers {
		f.Rangers = append(f.Rangers, r)
	}
	sort.Slice(f.Rangers, func(i, j int) bool { return f.Rangers[i].CreatedAt.Before(f.Rangers[j].CreatedAt) })

	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
