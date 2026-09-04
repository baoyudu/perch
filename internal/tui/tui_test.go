package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/baoyudu/perch/internal/config"
	"github.com/baoyudu/perch/internal/index"
	"github.com/baoyudu/perch/internal/preview"
)

func testModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("PERCH_CONFIG_DIR", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	projects := []index.Project{
		{Path: "/w/prior-analyst", Name: "prior-analyst", LastUsed: now.Add(-1 * time.Hour), LastAgent: "claude", ClaudeLast: now.Add(-1 * time.Hour)},
		{Path: "/w/learnitall", Name: "learnitall", LastUsed: now.Add(-24 * time.Hour), LastAgent: "codex", CodexLast: now.Add(-24 * time.Hour)},
		{Path: "/w/reports", Name: "reports", LastUsed: now.Add(-48 * time.Hour), LastAgent: "claude", ClaudeLast: now.Add(-48 * time.Hour)},
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

func TestResumeAsDefaultAction(t *testing.T) {
	m := testModel(t)
	m.cfg.Defaults.Action = config.ActionResume
	next, _ := m.Update(key(tea.KeyEnter))
	m = next.(Model)
	if m.result == nil || m.result.Action != ActResume {
		t.Fatalf("enter with resume default should resume, got %+v", m.result)
	}

	m = testModel(t)
	m.cfg.Defaults.Action = config.ActionResume
	m.all[0].LastAgent = "" // nothing to resume
	next, _ = m.Update(key(tea.KeyEnter))
	m = next.(Model)
	if m.result == nil || m.result.Action != ActCD {
		t.Fatalf("resume default without history should fall back to cd, got %+v", m.result)
	}
}

func TestSettingsOfferResume(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(key(tea.KeyCtrlE))
	m = next.(Model)
	for i := 0; i < 3; i++ { // cd → claude → codex → resume
		next, _ = m.Update(key(tea.KeyRight))
		m = next.(Model)
	}
	if m.cfg.Defaults.Action != config.ActionResume {
		t.Fatalf("third cycle should reach resume, got %q", m.cfg.Defaults.Action)
	}
	next, _ = m.Update(key(tea.KeyRight)) // wraps back to cd
	m = next.(Model)
	if m.cfg.Defaults.Action != config.ActionCD {
		t.Fatalf("cycle should wrap to cd, got %q", m.cfg.Defaults.Action)
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

func TestTabCyclesScope(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(key(tea.KeyTab)) // claude scope
	m = next.(Model)
	if len(m.filtered) != 2 {
		t.Fatalf("claude scope should show 2 projects, got %d", len(m.filtered))
	}
	for _, idx := range m.filtered {
		if m.all[idx].ClaudeLast.IsZero() {
			t.Errorf("claude scope leaked %s", m.all[idx].Name)
		}
	}
	next, _ = m.Update(key(tea.KeyTab)) // codex scope
	m = next.(Model)
	if len(m.filtered) != 1 || m.all[m.filtered[0]].Name != "learnitall" {
		t.Fatalf("codex scope should show only learnitall, got %d", len(m.filtered))
	}
	next, _ = m.Update(key(tea.KeyTab)) // back to all
	m = next.(Model)
	if len(m.filtered) != 3 {
		t.Fatalf("third tab should return to all, got %d", len(m.filtered))
	}
	next, _ = m.Update(key(tea.KeyShiftTab)) // backwards → codex
	m = next.(Model)
	if len(m.filtered) != 1 {
		t.Fatalf("shift+tab should cycle backwards to codex, got %d", len(m.filtered))
	}
}

func TestScopeCombinesWithQuery(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(key(tea.KeyTab)) // claude scope
	m = next.(Model)
	m = typeRunes(m, "learn") // learnitall is codex-only
	if len(m.filtered) != 0 {
		t.Fatalf("query outside scope should match nothing, got %d", len(m.filtered))
	}
}

func TestCreateProjectFlow(t *testing.T) {
	m := testModel(t)
	dir := t.TempDir()
	m.cfg.Defaults.ProjectsDir = dir
	m = typeRunes(m, "zzz-new") // matches nothing → prefills the create page
	next, _ := m.Update(key(tea.KeyCtrlT))
	m = next.(Model)
	if m.mode != modeCreate || m.editor.Value() != "zzz-new" {
		t.Fatalf("ctrl+t should open create prefilled, mode=%d value=%q", m.mode, m.editor.Value())
	}
	next, _ = m.Update(key(tea.KeyEnter))
	m = next.(Model)
	want := filepath.Join(dir, "zzz-new")
	if m.result == nil || m.result.Project.Path != want || m.result.Action != ActCD {
		t.Fatalf("create should finish with cd into %s, got %+v", want, m.result)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatal("directory should have been created")
	}
}

func TestCreateWithResumeDefaultFallsBackToCD(t *testing.T) {
	m := testModel(t)
	m.cfg.Defaults.ProjectsDir = t.TempDir()
	m.cfg.Defaults.Action = config.ActionResume
	next, _ := m.Update(key(tea.KeyCtrlT))
	m = next.(Model)
	m.editor.SetValue("fresh")
	next, _ = m.Update(key(tea.KeyEnter))
	m = next.(Model)
	if m.result == nil || m.result.Action != ActCD {
		t.Fatalf("new project has nothing to resume; want cd, got %+v", m.result)
	}
}

func TestCreateEscCancels(t *testing.T) {
	m := testModel(t)
	dir := t.TempDir()
	m.cfg.Defaults.ProjectsDir = dir
	next, _ := m.Update(key(tea.KeyCtrlT))
	m = next.(Model)
	m.editor.SetValue("never")
	next, _ = m.Update(key(tea.KeyEsc))
	m = next.(Model)
	if m.mode != modeList || m.result != nil {
		t.Fatal("esc should cancel back to the list")
	}
	if _, err := os.Stat(filepath.Join(dir, "never")); err == nil {
		t.Fatal("cancelled create must not make a directory")
	}
}

func TestSettingsEditProjectsDir(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(key(tea.KeyCtrlE))
	m = next.(Model)
	next, _ = m.Update(key(tea.KeyDown))
	m = next.(Model)
	next, _ = m.Update(key(tea.KeyDown)) // projects dir row
	m = next.(Model)
	next, _ = m.Update(key(tea.KeyEnter)) // start editing
	m = next.(Model)
	if !m.editingDir || m.editor.Value() != "~/Code" {
		t.Fatalf("enter should start editing prefilled, editing=%v value=%q", m.editingDir, m.editor.Value())
	}
	m.editor.SetValue("~/Work")
	next, _ = m.Update(key(tea.KeyEnter)) // save
	m = next.(Model)
	if m.editingDir || m.cfg.Defaults.ProjectsDir != "~/Work" {
		t.Fatalf("save failed: editing=%v dir=%q", m.editingDir, m.cfg.Defaults.ProjectsDir)
	}
	cfg2, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Defaults.ProjectsDir != "~/Work" {
		t.Errorf("projects_dir should persist to config.toml, got %q", cfg2.Defaults.ProjectsDir)
	}
}

func TestFirstResizeRequestsPreview(t *testing.T) {
	m := testModel(t)
	// Init sees the 80-col default where the preview pane is hidden, so the
	// first real WindowSizeMsg must request the selected project's preview.
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("first resize with a visible preview should request the preview")
	}
	pm, ok := cmd().(previewMsg)
	if !ok || pm.path != "/w/prior-analyst" {
		t.Fatalf("expected previewMsg for the selected project, got %#v", pm)
	}
	// Dedup: a second resize must not re-request it.
	_, cmd = m.Update(tea.WindowSizeMsg{Width: 130, Height: 30})
	if cmd != nil {
		t.Fatal("already-requested preview should not be requested again")
	}
}

func TestRightArrowFocusesPreviewEscReturns(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(Model)
	next, _ = m.Update(key(tea.KeyRight))
	m = next.(Model)
	if m.mode != modePreview {
		t.Fatalf("right at end of input should focus preview, mode=%d", m.mode)
	}
	next, cmd := m.Update(key(tea.KeyEsc))
	m = next.(Model)
	if m.mode != modeList || cmd != nil || m.result != nil {
		t.Fatalf("esc from preview should return to list without quitting")
	}
}

func TestRightArrowMidInputEditsFilter(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(Model)
	m = typeRunes(m, "re")
	next, _ = m.Update(key(tea.KeyLeft)) // cursor now mid-text
	m = next.(Model)
	next, _ = m.Update(key(tea.KeyRight)) // back to end: cursor move, not focus
	m = next.(Model)
	if m.mode != modePreview {
		// first right only moved the cursor; the second should switch
		next, _ = m.Update(key(tea.KeyRight))
		m = next.(Model)
	}
	if m.mode != modePreview {
		t.Fatal("right at end of input should switch to preview")
	}
	if m.input.Value() != "re" {
		t.Fatalf("filter text should be untouched, got %q", m.input.Value())
	}
}

func TestSettingsCycleAndPersist(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(key(tea.KeyCtrlE))
	m = next.(Model)
	if m.mode != modeSettings {
		t.Fatal("ctrl+e should open settings")
	}
	next, _ = m.Update(key(tea.KeyEnter)) // cycle default action: cd → claude
	m = next.(Model)
	if m.result != nil {
		t.Fatal("enter in settings must not pick a project")
	}
	if m.cfg.Defaults.Action != config.ActionClaude {
		t.Fatalf("default action = %q, want claude", m.cfg.Defaults.Action)
	}
	cfg2, err := config.Load() // same PERCH_CONFIG_DIR: reads state.json back
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Defaults.Action != config.ActionClaude {
		t.Error("default action should persist via state.json")
	}
	next, _ = m.Update(key(tea.KeyDown)) // icons row
	m = next.(Model)
	next, _ = m.Update(key(tea.KeyRight)) // nerd → plain
	m = next.(Model)
	if m.cfg.UI.Icons != "plain" || m.ic.prompt != plainIcons.prompt {
		t.Fatalf("icons should switch live, got %q", m.cfg.UI.Icons)
	}
	next, _ = m.Update(key(tea.KeyEsc))
	m = next.(Model)
	if m.mode != modeList {
		t.Error("esc should close settings")
	}
}

func TestPreviewScrollClamps(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 10})
	m = next.(Model)
	long := strings.Repeat("long preview content ", 40)
	next, _ = m.Update(previewMsg{"/w/prior-analyst", preview.Snippet{Agent: "claude", Role: "assistant", Text: long}})
	m = next.(Model)
	next, _ = m.Update(key(tea.KeyRight))
	m = next.(Model)
	for i := 0; i < 100; i++ {
		next, _ = m.Update(key(tea.KeyDown))
		m = next.(Model)
	}
	maxOff := len(m.renderPreviewLines(m.previewWidth()-4)) - m.listHeight()
	if maxOff <= 0 {
		t.Fatal("test needs overflowing preview content")
	}
	if m.previewOff != maxOff {
		t.Fatalf("offset %d, want clamped at %d", m.previewOff, maxOff)
	}
	for i := 0; i < 200; i++ {
		next, _ = m.Update(key(tea.KeyUp))
		m = next.(Model)
	}
	if m.previewOff != 0 {
		t.Fatalf("offset should clamp at 0, got %d", m.previewOff)
	}
}

func TestFirstRunHintDismissed(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(Model)
	if !containsPlain(m.View(), " ^e settings ") {
		t.Fatal("first run should show the settings hint chip")
	}
	next, _ = m.Update(key(tea.KeyCtrlE))
	m = next.(Model)
	next, _ = m.Update(key(tea.KeyEsc))
	m = next.(Model)
	if containsPlain(m.View(), " ^e settings ") {
		t.Error("hint chip should disappear after settings were opened")
	}
}

func TestFuzzyMatchPositions(t *testing.T) {
	// Substring: contiguous positions at the match site.
	score, pos, ok := fuzzyMatch("nal", "prior-analyst")
	if !ok || len(pos) != 3 || pos[0] != 7 || pos[2] != 9 {
		t.Fatalf("substring match: score=%d pos=%v ok=%v", score, pos, ok)
	}
	// Subsequence: increasing positions on the matched runes.
	_, pos, ok = fuzzyMatch("pran", "prior-analyst")
	if !ok || len(pos) != 4 {
		t.Fatalf("subsequence match failed: %v", pos)
	}
	for i := 1; i < len(pos); i++ {
		if pos[i] <= pos[i-1] {
			t.Fatalf("positions must increase: %v", pos)
		}
	}
	// Substring should outrank subsequence of the same query.
	sub, _, _ := fuzzyMatch("ana", "prior-analyst")
	seq, _, _ := fuzzyMatch("pns", "prior-analyst")
	if sub <= seq {
		t.Errorf("substring score %d should beat subsequence %d", sub, seq)
	}
	if _, _, ok := fuzzyMatch("zzz", "prior-analyst"); ok {
		t.Error("no match expected")
	}
}

func TestMouseWheelAndClick(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(Model)
	// Click the second visible row (header=0, border=1, rows start at 2).
	next, _ = m.Update(tea.MouseMsg{X: 4, Y: 3, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m = next.(Model)
	if m.cursor != 1 {
		t.Fatalf("click should select row 1, cursor=%d", m.cursor)
	}
	next, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	m = next.(Model)
	if m.cursor != 0 {
		t.Fatalf("wheel up should move cursor to 0, cursor=%d", m.cursor)
	}
}

func TestZeroSizeWindowDoesNotPanic(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 0, Height: 0})
	m = next.(Model)
	if m.View() == "" {
		t.Error("view should still render with defaults on 0x0 pty")
	}
	next, _ = m.Update(tea.WindowSizeMsg{Width: 8, Height: 3})
	m = next.(Model)
	_ = m.View() // tiny real sizes must not panic either
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
