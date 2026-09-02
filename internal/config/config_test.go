package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestSetDefaultActionEditsTOMLSurgically(t *testing.T) {
	dir := setupDir(t)
	orig := `# psw config — hand-written, do not reorder
ignore = ["**/x/**"]

[defaults]
action = "cd"   # what enter does
command = "p"

[projects."/w/app"]
action = "codex"
`
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetDefaultAction("claude"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{
		"# psw config — hand-written, do not reorder", // leading comment intact
		`action = "claude"   # what enter does`,       // value swapped, inline comment kept
		`command = "p"`,                               // sibling key untouched
		`[projects."/w/app"]`,                         // other sections untouched
	} {
		if !strings.Contains(got, want) {
			t.Errorf("config.toml missing %q after edit:\n%s", want, got)
		}
	}
	if strings.Count(got, `"codex"`) != 1 {
		t.Errorf("per-project action must not be rewritten:\n%s", got)
	}
	reloaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Defaults.Action != ActionClaude {
		t.Errorf("reload: default action = %q", reloaded.Defaults.Action)
	}
	if reloaded.ActionFor("/w/app") != ActionCodex {
		t.Errorf("reload: project action = %q", reloaded.ActionFor("/w/app"))
	}
}

func TestEditConfigKeyCreatesFileAndSections(t *testing.T) {
	dir := setupDir(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetIcons("plain"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetDefaultAction("codex"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetIcons("nerd"); err != nil { // update the key it created
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Count(got, "icons") != 1 || !strings.Contains(got, `icons = "nerd"`) {
		t.Errorf("icons key should exist exactly once with the new value:\n%s", got)
	}
	reloaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.UI.Icons != "nerd" || reloaded.Defaults.Action != ActionCodex {
		t.Errorf("reload: icons=%q action=%q", reloaded.UI.Icons, reloaded.Defaults.Action)
	}
}
