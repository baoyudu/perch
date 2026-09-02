// perch — jump to recent Claude Code / Codex project
// directories and optionally launch the agent there.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/baoyudu/perch/internal/config"
	"github.com/baoyudu/perch/internal/index"
	"github.com/baoyudu/perch/internal/shell"
	"github.com/baoyudu/perch/internal/tui"
)

// Version is stamped by goreleaser via ldflags.
var Version = "dev"

const usage = `perch — switch between recent Claude Code / Codex projects

Usage:
  perch init <zsh|bash|fish>   print shell integration (eval in your rc file)
  perch pick                   interactive picker (used by the shell function)
  perch list [--json]          print the merged project list
  perch pin <path>             pin a project to the top
  perch unpin <path>           remove a pin
  perch doctor                 check data sources and integration
  perch version                print version

Setup (zsh):  add to ~/.zshrc:   eval "$(perch init zsh)"
Then run:     p
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "init":
		err = cmdInit(os.Args[2:])
	case "pick":
		err = cmdPick()
	case "list":
		err = cmdList(os.Args[2:])
	case "pin":
		err = cmdPin(os.Args[2:], true)
	case "unpin":
		err = cmdPin(os.Args[2:], false)
	case "doctor":
		err = cmdDoctor()
	case "version", "--version", "-v":
		fmt.Println("perch", Version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "perch: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "perch:", err)
		os.Exit(1)
	}
}

func cmdInit(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: perch init <zsh|bash|fish>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	script, err := shell.Init(args[0], cfg.Defaults.Command)
	if err != nil {
		return err
	}
	fmt.Print(script)
	return nil
}

func cmdPick() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	projects, err := index.Build(cfg)
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		return index.ErrNoProjects
	}
	result, err := tui.Run(cfg, projects)
	if err != nil {
		return err
	}
	if result == nil {
		os.Exit(130) // cancelled, like fzf
	}
	fmt.Printf("%s\n%s\n", result.Project.Path, buildCommand(cfg, result))
	return nil
}

// buildCommand renders the post-cd command for the shell wrapper ("-" = none).
func buildCommand(cfg *config.Config, r *tui.Result) string {
	p := r.Project
	agent := string(r.Action)
	if r.Action == tui.ActResume {
		agent = p.LastAgent
	}
	var argv []string
	switch agent {
	case config.ActionClaude:
		argv = []string{"claude"}
		if r.Action == tui.ActResume {
			argv = append(argv, "--continue")
		}
	case config.ActionCodex:
		argv = []string{"codex"}
		if r.Action == tui.ActResume {
			if p.CodexSessionID != "" {
				argv = append(argv, "resume", p.CodexSessionID)
			} else {
				argv = append(argv, "resume", "--last")
			}
		}
	default:
		return "-"
	}
	argv = append(argv, cfg.AgentArgs(p.Path, agent)...)
	return shell.Command(argv...)
}

func cmdList(args []string) error {
	asJSON := len(args) > 0 && args[0] == "--json"
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	projects, err := index.Build(cfg)
	if err != nil {
		return err
	}
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(projects)
	}
	for _, p := range projects {
		pin := " "
		if p.Pinned {
			pin = "★"
		}
		agent := p.LastAgent
		if agent == "" {
			agent = "-"
		}
		fmt.Printf("%s %s %-7s %-5s %s\n", pin, padCell(truncate(p.Name, 28), 28), agent,
			index.RelTime(p.LastUsed, time.Now()), index.TildePath(p.Path))
	}
	return nil
}

// padCell pads by display width so CJK names stay aligned.
func padCell(s string, w int) string {
	if n := w - lipgloss.Width(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

func cmdPin(args []string, pinned bool) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: perch pin|unpin <path>")
	}
	abs, err := filepath.Abs(args[0])
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.SetPinned(abs, pinned); err != nil {
		return err
	}
	state := "pinned"
	if !pinned {
		state = "unpinned"
	}
	fmt.Printf("%s %s\n", state, abs)
	return nil
}

func cmdDoctor() error {
	home, _ := os.UserHomeDir()
	ok := func(good bool, label, detail string) {
		mark := "✗"
		if good {
			mark = "✓"
		}
		fmt.Printf("  %s %-28s %s\n", mark, label, detail)
	}

	fmt.Println("data sources:")
	claudeProjects := 0
	if _, err := os.Stat(filepath.Join(home, ".claude.json")); err == nil {
		cfg, _ := config.Load()
		if cfg != nil {
			if ps, err := index.Build(cfg); err == nil {
				for _, p := range ps {
					if !p.ClaudeLast.IsZero() {
						claudeProjects++
					}
				}
			}
		}
		ok(true, "~/.claude.json", fmt.Sprintf("%d projects with Claude history", claudeProjects))
	} else {
		ok(false, "~/.claude.json", "not found — has Claude Code been used?")
	}
	sessions := filepath.Join(home, ".codex", "sessions")
	if info, err := os.Stat(sessions); err == nil && info.IsDir() {
		ok(true, "~/.codex/sessions", "present")
	} else {
		ok(false, "~/.codex/sessions", "not found — has Codex been used?")
	}

	fmt.Println("binaries:")
	for _, bin := range []string{"claude", "codex", "git"} {
		path, err := exec.LookPath(bin)
		ok(err == nil, bin, path)
	}

	fmt.Println("config:")
	if _, err := config.Load(); err != nil {
		ok(false, config.Path(), err.Error())
	} else if _, err := os.Stat(config.Path()); err == nil {
		ok(true, config.Path(), "loaded")
	} else {
		ok(true, config.Path(), "not present (defaults in use)")
	}

	fmt.Println("shell integration:")
	installed := false
	for _, rc := range []string{".zshrc", ".bashrc", ".config/fish/config.fish"} {
		data, err := os.ReadFile(filepath.Join(home, rc))
		if err != nil {
			continue
		}
		switch s := string(data); {
		case strings.Contains(s, "perch init"):
			ok(true, "~/"+rc, "perch init found")
			installed = true
		case strings.Contains(s, "psw init"):
			ok(false, "~/"+rc, `still evals psw (renamed) — change to: eval "$(perch init zsh)"`)
			installed = true
		}
	}
	if !installed {
		ok(false, "rc file", `add: eval "$(perch init zsh)"`)
	}
	return nil
}

// truncate clips to display width, so CJK text counts double-width cells.
func truncate(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > w {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}
