// Package config loads and validates the per-repository Zordon configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileName is the configuration file Zordon looks for at the repository root.
const FileName = ".zordon.yml"

// Runner identifies how a ranger's agent process is executed.
type Runner string

const (
	// RunnerExec runs the agent as a detached child process writing to a log file.
	RunnerExec Runner = "exec"
	// RunnerTmux runs the agent inside a tmux session that can be attached to.
	RunnerTmux Runner = "tmux"
)

// Agent describes one invocable coding agent.
type Agent struct {
	// Command is the argv used to launch the agent. The placeholder {{task}} is
	// replaced with the task description, {{name}} with the ranger name.
	Command []string `yaml:"command"`
	// Env holds extra environment variables set for this agent only.
	Env map[string]string `yaml:"env,omitempty"`
	// Description is shown in `zordon agents`.
	Description string `yaml:"description,omitempty"`
}

// Config is the resolved configuration for a repository.
type Config struct {
	Agents       map[string]Agent `yaml:"agents"`
	DefaultAgent string           `yaml:"default_agent"`
	BaseBranch   string           `yaml:"base_branch"`
	BranchPrefix string           `yaml:"branch_prefix"`
	WorktreeDir  string           `yaml:"worktree_dir"`
	Runner       Runner           `yaml:"runner"`

	// Path is the file this config was read from, empty when defaulted.
	Path string `yaml:"-"`
}

// Default returns the configuration used when the repository has no config file.
func Default() *Config {
	return &Config{
		Agents: map[string]Agent{
			"claude": {
				Command:     []string{"claude", "-p", "{{task}}"},
				Description: "Claude Code, headless prompt mode",
			},
			"codex": {
				Command:     []string{"codex", "exec", "{{task}}"},
				Description: "Codex CLI, non-interactive exec mode",
			},
			"aider": {
				Command:     []string{"aider", "--yes", "--message", "{{task}}"},
				Description: "Aider, single message mode",
			},
		},
		DefaultAgent: "claude",
		BaseBranch:   "",
		BranchPrefix: "zordon/",
		WorktreeDir:  ".zordon/worktrees",
		Runner:       RunnerExec,
	}
}

// Load reads the configuration from root, falling back to defaults when the file
// does not exist. Missing fields are filled in from Default.
func Load(root string) (*Config, error) {
	path := filepath.Join(root, FileName)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Default(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.Path = path
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	d := Default()
	if len(c.Agents) == 0 {
		c.Agents = d.Agents
	}
	if c.DefaultAgent == "" {
		if _, ok := c.Agents[d.DefaultAgent]; ok {
			c.DefaultAgent = d.DefaultAgent
		} else {
			c.DefaultAgent = firstKey(c.Agents)
		}
	}
	if c.BranchPrefix == "" {
		c.BranchPrefix = d.BranchPrefix
	}
	if c.WorktreeDir == "" {
		c.WorktreeDir = d.WorktreeDir
	}
	if c.Runner == "" {
		c.Runner = d.Runner
	}
}

// Validate reports configuration errors that would break ranger creation.
func (c *Config) Validate() error {
	if len(c.Agents) == 0 {
		return fmt.Errorf("no agents defined")
	}
	for name, agent := range c.Agents {
		if len(agent.Command) == 0 {
			return fmt.Errorf("agent %q has an empty command", name)
		}
	}
	if _, ok := c.Agents[c.DefaultAgent]; !ok {
		return fmt.Errorf("default_agent %q is not defined under agents", c.DefaultAgent)
	}
	switch c.Runner {
	case RunnerExec, RunnerTmux:
	default:
		return fmt.Errorf("runner %q is not one of %q, %q", c.Runner, RunnerExec, RunnerTmux)
	}
	return nil
}

// Agent resolves an agent by name, or the default agent when name is empty.
func (c *Config) Agent(name string) (string, Agent, error) {
	if name == "" {
		name = c.DefaultAgent
	}
	agent, ok := c.Agents[name]
	if !ok {
		return "", Agent{}, fmt.Errorf("unknown agent %q (available: %s)", name, strings.Join(c.AgentNames(), ", "))
	}
	return name, agent, nil
}

// AgentNames returns the configured agent names in sorted order.
func (c *Config) AgentNames() []string {
	names := make([]string, 0, len(c.Agents))
	for name := range c.Agents {
		names = append(names, name)
	}
	sortStrings(names)
	return names
}

// Write saves the configuration to root, refusing to clobber an existing file.
func (c *Config) Write(root string) (string, error) {
	path := filepath.Join(root, FileName)
	if _, err := os.Stat(path); err == nil {
		return path, fmt.Errorf("%s already exists", FileName)
	}
	raw, err := yaml.Marshal(c)
	if err != nil {
		return path, err
	}
	header := "# Zordon configuration. Docs: https://github.com/ricodevvv/zordon\n"
	if err := os.WriteFile(path, append([]byte(header), raw...), 0o644); err != nil {
		return path, err
	}
	return path, nil
}

func firstKey(m map[string]Agent) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortStrings(keys)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
