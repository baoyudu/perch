// Package tui implements the interactive picker. It renders to /dev/tty so
// stdout stays free for the selection protocol consumed by the shell wrapper.
package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/baoyudu/psw/internal/config"
	"github.com/baoyudu/psw/internal/gitinfo"
	"github.com/baoyudu/psw/internal/index"
	"github.com/baoyudu/psw/internal/preview"
)

type Action string

const (
	ActCD     Action = config.ActionCD
	ActClaude Action = config.ActionClaude
	ActCodex  Action = config.ActionCodex
	ActResume Action = "resume"
)

// Result is what the user picked; nil result means cancelled.
type Result struct {
	Project index.Project
	Action  Action
}

type gitMsg struct {
	path string
	st   gitinfo.Status
}

type previewMsg struct {
	path string
	snip preview.Snippet
}

var (
	dim        = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "245", Dark: "241"})
	accent     = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	claudeSt   = lipgloss.NewStyle().Foreground(lipgloss.Color("209"))
	codexSt    = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
	pinSt      = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	gitSt      = lipgloss.NewStyle().Foreground(lipgloss.Color("108"))
	dirtySt    = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
	nameSt     = lipgloss.NewStyle().Bold(true)
	selectedSt = lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "254", Dark: "236"})
	borderSt   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "250", Dark: "238"})
)

type Model struct {
	cfg        *config.Config
	all        []index.Project
	filtered   []int // indices into all, in display order
	cursor     int   // position within filtered
	offset     int   // first visible row
	input      textinput.Model
	width      int
	height     int
	git        map[string]gitinfo.Status
	previews   map[string]preview.Snippet
	previewReq map[string]bool
	result     *Result
	now        time.Time
}

func New(cfg *config.Config, projects []index.Project) Model {
	ti := textinput.New()
	ti.Prompt = "❯ "
	ti.PromptStyle = accent
	ti.Placeholder = "type to filter"
	ti.Focus()
	m := Model{
		cfg:        cfg,
		all:        projects,
		input:      ti,
		width:      80,
		height:     24,
		git:        map[string]gitinfo.Status{},
		previews:   map[string]preview.Snippet{},
		previewReq: map[string]bool{},
		now:        time.Now(),
	}
	m.refilter()
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.all)+1)
	for _, p := range m.all {
		path := p.Path
		cmds = append(cmds, func() tea.Msg { return gitMsg{path, gitinfo.Load(path)} })
	}
	if c := m.requestPreview(); c != nil {
		cmds = append(cmds, c)
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampScroll()
		return m, nil
	case gitMsg:
		m.git[msg.path] = msg.st
		return m, nil
	case previewMsg:
		m.previews[msg.path] = msg.snip
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			return m.choose(Action(m.cfg.ActionFor(m.selectedPath())))
		case tea.KeyCtrlO:
			return m.choose(ActCD)
		case tea.KeyCtrlA:
			return m.choose(ActClaude)
		case tea.KeyCtrlX:
			return m.choose(ActCodex)
		case tea.KeyCtrlR:
			return m.choose(ActResume)
		case tea.KeyCtrlS:
			return m.togglePin()
		case tea.KeyUp, tea.KeyCtrlP, tea.KeyCtrlK:
			return m.move(-1)
		case tea.KeyDown, tea.KeyCtrlN, tea.KeyCtrlJ:
			return m.move(1)
		case tea.KeyPgUp:
			return m.move(-m.listHeight())
		case tea.KeyPgDown:
			return m.move(m.listHeight())
		case tea.KeyHome:
			return m.move(-len(m.filtered))
		case tea.KeyEnd:
			return m.move(len(m.filtered))
		}
		before := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if m.input.Value() != before {
			m.refilter()
			return m, tea.Batch(cmd, m.requestPreview())
		}
		return m, cmd
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) move(delta int) (tea.Model, tea.Cmd) {
	if len(m.filtered) == 0 {
		return m, nil
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	m.clampScroll()
	return m, m.requestPreview()
}

func (m *Model) clampScroll() {
	h := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m Model) selectedPath() string {
	if len(m.filtered) == 0 {
		return ""
	}
	return m.all[m.filtered[m.cursor]].Path
}

func (m Model) selectedProject() *index.Project {
	if len(m.filtered) == 0 {
		return nil
	}
	return &m.all[m.filtered[m.cursor]]
}

func (m Model) choose(act Action) (tea.Model, tea.Cmd) {
	p := m.selectedProject()
	if p == nil {
		return m, nil
	}
	if act == ActResume && p.LastAgent == "" {
		act = ActCD // nothing to resume
	}
	m.result = &Result{Project: *p, Action: act}
	return m, tea.Quit
}

func (m Model) togglePin() (tea.Model, tea.Cmd) {
	p := m.selectedProject()
	if p == nil {
		return m, nil
	}
	path := p.Path
	pinned := !p.Pinned
	_ = m.cfg.SetPinned(path, pinned)
	for i := range m.all {
		if m.all[i].Path == path {
			m.all[i].Pinned = pinned
		}
	}
	sort.SliceStable(m.all, func(i, j int) bool {
		if m.all[i].Pinned != m.all[j].Pinned {
			return m.all[i].Pinned
		}
		return m.all[i].LastUsed.After(m.all[j].LastUsed)
	})
	m.refilter()
	for i, idx := range m.filtered {
		if m.all[idx].Path == path {
			m.cursor = i
			break
		}
	}
	m.clampScroll()
	return m, nil
}

// refilter recomputes the visible list from the query, preserving base order
// (pinned first, then recency) and ranking better fuzzy matches higher.
func (m *Model) refilter() {
	query := strings.ToLower(strings.TrimSpace(m.input.Value()))
	m.filtered = m.filtered[:0]
	if query == "" {
		for i := range m.all {
			m.filtered = append(m.filtered, i)
		}
	} else {
		type scored struct{ idx, score int }
		var hits []scored
		for i, p := range m.all {
			if s, ok := matchScore(query, p.Name, index.TildePath(p.Path)); ok {
				hits = append(hits, scored{i, s})
			}
		}
		sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
		for _, h := range hits {
			m.filtered = append(m.filtered, h.idx)
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.offset = 0
	m.clampScroll()
}

// matchScore fuzzy-matches query against name (weighted) and path.
func matchScore(query, name, path string) (int, bool) {
	if s, ok := fuzzyOne(query, strings.ToLower(name)); ok {
		return s + 200, true
	}
	return fuzzyOne(query, strings.ToLower(path))
}

func fuzzyOne(query, s string) (int, bool) {
	if idx := strings.Index(s, query); idx >= 0 {
		return 1000 - idx - len(s)/8, true
	}
	// Subsequence match: fewer gaps and an earlier start score higher.
	start, gaps, prev := -1, 0, -2
	si := 0
	for _, qc := range query {
		found := false
		for si < len(s) {
			if rune(s[si]) == qc {
				if start < 0 {
					start = si
				}
				if si != prev+1 && prev >= 0 {
					gaps++
				}
				prev = si
				si++
				found = true
				break
			}
			si++
		}
		if !found {
			return 0, false
		}
	}
	return 500 - gaps*20 - start, true
}

func (m Model) requestPreview() tea.Cmd {
	p := m.selectedProject()
	if p == nil || m.previewReq[p.Path] || !m.previewVisible() {
		return nil
	}
	m.previewReq[p.Path] = true
	proj := *p
	return func() tea.Msg { return previewMsg{proj.Path, preview.Load(proj)} }
}

func (m Model) previewVisible() bool { return m.width >= 96 }

func (m Model) previewWidth() int {
	if !m.previewVisible() {
		return 0
	}
	w := m.width * 2 / 5
	if w > 60 {
		w = 60
	}
	return w
}

func (m Model) listWidth() int {
	if !m.previewVisible() {
		return m.width
	}
	return m.width - m.previewWidth() - 1
}

func (m Model) listHeight() int {
	h := m.height - 3 // header + counter line + help bar
	if h < 1 {
		h = 1
	}
	return h
}

// --- rendering ---

func (m Model) View() string {
	if m.result != nil {
		return ""
	}
	header := m.input.View()
	counter := dim.Render(fmt.Sprintf("  %d/%d projects", len(m.filtered), len(m.all)))
	left := m.renderList()
	body := left
	if m.previewVisible() {
		body = joinColumns(left, m.renderPreview(), m.listWidth(), m.listHeight())
	}
	help := dim.Render(" enter default · ^o cd · ^a claude · ^x codex · ^r resume · ^s pin · esc quit")
	return header + counter + "\n" + body + help
}

func (m Model) renderList() string {
	var b strings.Builder
	h := m.listHeight()
	w := m.listWidth()
	end := m.offset + h
	if end > len(m.filtered) {
		end = len(m.filtered)
	}
	if len(m.filtered) == 0 {
		b.WriteString(dim.Render("  nothing matches") + "\n")
		for i := 1; i < h; i++ {
			b.WriteString("\n")
		}
		return b.String()
	}
	for row := m.offset; row < end; row++ {
		b.WriteString(m.renderRow(m.all[m.filtered[row]], row == m.cursor, w))
		b.WriteString("\n")
	}
	for i := end - m.offset; i < h; i++ {
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) renderRow(p index.Project, selected bool, width int) string {
	marker := "  "
	if selected {
		marker = accent.Render("▸ ")
	}
	pin := "  "
	if p.Pinned {
		pin = pinSt.Render("★ ")
	}
	name := truncate(p.Name, 26)
	nameCell := nameSt.Render(name) + strings.Repeat(" ", max(0, 27-lipgloss.Width(name)))

	badge := strings.Repeat(" ", 9)
	switch p.LastAgent {
	case index.AgentClaude:
		badge = claudeSt.Render("✳ claude ")
	case index.AgentCodex:
		badge = codexSt.Render("◆ codex  ")
	}
	when := dim.Render(pad(index.RelTime(p.LastUsed, m.now), 5))

	git := ""
	if st, ok := m.git[p.Path]; ok && st.IsRepo {
		git = gitSt.Render(truncate(st.Branch, 14))
		if st.Dirty > 0 {
			git += dirtySt.Render(fmt.Sprintf("*%d", st.Dirty))
		}
		git += " "
	}

	line := marker + pin + nameCell + badge + when + " " + git
	rest := width - lipgloss.Width(line) - 1
	if rest > 4 {
		line += dim.Render(truncate(index.TildePath(p.Path), rest))
	}
	if selected {
		if pad := width - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		line = selectedSt.Render(line)
	}
	return line
}

func (m Model) renderPreview() string {
	w := m.previewWidth() - 2
	p := m.selectedProject()
	if p == nil || w < 10 {
		return ""
	}
	wrap := lipgloss.NewStyle().Width(w)
	var parts []string
	parts = append(parts, nameSt.Render(truncate(p.Name, w)))
	parts = append(parts, dim.Render(wrap.Render(index.TildePath(p.Path))))

	var usage []string
	if !p.ClaudeLast.IsZero() {
		usage = append(usage, claudeSt.Render("✳ claude ")+dim.Render(index.RelTime(p.ClaudeLast, m.now)+" ago"))
	}
	if !p.CodexLast.IsZero() {
		usage = append(usage, codexSt.Render("◆ codex ")+dim.Render(index.RelTime(p.CodexLast, m.now)+" ago"))
	}
	if len(usage) > 0 {
		parts = append(parts, strings.Join(usage, dim.Render(" · ")))
	}
	if st, ok := m.git[p.Path]; ok && st.IsRepo {
		line := gitSt.Render(" " + st.Branch)
		if st.Dirty > 0 {
			line += dirtySt.Render(fmt.Sprintf("  %d uncommitted", st.Dirty))
		}
		parts = append(parts, line)
	}
	parts = append(parts, dim.Render(strings.Repeat("─", w)))
	if snip, ok := m.previews[p.Path]; ok {
		if snip.Text != "" {
			parts = append(parts, dim.Render(fmt.Sprintf("last session · %s (%s)", snip.Agent, snip.Role)))
			parts = append(parts, wrap.Render(snip.Text))
		} else {
			parts = append(parts, dim.Render("no session preview"))
		}
	} else if p.LastAgent != "" {
		parts = append(parts, dim.Render("loading preview…"))
	}
	content := strings.Join(parts, "\n")
	lines := strings.Split(content, "\n")
	if h := m.listHeight(); len(lines) > h {
		lines = lines[:h]
	}
	return strings.Join(lines, "\n")
}

// joinColumns places right beside left with a vertical border, both clipped
// to height rows.
func joinColumns(left, right string, leftWidth, height int) string {
	ll := strings.Split(strings.TrimRight(left, "\n"), "\n")
	rl := strings.Split(strings.TrimRight(right, "\n"), "\n")
	var b strings.Builder
	for i := 0; i < height; i++ {
		var l, r string
		if i < len(ll) {
			l = ll[i]
		}
		if i < len(rl) {
			r = rl[i]
		}
		if pad := leftWidth - lipgloss.Width(l); pad > 0 {
			l += strings.Repeat(" ", pad)
		}
		b.WriteString(l + borderSt.Render("│") + r + "\n")
	}
	return b.String()
}

func truncate(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > w {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

func pad(s string, w int) string {
	if n := w - lipgloss.Width(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Run shows the picker on /dev/tty and returns the user's choice, or nil if
// cancelled.
func Run(cfg *config.Config, projects []index.Project) (*Result, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("psw pick needs an interactive terminal: %w", err)
	}
	defer tty.Close()
	prog := tea.NewProgram(New(cfg, projects), tea.WithInput(tty), tea.WithOutput(tty))
	final, err := prog.Run()
	if err != nil {
		return nil, err
	}
	return final.(Model).result, nil
}
