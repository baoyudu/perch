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
	accentC = lipgloss.Color("205")
	claudeC = lipgloss.Color("209")
	codexC  = lipgloss.Color("75")
	pinC    = lipgloss.Color("220")
	gitC    = lipgloss.Color("108")
	dirtyC  = lipgloss.Color("179")
	dimC    = lipgloss.AdaptiveColor{Light: "245", Dark: "242"}
	borderC = lipgloss.AdaptiveColor{Light: "251", Dark: "238"}
	selBg   = lipgloss.AdaptiveColor{Light: "254", Dark: "236"}

	plain    = lipgloss.NewStyle()
	dim      = lipgloss.NewStyle().Foreground(dimC)
	accent   = lipgloss.NewStyle().Foreground(accentC)
	claudeSt = lipgloss.NewStyle().Foreground(claudeC)
	codexSt  = lipgloss.NewStyle().Foreground(codexC)
	pinSt    = lipgloss.NewStyle().Foreground(pinC)
	gitSt    = lipgloss.NewStyle().Foreground(gitC)
	dirtySt  = lipgloss.NewStyle().Foreground(dirtyC)
	nameSt   = lipgloss.NewStyle().Bold(true)
	matchSt  = lipgloss.NewStyle().Foreground(accentC).Bold(true).Underline(true)
	borderSt = lipgloss.NewStyle().Foreground(borderC)
	titleSt  = lipgloss.NewStyle().Foreground(dimC).Bold(true)
	helpKey  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "250"}).Bold(true)
)

// rowMatch records which runes of a row matched the filter, for highlighting.
type rowMatch struct {
	onName    bool
	positions []int
}

type Model struct {
	cfg        *config.Config
	all        []index.Project
	filtered   []int      // indices into all, in display order
	matchFor   []rowMatch // aligned with filtered
	cursor     int        // position within filtered
	offset     int        // first visible row
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
	ti.PlaceholderStyle = dim
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
		// Some ptys report 0×0; keep the previous (or default) size then.
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		m.input.Width = min(48, max(16, m.width/3))
		m.clampScroll()
		return m, nil
	case gitMsg:
		m.git[msg.path] = msg.st
		return m, nil
	case previewMsg:
		m.previews[msg.path] = msg.snip
		return m, nil
	case tea.MouseMsg:
		return m.handleMouse(msg)
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

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		return m.move(-3)
	case tea.MouseButtonWheelDown:
		return m.move(3)
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		// Rows start below the header (y=0) and panel top border (y=1).
		row := msg.Y - 2
		if row < 0 || row >= m.listHeight() || msg.X >= m.listWidth() {
			return m, nil
		}
		idx := m.offset + row
		if idx >= len(m.filtered) {
			return m, nil
		}
		m.cursor = idx
		m.clampScroll()
		return m, m.requestPreview()
	}
	return m, nil
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
	m.matchFor = m.matchFor[:0]
	if query == "" {
		for i := range m.all {
			m.filtered = append(m.filtered, i)
			m.matchFor = append(m.matchFor, rowMatch{})
		}
	} else {
		type scored struct {
			idx, score int
			rm         rowMatch
		}
		var hits []scored
		for i, p := range m.all {
			if score, rm, ok := matchProject(query, p); ok {
				hits = append(hits, scored{i, score, rm})
			}
		}
		sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
		for _, h := range hits {
			m.filtered = append(m.filtered, h.idx)
			m.matchFor = append(m.matchFor, h.rm)
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

// matchProject fuzzy-matches against the name (weighted) then the path.
func matchProject(query string, p index.Project) (int, rowMatch, bool) {
	if score, pos, ok := fuzzyMatch(query, p.Name); ok {
		return score + 200, rowMatch{onName: true, positions: pos}, true
	}
	if score, pos, ok := fuzzyMatch(query, index.TildePath(p.Path)); ok {
		return score, rowMatch{onName: false, positions: pos}, true
	}
	return 0, rowMatch{}, false
}

// fuzzyMatch returns a score and the matched rune positions in target.
// Exact substrings rank above subsequences; fewer gaps and earlier starts
// rank higher.
func fuzzyMatch(query, target string) (int, []int, bool) {
	q := []rune(strings.ToLower(query))
	t := []rune(strings.ToLower(target))
	if len(q) == 0 || len(q) > len(t) {
		return 0, nil, false
	}
	// Substring scan.
	for start := 0; start+len(q) <= len(t); start++ {
		hit := true
		for i, qc := range q {
			if t[start+i] != qc {
				hit = false
				break
			}
		}
		if hit {
			pos := make([]int, len(q))
			for i := range q {
				pos[i] = start + i
			}
			return 1000 - start - len(t)/8, pos, true
		}
	}
	// Subsequence scan.
	pos := make([]int, 0, len(q))
	ti, gaps, prev, first := 0, 0, -2, -1
	for _, qc := range q {
		for ti < len(t) && t[ti] != qc {
			ti++
		}
		if ti >= len(t) {
			return 0, nil, false
		}
		if first < 0 {
			first = ti
		}
		if prev >= 0 && ti != prev+1 {
			gaps++
		}
		pos = append(pos, ti)
		prev = ti
		ti++
	}
	return 500 - gaps*20 - first, pos, true
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
	return max(36, min(64, m.width*2/5))
}

func (m Model) listWidth() int { return m.width - m.previewWidth() }

// listHeight is the row count inside the panel: total minus header, panel
// borders, and help bar.
func (m Model) listHeight() int {
	return max(1, m.height-4)
}

// --- rendering ---

func (m Model) View() string {
	if m.result != nil {
		return ""
	}
	h := m.listHeight()
	list := panel("Projects", m.renderRows(h), m.listWidth(), h)
	if m.previewVisible() {
		prev := panel("Preview", m.renderPreviewLines(m.previewWidth()-4, h), m.previewWidth(), h)
		for i := range list {
			list[i] += prev[i]
		}
	}
	return m.renderHeader() + "\n" + strings.Join(list, "\n") + "\n" + m.renderHelp()
}

func (m Model) renderHeader() string {
	left := " " + m.input.View()
	counter := dim.Render(fmt.Sprintf("%d/%d", len(m.filtered), len(m.all)))
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(counter) - 2
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + counter
}

// panel wraps content lines in a rounded border with an embedded title.
// Content lines must already be exactly w-4 display cells wide.
func panel(title string, content []string, w, h int) []string {
	inner := max(2, w-2)
	t := titleSt.Render(" " + title + " ")
	fill := max(0, inner-1-lipgloss.Width(t))
	out := make([]string, 0, h+2)
	out = append(out, borderSt.Render("╭─")+t+borderSt.Render(strings.Repeat("─", fill)+"╮"))
	side := borderSt.Render("│")
	for i := 0; i < h; i++ {
		line := ""
		if i < len(content) {
			line = content[i]
		}
		if pad := inner - 2 - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		out = append(out, side+" "+line+" "+side)
	}
	out = append(out, borderSt.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return out
}

func (m Model) renderRows(h int) []string {
	cw := max(10, m.listWidth()-4)
	out := make([]string, 0, h)
	if len(m.filtered) == 0 {
		out = append(out, dim.Render("nothing matches — ctrl+u clears the filter"))
		return out
	}
	end := min(m.offset+h, len(m.filtered))
	for row := m.offset; row < end; row++ {
		out = append(out, m.renderRow(row, row == m.cursor, cw))
	}
	return out
}

// renderRow lays out fixed columns: marker, pin, name, agent, time, git, path.
// Every segment carries the selection background so the band covers the row.
func (m Model) renderRow(row int, selected bool, cw int) string {
	p := m.all[m.filtered[row]]
	match := m.matchFor[row]
	seg := func(st lipgloss.Style) lipgloss.Style {
		if selected {
			return st.Background(selBg)
		}
		return st
	}

	nameW := min(24, max(12, cw/3))
	showBadge := cw >= 46
	showGit := cw >= 74
	gitW := 14

	var b strings.Builder
	if selected {
		b.WriteString(seg(accent).Render("▌ "))
	} else {
		b.WriteString(seg(plain).Render("  "))
	}
	if p.Pinned {
		b.WriteString(seg(pinSt).Render("★ "))
	} else {
		b.WriteString(seg(plain).Render("  "))
	}

	namePos := match.positions
	if !match.onName {
		namePos = nil
	}
	name := highlight(p.Name, namePos, seg(nameSt), seg(matchSt), nameW)
	b.WriteString(name)
	b.WriteString(seg(plain).Render(strings.Repeat(" ", max(0, nameW-lipgloss.Width(name))+1)))

	if showBadge {
		switch p.LastAgent {
		case index.AgentClaude:
			b.WriteString(seg(claudeSt).Render("✳ claude"))
		case index.AgentCodex:
			b.WriteString(seg(codexSt).Render("◆ codex "))
		default:
			b.WriteString(seg(plain).Render("        "))
		}
		when := index.RelTime(p.LastUsed, m.now)
		b.WriteString(seg(dim).Render(fmt.Sprintf(" %4s", when)))
	}

	if showGit {
		cell := ""
		if st, ok := m.git[p.Path]; ok && st.IsRepo {
			cell = truncate(st.Branch, 10)
			if st.Dirty > 0 {
				cell += fmt.Sprintf("*%d", min(st.Dirty, 99))
			}
		}
		b.WriteString(seg(plain).Render("  "))
		b.WriteString(seg(gitSt).Render(cell))
		b.WriteString(seg(plain).Render(strings.Repeat(" ", max(0, gitW-lipgloss.Width(cell)))))
	}

	rest := cw - lipgloss.Width(b.String()) - 1
	if rest > 6 {
		pathPos := match.positions
		if match.onName {
			pathPos = nil
		}
		b.WriteString(seg(plain).Render(" "))
		b.WriteString(highlight(index.TildePath(p.Path), pathPos, seg(dim), seg(matchSt), rest))
	}
	if pad := cw - lipgloss.Width(b.String()); pad > 0 {
		b.WriteString(seg(plain).Render(strings.Repeat(" ", pad)))
	}
	return b.String()
}

// highlight renders s truncated to maxW cells, painting matched rune
// positions with hl and the rest with base.
func highlight(s string, positions []int, base, hl lipgloss.Style, maxW int) string {
	runes := []rune(s)
	kept := len(runes)
	if lipgloss.Width(s) > maxW {
		for kept > 0 && lipgloss.Width(string(runes[:kept]))+1 > maxW {
			kept--
		}
	}
	if len(positions) == 0 {
		out := string(runes[:kept])
		if kept < len(runes) {
			out += "…"
		}
		return base.Render(out)
	}
	posSet := make(map[int]bool, len(positions))
	for _, p := range positions {
		posSet[p] = true
	}
	var b strings.Builder
	// Batch consecutive runes of the same style to limit escape sequences.
	run := []rune{}
	runHL := false
	flush := func() {
		if len(run) == 0 {
			return
		}
		if runHL {
			b.WriteString(hl.Render(string(run)))
		} else {
			b.WriteString(base.Render(string(run)))
		}
		run = run[:0]
	}
	for i := 0; i < kept; i++ {
		if posSet[i] != runHL {
			flush()
			runHL = posSet[i]
		}
		run = append(run, runes[i])
	}
	flush()
	if kept < len(runes) {
		b.WriteString(base.Render("…"))
	}
	return b.String()
}

func (m Model) renderPreviewLines(cw, h int) []string {
	p := m.selectedProject()
	if p == nil || cw < 10 {
		return nil
	}
	wrap := lipgloss.NewStyle().Width(cw)
	var lines []string
	add := func(s string) { lines = append(lines, strings.Split(s, "\n")...) }

	title := truncate(p.Name, cw-2)
	if p.Pinned {
		title = pinSt.Render("★ ") + nameSt.Render(title)
	} else {
		title = nameSt.Render(title)
	}
	add(title)
	add(dim.Render(wrap.Render(index.TildePath(p.Path))))
	add("")

	var usage []string
	if !p.ClaudeLast.IsZero() {
		usage = append(usage, claudeSt.Render("✳ claude ")+dim.Render(index.RelTime(p.ClaudeLast, m.now)+" ago"))
	}
	if !p.CodexLast.IsZero() {
		usage = append(usage, codexSt.Render("◆ codex ")+dim.Render(index.RelTime(p.CodexLast, m.now)+" ago"))
	}
	if len(usage) > 0 {
		add(strings.Join(usage, dim.Render("  ·  ")))
	}
	if st, ok := m.git[p.Path]; ok && st.IsRepo {
		line := gitSt.Render(st.Branch)
		if st.Dirty > 0 {
			line += dirtySt.Render(fmt.Sprintf("  %d uncommitted", st.Dirty))
		}
		add(line)
	}
	action := m.cfg.ActionFor(p.Path)
	add(dim.Render("enter → ") + accent.Render(action))
	add(dim.Render(strings.Repeat("─", cw)))

	if snip, ok := m.previews[p.Path]; ok {
		if snip.Text != "" {
			add(dim.Render(fmt.Sprintf("last session · %s · %s", snip.Agent, snip.Role)))
			add(wrap.Render(snip.Text))
		} else {
			add(dim.Render("no session preview"))
		}
	} else if p.LastAgent != "" {
		add(dim.Render("loading preview…"))
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	return lines
}

func (m Model) renderHelp() string {
	type item struct{ key, label string }
	items := []item{
		{"enter", "default"}, {"^o", "cd"}, {"^a", "claude"}, {"^x", "codex"},
		{"^r", "resume"}, {"^s", "pin"}, {"esc", "quit"},
	}
	var parts []string
	for _, it := range items {
		parts = append(parts, helpKey.Render(it.key)+dim.Render(" "+it.label))
	}
	full := "  " + strings.Join(parts, dim.Render("  ·  "))
	if lipgloss.Width(full) <= m.width {
		return full
	}
	var keys []string
	for _, it := range items {
		keys = append(keys, helpKey.Render(it.key))
	}
	return "  " + strings.Join(keys, " ")
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
	prog := tea.NewProgram(New(cfg, projects),
		tea.WithInput(tty), tea.WithOutput(tty),
		tea.WithAltScreen(), tea.WithMouseCellMotion())
	final, err := prog.Run()
	if err != nil {
		return nil, err
	}
	return final.(Model).result, nil
}
