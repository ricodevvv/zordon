package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultAgent != "claude" {
		t.Errorf("DefaultAgent = %q, want claude", cfg.DefaultAgent)
	}
	if cfg.Runner != RunnerExec {
		t.Errorf("Runner = %q, want %q", cfg.Runner, RunnerExec)
	}
	if cfg.Path != "" {
		t.Errorf("Path = %q, want empty for defaulted config", cfg.Path)
	}
}

func TestLoadFillsInMissingFields(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `
agents:
  mine:
    command: ["echo", "{{task}}"]
default_agent: mine
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(cfg.Agents); got != 1 {
		t.Fatalf("len(Agents) = %d, want 1", got)
	}
	if cfg.BranchPrefix != "zordon/" {
		t.Errorf("BranchPrefix = %q, want the default", cfg.BranchPrefix)
	}
	if cfg.WorktreeDir != ".zordon/worktrees" {
		t.Errorf("WorktreeDir = %q, want the default", cfg.WorktreeDir)
	}
	if cfg.Path != filepath.Join(dir, FileName) {
		t.Errorf("Path = %q, want the config file path", cfg.Path)
	}
}

func TestLoadDefaultsAgentWhenUnset(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "agents:\n  zeta:\n    command: [\"true\"]\n  alpha:\n    command: [\"true\"]\n")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultAgent != "alpha" {
		t.Errorf("DefaultAgent = %q, want the first agent by name", cfg.DefaultAgent)
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	tests := map[string]string{
		"unknown default agent": "agents:\n  a:\n    command: [\"true\"]\ndefault_agent: b\n",
		"empty command":         "agents:\n  a:\n    command: []\ndefault_agent: a\n",
		"unknown runner":        "agents:\n  a:\n    command: [\"true\"]\ndefault_agent: a\nrunner: screen\n",
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, body)
			if _, err := Load(dir); err == nil {
				t.Fatal("Load succeeded, want an error")
			}
		})
	}
}

func TestAgentResolution(t *testing.T) {
	cfg := Default()

	name, agent, err := cfg.Agent("")
	if err != nil {
		t.Fatalf("Agent(\"\"): %v", err)
	}
	if name != cfg.DefaultAgent || len(agent.Command) == 0 {
		t.Errorf("Agent(\"\") = %q, want the default agent", name)
	}
	if _, _, err := cfg.Agent("nope"); err == nil {
		t.Error("Agent(\"nope\") succeeded, want an error")
	}
}

func TestWriteRefusesToClobber(t *testing.T) {
	dir := t.TempDir()
	if _, err := Default().Write(dir); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := Default().Write(dir); err == nil {
		t.Fatal("second Write succeeded, want an error")
	}
	if _, err := Load(dir); err != nil {
		t.Fatalf("Load of written config: %v", err)
	}
}

func write(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
