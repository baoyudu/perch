// Package index merges agent histories into a single ranked project list.
package index

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/baoyudu/perch/internal/config"
	"github.com/baoyudu/perch/internal/source"
)

const (
	AgentClaude = "claude"
	AgentCodex  = "codex"
)

type Project struct {
	Path       string    `json:"path"`
	Name       string    `json:"name"`
	LastUsed   time.Time `json:"last_used"`
	LastAgent  string    `json:"last_agent,omitempty"`
	ClaudeLast time.Time `json:"claude_last,omitzero"`
	CodexLast  time.Time `json:"codex_last,omitzero"`
	Pinned     bool      `json:"pinned,omitempty"`

	ClaudeSessionFile string `json:"-"`
	CodexSessionFile  string `json:"-"`
	CodexSessionID    string `json:"-"`
}

// cacheDir follows XDG (~/.cache/perch); PSW_CACHE_DIR still works
// for pre-rename setups.
func cacheDir() string {
	if d := os.Getenv("PERCH_CACHE_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("PSW_CACHE_DIR"); d != "" {
		return d
	}
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "perch")
}

func loadCodexCache() *source.CodexCache {
	c := &source.CodexCache{}
	if data, err := os.ReadFile(filepath.Join(cacheDir(), "codex-index.json")); err == nil {
		_ = json.Unmarshal(data, c)
	}
	return c
}

func saveCodexCache(c *source.CodexCache) {
	dir := cacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	if data, err := json.Marshal(c); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "codex-index.json"), data, 0o644)
	}
}

// Build loads both sources, merges by path, filters ignored and vanished
// directories, applies pins, and sorts pinned-first by recency.
func Build(cfg *config.Config) ([]Project, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	byPath := map[string]*Project{}
	get := func(path string) *Project {
		if p, ok := byPath[path]; ok {
			return p
		}
		p := &Project{Path: path, Name: displayName(path, home)}
		byPath[path] = p
		return p
	}

	claudeProjects, claudeErr := source.LoadClaude(home)
	for _, cp := range claudeProjects {
		p := get(cp.Path)
		p.ClaudeLast = cp.LastUsed
		p.ClaudeSessionFile = cp.SessionFile
	}

	cache := loadCodexCache()
	codexSessions, codexErr := source.LoadCodex(home, cache)
	if codexErr == nil {
		saveCodexCache(cache)
	}
	for cwd, s := range codexSessions {
		p := get(cwd)
		p.CodexLast = s.Time
		p.CodexSessionFile = s.File
		p.CodexSessionID = s.ID
	}

	if claudeErr != nil && codexErr != nil {
		return nil, fmt.Errorf("no usable history: claude: %v; codex: %v", claudeErr, codexErr)
	}

	out := make([]Project, 0, len(byPath))
	for _, p := range byPath {
		if p.CodexLast.After(p.ClaudeLast) {
			p.LastUsed, p.LastAgent = p.CodexLast, AgentCodex
		} else if !p.ClaudeLast.IsZero() {
			p.LastUsed, p.LastAgent = p.ClaudeLast, AgentClaude
		}
		if Ignored(cfg.Ignore, p.Path) || !dirExists(p.Path) {
			continue
		}
		p.Pinned = cfg.Pinned(p.Path)
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		if !out[i].LastUsed.Equal(out[j].LastUsed) {
			return out[i].LastUsed.After(out[j].LastUsed)
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}

// Ignored reports whether path matches any of the glob patterns. Patterns
// use doublestar syntax; a trailing "/**" also matches the directory itself.
func Ignored(patterns []string, path string) bool {
	for _, pat := range patterns {
		if ok, _ := doublestar.Match(pat, path); ok {
			return true
		}
		// "**/.worktrees/**" should ignore ".../.worktrees/x" itself, whose
		// path has no trailing segment; retry against path + "/".
		if ok, _ := doublestar.Match(pat, path+"/"); ok {
			return true
		}
	}
	return false
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func displayName(path, home string) string {
	if path == home {
		return "~"
	}
	return filepath.Base(path)
}

// TildePath abbreviates home to ~ for display.
func TildePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rel, err := filepath.Rel(home, path); err == nil && !filepath.IsAbs(rel) && rel != ".." && !hasDotDotPrefix(rel) {
		return "~/" + rel
	}
	return path
}

func hasDotDotPrefix(rel string) bool {
	return rel == ".." || len(rel) >= 3 && rel[:3] == "../"
}

// RelTime renders a compact relative timestamp: "2h", "3d", "5w".
func RelTime(t, now time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 12*30*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
	}
}

var ErrNoProjects = errors.New("no projects found in Claude Code or Codex history")
