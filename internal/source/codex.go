package source

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type CodexSession struct {
	Cwd  string    `json:"cwd"`
	Time time.Time `json:"time"`
	ID   string    `json:"id"`
	File string    `json:"file"`
}

// CodexCache memoizes the (immutable) session_meta line of each rollout file
// so repeat scans only open files created since the last run.
type CodexCache struct {
	Files map[string]CodexSession `json:"files"`
}

var rolloutTime = regexp.MustCompile(`rollout-(\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2})-`)

// LoadCodex walks <home>/.codex/sessions and returns the newest session per
// working directory. The cache is updated in place; callers persist it.
func LoadCodex(home string, cache *CodexCache) (map[string]CodexSession, error) {
	if cache.Files == nil {
		cache.Files = map[string]CodexSession{}
	}
	root := filepath.Join(home, ".codex", "sessions")
	seen := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		seen[path] = true
		if _, ok := cache.Files[path]; ok {
			return nil
		}
		if s, ok := readSessionMeta(path); ok {
			cache.Files[path] = s
		} else {
			// Negative-cache unparseable files so they aren't re-read every scan.
			cache.Files[path] = CodexSession{File: path}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for path := range cache.Files {
		if !seen[path] {
			delete(cache.Files, path)
		}
	}
	newest := map[string]CodexSession{}
	for _, s := range cache.Files {
		if s.Cwd == "" {
			continue
		}
		if cur, ok := newest[s.Cwd]; !ok || s.Time.After(cur.Time) {
			newest[s.Cwd] = s
		}
	}
	return newest, nil
}

// readSessionMeta parses the first line of a rollout file:
//
//	{"type":"session_meta","payload":{"id":"...","cwd":"/abs/path",...}}
func readSessionMeta(path string) (CodexSession, bool) {
	f, err := os.Open(path)
	if err != nil {
		return CodexSession{}, false
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 64*1024)
	line, err := r.ReadBytes('\n')
	if len(line) == 0 && err != nil {
		return CodexSession{}, false
	}
	var meta struct {
		Type    string `json:"type"`
		Payload struct {
			ID  string `json:"id"`
			Cwd string `json:"cwd"`
		} `json:"payload"`
	}
	if json.Unmarshal(line, &meta) != nil || meta.Type != "session_meta" || meta.Payload.Cwd == "" {
		return CodexSession{}, false
	}
	s := CodexSession{Cwd: meta.Payload.Cwd, ID: meta.Payload.ID, File: path}
	if m := rolloutTime.FindStringSubmatch(filepath.Base(path)); m != nil {
		if t, err := time.ParseInLocation("2006-01-02T15-04-05", m[1], time.Local); err == nil {
			s.Time = t
		}
	}
	if s.Time.IsZero() {
		if info, err := os.Stat(path); err == nil {
			s.Time = info.ModTime()
		}
	}
	return s, true
}
