package main

import (
	"runtime"
	"strings"
	"testing"

	"github.com/baoyudu/perch/internal/config"
	"github.com/baoyudu/perch/internal/index"
	"github.com/baoyudu/perch/internal/tui"
)

func cliCfg() *config.Config {
	return &config.Config{Defaults: config.Defaults{CodexApp: config.CodexCLI}}
}

func desktopCfg() *config.Config {
	return &config.Config{Defaults: config.Defaults{CodexApp: config.CodexDesktop}}
}

func proj() index.Project {
	return index.Project{
		Path:           "/Users/x/Code/My Proj",
		Name:           "My Proj",
		LastAgent:      config.ActionCodex,
		CodexSessionID: "01a0946d-414c-7bc3-a1be-245c8b6aba33",
	}
}

func TestBuildCommandCodexCLIUnchanged(t *testing.T) {
	got := buildCommand(cliCfg(), &tui.Result{Project: proj(), Action: tui.ActCodex})
	if got != "codex" {
		t.Errorf("fresh codex = %q, want %q", got, "codex")
	}
	got = buildCommand(cliCfg(), &tui.Result{Project: proj(), Action: tui.ActResume})
	if got != "codex resume 01a0946d-414c-7bc3-a1be-245c8b6aba33" {
		t.Errorf("resume = %q", got)
	}
}

func TestBuildCommandCodexDesktop(t *testing.T) {
	opener := "open"
	if runtime.GOOS != "darwin" {
		opener = "xdg-open"
	}
	got := buildCommand(desktopCfg(), &tui.Result{Project: proj(), Action: tui.ActCodex})
	want := opener + " 'codex://threads/new?path=%2FUsers%2Fx%2FCode%2FMy+Proj'"
	if got != want {
		t.Errorf("desktop new:\n got %q\nwant %q", got, want)
	}
	got = buildCommand(desktopCfg(), &tui.Result{Project: proj(), Action: tui.ActResume})
	want = opener + " codex://threads/01a0946d-414c-7bc3-a1be-245c8b6aba33"
	if got != want {
		t.Errorf("desktop resume:\n got %q\nwant %q", got, want)
	}
}

// A project the desktop app has never seen has no session id, so resume has
// to degrade to opening the directory rather than a bogus thread URL.
func TestBuildCommandDesktopResumeWithoutSessionFallsBackToPath(t *testing.T) {
	p := proj()
	p.CodexSessionID = ""
	got := buildCommand(desktopCfg(), &tui.Result{Project: p, Action: tui.ActResume})
	if !strings.Contains(got, "threads/new?path=") {
		t.Errorf("want a new-thread link, got %q", got)
	}
}

// The desktop app takes a deep link, so CLI-only codex_args must not leak
// into the open command.
func TestBuildCommandDesktopIgnoresCodexArgs(t *testing.T) {
	cfg := desktopCfg()
	cfg.Defaults.CodexArgs = []string{"--model", "gpt-5"}
	got := buildCommand(cfg, &tui.Result{Project: proj(), Action: tui.ActCodex})
	if strings.Contains(got, "--model") {
		t.Errorf("codex_args leaked into the desktop link: %q", got)
	}
}

// Claude is untouched by the codex setting.
func TestBuildCommandClaudeUnaffected(t *testing.T) {
	p := proj()
	p.LastAgent = config.ActionClaude
	got := buildCommand(desktopCfg(), &tui.Result{Project: p, Action: tui.ActResume})
	if got != "claude --continue" {
		t.Errorf("claude resume = %q", got)
	}
}
