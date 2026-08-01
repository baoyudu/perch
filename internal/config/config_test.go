package config

import (
	"os"
	"path/filepath"
	"testing"
)

func setupDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PSW_CONFIG_DIR", dir)
	return dir
}

func TestLoadDefaults(t *testing.T) {
	setupDir(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.Action != ActionCD || cfg.Defaults.Command != "p" {
		t.Errorf("unexpected defaults: %+v", cfg.Defaults)
	}
	if len(cfg.Ignore) == 0 {
		t.Error("default ignore patterns should apply")
	}
}

func TestLoadFile(t *testing.T) {
	dir := setupDir(t)
	toml := `
ignore = []

[defaults]
action = "claude"
claude_args = ["--verbose"]

[projects."/work/demo"]
action = "claude"
args = ["--dangerously-skip-permissions"]
pinned = true
`
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ActionFor("/work/demo") != ActionClaude || cfg.ActionFor("/other") != ActionClaude {
		t.Error("ActionFor should honor project then default")
	}
	if len(cfg.Ignore) != 0 {
		t.Error("explicit empty ignore should override defaults")
	}
	args := cfg.AgentArgs("/work/demo", ActionClaude)
	want := []string{"--verbose", "--dangerously-skip-permissions"}
	if len(args) != len(want) || args[0] != want[0] || args[1] != want[1] {
		t.Errorf("AgentArgs = %v, want %v", args, want)
	}
	if got := cfg.AgentArgs("/work/demo", ActionCodex); len(got) != 0 {
		t.Errorf("codex args should be empty, got %v", got)
	}
	if !cfg.Pinned("/work/demo") {
		t.Error("TOML pin should be honored")
	}
}

func TestPinStatePersistsAndOverrides(t *testing.T) {
	dir := setupDir(t)
	toml := `[projects."/work/demo"]
pinned = true
`
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetPinned("/work/demo", false); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetPinned("/work/other", true); err != nil {
		t.Fatal(err)
	}
	cfg2, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Pinned("/work/demo") {
		t.Error("state unpin should override TOML pin")
	}
	if !cfg2.Pinned("/work/other") {
		t.Error("state pin should persist across loads")
	}
}
