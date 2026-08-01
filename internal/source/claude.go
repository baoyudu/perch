// Package source parses Claude Code and Codex on-disk history into project records.
package source

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ClaudeProject struct {
	Path        string
	LastUsed    time.Time
	SessionFile string // most recent transcript .jsonl, empty if none
}

// EncodeClaudeDir maps a project path to its directory name under
// ~/.claude/projects: every non-alphanumeric byte becomes '-'.
func EncodeClaudeDir(path string) string {
	var b strings.Builder
	b.Grow(len(path))
	for _, r := range path {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

// LoadClaude reads project paths from <home>/.claude.json and resolves each
// one's recency from its transcript directory under <home>/.claude/projects.
func LoadClaude(home string) ([]ClaudeProject, error) {
	data, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		return nil, err
	}
	var top struct {
		Projects map[string]json.RawMessage `json:"projects"`
	}
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, err
	}
	projectsDir := filepath.Join(home, ".claude", "projects")
	out := make([]ClaudeProject, 0, len(top.Projects))
	for path := range top.Projects {
		p := ClaudeProject{Path: path}
		dir := filepath.Join(projectsDir, EncodeClaudeDir(path))
		p.LastUsed, p.SessionFile = latestTranscript(dir)
		out = append(out, p)
	}
	return out, nil
}

// latestTranscript returns the newest *.jsonl in dir by mtime, falling back
// to the directory's own mtime when it holds no transcripts.
func latestTranscript(dir string) (time.Time, string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return time.Time{}, ""
	}
	var best time.Time
	var bestFile string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(best) {
			best = info.ModTime()
			bestFile = filepath.Join(dir, e.Name())
		}
	}
	if bestFile == "" {
		if info, err := os.Stat(dir); err == nil {
			return info.ModTime(), ""
		}
	}
	return best, bestFile
}
