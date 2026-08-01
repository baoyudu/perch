package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/baoyudu/psw/internal/config"
	"github.com/baoyudu/psw/internal/index"
)

func testModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("PSW_CONFIG_DIR", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	projects := []index.Project{
		{Path: "/w/prior-analyst", Name: "prior-analyst", LastUsed: now.Add(-1 * time.Hour), LastAgent: "claude"},
		{Path: "/w/learnitall", Name: "learnitall", LastUsed: now.Add(-24 * time.Hour), LastAgent: "codex"},
		{Path: "/w/reports", Name: "reports", LastUsed: now.Add(-48 * time.Hour), LastAgent: "claude"},
	}
	return New(cfg, projects)
}

func key(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

func typeRunes(m Model, s string) Model {
	for _, r := range s {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	return m
}

func TestFilterNarrowsList(t *testing.T) {
	m := testModel(t)
	if len(m.filtered) != 3 {
		t.Fatalf("initial filtered = %d", len(m.filtered))
	}
	m = typeRunes(m, "learn")
	if len(m.filtered) != 1 || m.all[m.filtered[0]].Name != "learnitall" {
		t.Fatalf("filter failed: %d matches", len(m.filtered))
	}
}

func TestFuzzySubsequence(t *testing.T) {
	m := testModel(t)
	m = typeRunes(m, "pran") // subsequence of prior-analyst
	if len(m.filtered) == 0 || m.all[m.filtered[0]].Name != "prior-analyst" {
		t.Fatalf("subsequence match failed")
	}
}

func TestEnterUsesDefaultAction(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(key(tea.KeyEnter))
	m = next.(Model)
	if m.result == nil || m.result.Action != ActCD {
		t.Fatalf("enter should pick default cd, got %+v", m.result)
	}
	if m.result.Project.Name != "prior-analyst" {
		t.Errorf("should select first row, got %s", m.result.Project.Name)
	}
}

func TestActionKeys(t *testing.T) {
	cases := map[tea.KeyType]Action{
		tea.KeyCtrlO: ActCD,
		tea.KeyCtrlA: ActClaude,
		tea.KeyCtrlX: ActCodex,
		tea.KeyCtrlR: ActResume,
	}
	for k, want := range cases {
		m := testModel(t)
		next, _ := m.Update(key(k))
		m = next.(Model)
		if m.result == nil || m.result.Action != want {
			t.Errorf("key %v: got %+v, want action %s", k, m.result, want)
		}
	}
}

func TestResumeWithoutHistoryFallsBackToCD(t *testing.T) {
	m := testModel(t)
	m.all[0].LastAgent = ""
	next, _ := m.Update(key(tea.KeyCtrlR))
	m = next.(Model)
	if m.result == nil || m.result.Action != ActCD {
		t.Fatalf("resume without agent should fall back to cd, got %+v", m.result)
	}
}

func TestNavigationAndSelection(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(key(tea.KeyDown))
	m = next.(Model)
	next, _ = m.Update(key(tea.KeyCtrlX))
	m = next.(Model)
	if m.result == nil || m.result.Project.Name != "learnitall" {
		t.Fatalf("expected second row selected, got %+v", m.result)
	}
}

func TestPinReordersAndPersists(t *testing.T) {
	m := testModel(t)
	// Move to the last project and pin it.
	next, _ := m.Update(key(tea.KeyDown))
	m = next.(Model)
	next, _ = m.Update(key(tea.KeyDown))
	m = next.(Model)
	next, _ = m.Update(key(tea.KeyCtrlS))
	m = next.(Model)
	if m.all[m.filtered[0]].Name != "reports" || !m.all[m.filtered[0]].Pinned {
		t.Fatalf("pinned project should move to top, top = %+v", m.all[m.filtered[0]])
	}
	if m.all[m.filtered[m.cursor]].Name != "reports" {
		t.Errorf("cursor should follow pinned project")
	}
	if !m.cfg.Pinned("/w/reports") {
		t.Error("pin should persist to config state")
	}
}

func TestEscCancels(t *testing.T) {
	m := testModel(t)
	next, cmd := m.Update(key(tea.KeyEsc))
	m = next.(Model)
	if m.result != nil {
		t.Error("esc should not set a result")
	}
	if cmd == nil {
		t.Error("esc should quit")
	}
}

func TestViewRenders(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(Model)
	out := m.View()
	if out == "" {
		t.Fatal("view should render")
	}
	for _, want := range []string{"prior-analyst", "learnitall", "3/3"} {
		if !containsPlain(out, want) {
			t.Errorf("view missing %q", want)
		}
	}
}

// containsPlain strips ANSI escapes before substring search.
func containsPlain(s, sub string) bool {
	var b []rune
	inEsc := false
	for _, r := range s {
		switch {
		case inEsc:
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
		case r == 0x1b:
			inEsc = true
		default:
			b = append(b, r)
		}
	}
	return stringsContains(string(b), sub)
}

func stringsContains(s, sub string) bool {
	return len(sub) == 0 || len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
