// Package config loads and persists psw configuration and runtime state.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Action values a project or the global default can be set to.
const (
	ActionCD     = "cd"
	ActionClaude = "claude"
	ActionCodex  = "codex"
)

// DefaultIgnore is used when the config file does not set `ignore`.
var DefaultIgnore = []string{
	"**/.worktrees/**",
	"**/.claude/worktrees/**",
	"**/.claude-worktrees/**",
	"**/node_modules/**",
}

type Defaults struct {
	Action     string   `toml:"action"`
	ClaudeArgs []string `toml:"claude_args"`
	CodexArgs  []string `toml:"codex_args"`
	Command    string   `toml:"command"`
}

type ProjectConfig struct {
	Action string   `toml:"action"`
	Args   []string `toml:"args"`
	Pinned bool     `toml:"pinned"`
}

type Config struct {
	Defaults Defaults                 `toml:"defaults"`
	Ignore   []string                 `toml:"ignore"`
	Projects map[string]ProjectConfig `toml:"projects"`

	// state holds TUI-toggled pins, kept separate from the user's TOML so
	// saving never rewrites (and reformats) their hand-edited config.
	state statefile
	dir   string
}

type statefile struct {
	Pinned map[string]bool `json:"pinned"`
}

// Dir follows the XDG convention (~/.config/psw) on every platform, matching
// CLI-tool practice rather than macOS's Application Support.
func Dir() string {
	if d := os.Getenv("PSW_CONFIG_DIR"); d != "" {
		return d
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "psw")
}

func Path() string { return filepath.Join(Dir(), "config.toml") }

func Load() (*Config, error) {
	cfg := &Config{
		Defaults: Defaults{Action: ActionCD, Command: "p"},
		Projects: map[string]ProjectConfig{},
		dir:      Dir(),
	}
	data, err := os.ReadFile(Path())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	ignoreDefined := false
	if err == nil {
		md, err := toml.Decode(string(data), cfg)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", Path(), err)
		}
		ignoreDefined = md.IsDefined("ignore")
	}
	if !ignoreDefined {
		cfg.Ignore = DefaultIgnore
	}
	if cfg.Defaults.Action == "" {
		cfg.Defaults.Action = ActionCD
	}
	if cfg.Defaults.Command == "" {
		cfg.Defaults.Command = "p"
	}
	if cfg.Projects == nil {
		cfg.Projects = map[string]ProjectConfig{}
	}
	cfg.loadState()
	return cfg, nil
}

func (c *Config) statePath() string { return filepath.Join(c.dir, "state.json") }

func (c *Config) loadState() {
	c.state.Pinned = map[string]bool{}
	data, err := os.ReadFile(c.statePath())
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &c.state)
	if c.state.Pinned == nil {
		c.state.Pinned = map[string]bool{}
	}
}

func (c *Config) saveState() error {
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.statePath(), data, 0o644)
}

// Pinned reports the effective pin status: a TUI toggle overrides the TOML.
func (c *Config) Pinned(path string) bool {
	if v, ok := c.state.Pinned[path]; ok {
		return v
	}
	return c.Projects[path].Pinned
}

// SetPinned records a pin toggle and persists it immediately.
func (c *Config) SetPinned(path string, pinned bool) error {
	c.state.Pinned[path] = pinned
	return c.saveState()
}

// ActionFor returns the Enter-key action for a project.
func (c *Config) ActionFor(path string) string {
	if p, ok := c.Projects[path]; ok && p.Action != "" {
		return p.Action
	}
	return c.Defaults.Action
}

// AgentArgs returns the extra args for launching the given agent in a project:
// global per-agent defaults plus the project's args when its configured action
// is that same agent.
func (c *Config) AgentArgs(path, agent string) []string {
	var args []string
	switch agent {
	case ActionClaude:
		args = append(args, c.Defaults.ClaudeArgs...)
	case ActionCodex:
		args = append(args, c.Defaults.CodexArgs...)
	}
	if p, ok := c.Projects[path]; ok && p.Action == agent {
		args = append(args, p.Args...)
	}
	return args
}
