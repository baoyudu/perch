package source

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEncodeClaudeDir(t *testing.T) {
	cases := map[string]string{
		"/Users/bao/Code/prior-analyst":        "-Users-bao-Code-prior-analyst",
		"/Users/bao/Code/Math4AI_Homework_stat": "-Users-bao-Code-Math4AI-Homework-stat",
		"/Users/bao/Code/a.b c":                 "-Users-bao-Code-a-b-c",
		"/Users/bao/项目":                         "-Users-bao---",
	}
	for in, want := range cases {
		if got := EncodeClaudeDir(in); got != want {
			t.Errorf("EncodeClaudeDir(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadClaude(t *testing.T) {
	home := t.TempDir()
	proj := filepath.Join(home, "work", "demo")
	claudeJSON := `{"projects": {"` + proj + `": {"history": []}}}`
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(claudeJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(home, ".claude", "projects", EncodeClaudeDir(proj))
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	session := filepath.Join(transcriptDir, "abc.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadClaude(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != proj {
		t.Fatalf("got %+v", got)
	}
	if got[0].SessionFile != session {
		t.Errorf("SessionFile = %q, want %q", got[0].SessionFile, session)
	}
	if got[0].LastUsed.IsZero() {
		t.Error("LastUsed should be set from transcript mtime")
	}
}

func TestLoadCodex(t *testing.T) {
	home := t.TempDir()
	day := filepath.Join(home, ".codex", "sessions", "2026", "07", "20")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"session_meta","payload":{"id":"id-1","cwd":"/work/demo"}}` + "\n"
	file := filepath.Join(day, "rollout-2026-07-20T19-02-34-id-1.jsonl")
	if err := os.WriteFile(file, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	// A later session in the same cwd should win.
	line2 := `{"type":"session_meta","payload":{"id":"id-2","cwd":"/work/demo"}}` + "\n"
	file2 := filepath.Join(day, "rollout-2026-07-20T21-00-00-id-2.jsonl")
	if err := os.WriteFile(file2, []byte(line2), 0o644); err != nil {
		t.Fatal(err)
	}

	cache := &CodexCache{}
	got, err := LoadCodex(home, cache)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := got["/work/demo"]
	if !ok || s.ID != "id-2" {
		t.Fatalf("got %+v", got)
	}
	want := time.Date(2026, 7, 20, 21, 0, 0, 0, time.Local)
	if !s.Time.Equal(want) {
		t.Errorf("Time = %v, want %v", s.Time, want)
	}
	if len(cache.Files) != 2 {
		t.Errorf("cache should hold both files, got %d", len(cache.Files))
	}

	// Deleting a file must evict it from the cache on rescan.
	if err := os.Remove(file2); err != nil {
		t.Fatal(err)
	}
	got, err = LoadCodex(home, cache)
	if err != nil {
		t.Fatal(err)
	}
	if got["/work/demo"].ID != "id-1" {
		t.Errorf("after removal newest should be id-1, got %+v", got["/work/demo"])
	}
	if len(cache.Files) != 1 {
		t.Errorf("cache should evict removed file, got %d entries", len(cache.Files))
	}
}
