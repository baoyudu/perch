// Package config loads and persists perch configuration and runtime state.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// Action values a project or the global default can be set to.
const (
	ActionCD     = "cd"
	ActionClaude = "claude"
	ActionCodex  = "codex"
	ActionResume = "resume" // reopen the project's last agent session (cd if none)
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

// UI holds picker appearance settings.
type UI struct {
	// Icons selects the glyph set: "nerd" (default, needs a Nerd Font —
	// https://www.nerdfonts.com) or "plain" (universal Unicode).
	Icons string `toml:"icons"`
}

type Config struct {
	Defaults Defaults                 `toml:"defaults"`
	UI       UI                       `toml:"ui"`
	Ignore   []string                 `toml:"ignore"`
	Projects map[string]ProjectConfig `toml:"projects"`

	// state holds TUI-toggled pins, kept separate from the user's TOML so
	// saving never rewrites (and reformats) their hand-edited config.
	state statefile
	dir   string
}

// statefile holds runtime state that is not configuration: pin toggles and
// the first-run hint marker. Settings changed in the TUI are written into
// config.toml itself (surgically, preserving comments and formatting).
type statefile struct {
	Pinned   map[string]bool `json:"pinned"`
	HintSeen bool            `json:"seen_settings_hint,omitempty"`
}

// Dir follows the XDG convention (~/.config/perch) on every platform,
// matching CLI-tool practice rather than macOS's Application Support.
// PSW_CONFIG_DIR is honored for pre-rename setups.
func Dir() string {
	if d := os.Getenv("PERCH_CONFIG_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("PSW_CONFIG_DIR"); d != "" {
		return d
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "perch")
}

// legacyDir is the pre-rename config location (~/.config/psw).
func legacyDir() string {
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

// migrateLegacy copies config.toml and state.json from the old psw config
// dir the first time perch runs; the originals are left untouched.
func migrateLegacy() {
	newDir, oldDir := Dir(), legacyDir()
	if newDir == oldDir {
		return
	}
	if _, err := os.Stat(newDir); err == nil {
		return // perch dir already exists
	}
	if _, err := os.Stat(oldDir); err != nil {
		return // nothing to migrate
	}
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		return
	}
	for _, name := range []string{"config.toml", "state.json"} {
		data, err := os.ReadFile(filepath.Join(oldDir, name))
		if err != nil {
			continue
		}
		_ = os.WriteFile(filepath.Join(newDir, name), data, 0o644)
	}
}

func Path() string { return filepath.Join(Dir(), "config.toml") }

func Load() (*Config, error) {
	migrateLegacy()
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
	if cfg.UI.Icons != "plain" {
		cfg.UI.Icons = "nerd"
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

// SetDefaultAction writes the Enter action into config.toml and applies it.
func (c *Config) SetDefaultAction(action string) error {
	if err := c.editConfigKey("defaults", "action", action); err != nil {
		return err
	}
	c.Defaults.Action = action
	return nil
}

// SetIcons writes the icon set into config.toml and applies it.
func (c *Config) SetIcons(icons string) error {
	if err := c.editConfigKey("ui", "icons", icons); err != nil {
		return err
	}
	c.UI.Icons = icons
	return nil
}

// editConfigKey surgically sets `key = "value"` inside [section] of
// config.toml, leaving every other byte — comments, formatting, other
// sections — untouched. A missing file, section, or key is created. The
// result must survive a TOML parse or nothing is written; the write itself
// is atomic (temp file + rename).
func (c *Config) editConfigKey(section, key, value string) error {
	path := filepath.Join(c.dir, "config.toml")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var lines []string
	if len(data) > 0 {
		lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	}
	keyRe := regexp.MustCompile(`^(\s*` + regexp.QuoteMeta(key) + `\s*=\s*)("[^"]*"|'[^']*')(\s*(#.*)?)\s*$`)
	current, sectionAt, done := "", -1, false
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") {
			if end := strings.Index(t, "]"); end > 0 {
				current = strings.TrimPrefix(t[1:end], "[") // tolerate [[array]] headers
				if current == section {
					sectionAt = i
				}
			}
			continue
		}
		if done || current != section {
			continue
		}
		if m := keyRe.FindStringSubmatch(line); m != nil {
			lines[i] = m[1] + fmt.Sprintf("%q", value) + m[3]
			done = true
		}
	}
	if !done {
		entry := fmt.Sprintf("%s = %q", key, value)
		if sectionAt >= 0 {
			rest := append([]string{entry}, lines[sectionAt+1:]...)
			lines = append(lines[:sectionAt+1], rest...)
		} else {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, "["+section+"]", entry)
		}
	}
	out := strings.Join(lines, "\n") + "\n"
	if _, err := toml.Decode(out, &Config{}); err != nil {
		return fmt.Errorf("refusing to write config.toml that would not parse: %w", err)
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(out), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SettingsHintSeen reports whether the first-run "^e settings" hint has been
// dismissed (by opening settings or completing a pick).
func (c *Config) SettingsHintSeen() bool { return c.state.HintSeen }

// MarkSettingsHintSeen dismisses the first-run hint permanently.
func (c *Config) MarkSettingsHintSeen() {
	if c.state.HintSeen {
		return
	}
	c.state.HintSeen = true
	_ = c.saveState()
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
