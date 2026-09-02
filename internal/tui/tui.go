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
	"github.com/muesli/termenv"

	"github.com/baoyudu/perch/internal/config"
	"github.com/baoyudu/perch/internal/gitinfo"
	"github.com/baoyudu/perch/internal/index"
	"github.com/baoyudu/perch/internal/preview"
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

// Catppuccin-flavoured palette (Latte for light, Macchiato-ish for dark).
// Truecolor terminals get these exact values; lipgloss downsamples elsewhere.
var (
	accentC  = lipgloss.AdaptiveColor{Light: "#8839ef", Dark: "#cba6f7"} // mauve
	claudeC  = lipgloss.AdaptiveColor{Light: "#fe640b", Dark: "#fab387"} // peach
	codexC   = lipgloss.AdaptiveColor{Light: "#1e66f5", Dark: "#89b4fa"} // blue
	pinC     = lipgloss.AdaptiveColor{Light: "#df8e1d", Dark: "#f9e2af"} // yellow
	gitC     = lipgloss.AdaptiveColor{Light: "#40a02b", Dark: "#a6e3a1"} // green
	dirtyC   = lipgloss.AdaptiveColor{Light: "#e64553", Dark: "#eba0ac"} // maroon
	dimC     = lipgloss.AdaptiveColor{Light: "#9ca0b0", Dark: "#6c7086"}
	borderC  = lipgloss.AdaptiveColor{Light: "#ccd0da", Dark: "#45475a"}
	surfaceC = lipgloss.AdaptiveColor{Light: "#e6e9ef", Dark: "#313244"} // chips + selection band
	strongC  = lipgloss.AdaptiveColor{Light: "#4c4f69", Dark: "#cdd6f4"}
	onAccent = lipgloss.AdaptiveColor{Light: "#eff1f5", Dark: "#1e1e2e"} // text on accent chips
	selBg    = surfaceC

	plain       = lipgloss.NewStyle()
	dim         = lipgloss.NewStyle().Foreground(dimC)
	accent      = lipgloss.NewStyle().Foreground(accentC)
	claudeSt    = lipgloss.NewStyle().Foreground(claudeC)
	codexSt     = lipgloss.NewStyle().Foreground(codexC)
	pinSt       = lipgloss.NewStyle().Foreground(pinC)
	gitSt       = lipgloss.NewStyle().Foreground(gitC)
	dirtySt     = lipgloss.NewStyle().Foreground(dirtyC)
	nameSt      = lipgloss.NewStyle().Bold(true)
	nameSelSt   = lipgloss.NewStyle().Bold(true).Foreground(accentC)
	matchSt     = lipgloss.NewStyle().Foreground(accentC).Bold(true).Underline(true)
	borderSt    = lipgloss.NewStyle().Foreground(borderC)
	thumbSt     = lipgloss.NewStyle().Foreground(accentC)
	labelSt     = lipgloss.NewStyle().Foreground(dimC).Italic(true)
	titleOn     = lipgloss.NewStyle().Background(accentC).Foreground(onAccent).Bold(true)
	titleClaude = lipgloss.NewStyle().Background(claudeC).Foreground(onAccent).Bold(true)
	titleCodex  = lipgloss.NewStyle().Background(codexC).Foreground(onAccent).Bold(true)
	titleOff    = lipgloss.NewStyle().Background(surfaceC).Foreground(dimC).Bold(true)
	chipSt      = lipgloss.NewStyle().Background(surfaceC).Foreground(dimC)
	keySt       = lipgloss.NewStyle().Background(surfaceC).Foreground(strongC).Bold(true)
)

// iconSet holds the glyphs the picker draws. Prefix glyphs are exactly two
// display cells (icon + space) in both sets so row layout never shifts.
// Nerd glyphs stay in the BMP private-use area, which every measurer and
// terminal agrees is one cell wide.
type iconSet struct {
	prompt   string // filter input prompt
	folder   string // unpinned row marker
	pin      string // pinned row marker
	claude   string // claude badge prefix
	codex    string // codex badge prefix
	branch   string // git branch prefix
	preview  string // preview panel title
	settings string // settings panel title
}

var nerdIcons = iconSet{
	prompt:   " ", //  search
	folder:   " ", //  folder
	pin:      " ", //  pin
	claude:   " ", //  asterisk
	codex:    " ", //  diamond
	branch:   " ", //  powerline branch
	preview:  " Preview",
	settings: " Settings",
}

var plainIcons = iconSet{
	prompt:   "❯ ",
	folder:   "  ",
	pin:      "★ ",
	claude:   "✳ ",
	codex:    "◆ ",
	branch:   "⎇ ",
	preview:  "Preview",
	settings: "Settings",
}

// scope narrows the list to projects that have sessions from one agent.
type scope int

const (
	scopeAll scope = iota
	scopeClaude
	scopeCodex
	scopeCount
)

// mode is which pane owns the keyboard: the list (default), the preview
// (entered with →, scrollable), or the settings page (^e).
type mode int

const (
	modeList mode = iota
	modePreview
	modeSettings
)

// rowMatch records which runes of a row matched the filter, for highlighting.
type rowMatch struct {
	onName    bool
	positions []int
}

type Model struct {
	cfg         *config.Config
	all         []index.Project
	filtered    []int      // indices into all, in display order
	matchFor    []rowMatch // aligned with filtered
	cursor      int        // position within filtered
	offset      int        // first visible row
	scope       scope      // agent filter cycled with tab
	mode        mode
	settingsRow int // cursor within the settings page
	previewOff  int // scroll offset within the preview pane
	ic          iconSet
	input       textinput.Model
	width       int
	height      int
	git         map[string]gitinfo.Status
	previews    map[string]preview.Snippet
	previewReq  map[string]bool
	result      *Result
	now         time.Time
}

func New(cfg *config.Config, projects []index.Project) Model {
	ic := nerdIcons
	if cfg.UI.Icons == "plain" {
		ic = plainIcons
	}
	ti := textinput.New()
	ti.Prompt = ic.prompt
	ti.PromptStyle = accent
	ti.Placeholder = "type to filter"
	ti.PlaceholderStyle = dim
	ti.Focus()
	m := Model{
		cfg:        cfg,
		all:        projects,
		ic:         ic,
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
		if m.mode == modePreview && !m.previewVisible() {
			m.mode = modeList
			m.input.Focus()
		}
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
		if m.mode == modeSettings {
			return m.updateSettings(msg)
		}
		if m.mode == modePreview {
			if next, cmd, handled := m.updatePreview(msg); handled {
				return next, cmd
			}
		}
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyCtrlE:
			return m.openSettings()
		case tea.KeyRight:
			// Focus the preview only when the input cursor is already at the
			// end; otherwise → keeps editing the filter text.
			if m.previewVisible() && m.input.Position() >= len([]rune(m.input.Value())) {
				m.mode = modePreview
				m.input.Blur()
				return m, nil
			}
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
		case tea.KeyTab:
			return m.cycleScope(1)
		case tea.KeyShiftTab:
			return m.cycleScope(-1)
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
	if m.mode == modeSettings {
		return m, nil
	}
	inPreview := m.previewVisible() && msg.X > m.listWidth()
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if inPreview {
			m.scrollPreview(-3)
			return m, nil
		}
		return m.move(-3)
	case tea.MouseButtonWheelDown:
		if inPreview {
			m.scrollPreview(3)
			return m, nil
		}
		return m.move(3)
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		if inPreview {
			m.mode = modePreview
			m.input.Blur()
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
		m.mode = modeList
		m.input.Focus()
		m.cursor = idx
		m.previewOff = 0
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
	m.previewOff = 0
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
	m.cfg.MarkSettingsHintSeen()
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

// openSettings switches to the settings page and dismisses the first-run hint.
func (m Model) openSettings() (tea.Model, tea.Cmd) {
	m.mode = modeSettings
	m.input.Blur()
	m.cfg.MarkSettingsHintSeen()
	return m, nil
}

// settingOptions lists the cycleable values per settings row.
var settingOptions = [][]string{
	{config.ActionCD, config.ActionClaude, config.ActionCodex}, // default action
	{"nerd", "plain"}, // icons
}

func (m Model) settingValue(row int) string {
	if row == 0 {
		return m.cfg.Defaults.Action
	}
	return m.cfg.UI.Icons
}

// cycleSetting steps the selected setting's value and persists it.
func (m Model) cycleSetting(delta int) (tea.Model, tea.Cmd) {
	opts := settingOptions[m.settingsRow]
	cur := 0
	for i, v := range opts {
		if v == m.settingValue(m.settingsRow) {
			cur = i
			break
		}
	}
	next := opts[(cur+delta+len(opts))%len(opts)]
	switch m.settingsRow {
	case 0:
		_ = m.cfg.SetDefaultAction(next)
	case 1:
		_ = m.cfg.SetIcons(next)
		m.ic = nerdIcons
		if next == "plain" {
			m.ic = plainIcons
		}
		m.input.Prompt = m.ic.prompt
	}
	return m, nil
}

func (m Model) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc, tea.KeyCtrlE:
		m.mode = modeList
		m.input.Focus()
		return m, nil
	case tea.KeyUp, tea.KeyCtrlP, tea.KeyCtrlK:
		m.settingsRow = max(0, m.settingsRow-1)
		return m, nil
	case tea.KeyDown, tea.KeyCtrlN, tea.KeyCtrlJ:
		m.settingsRow = min(len(settingOptions)-1, m.settingsRow+1)
		return m, nil
	case tea.KeyLeft:
		return m.cycleSetting(-1)
	case tea.KeyRight, tea.KeyEnter, tea.KeySpace:
		return m.cycleSetting(1)
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "k":
			m.settingsRow = max(0, m.settingsRow-1)
		case "j":
			m.settingsRow = min(len(settingOptions)-1, m.settingsRow+1)
		case "h":
			return m.cycleSetting(-1)
		case "l", " ":
			return m.cycleSetting(1)
		}
		return m, nil
	}
	return m, nil
}

// updatePreview handles keys owned by the focused preview pane; unhandled
// keys (enter, ctrl-actions, tab) fall through to the global bindings.
func (m Model) updatePreview(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	back := func() (tea.Model, tea.Cmd, bool) {
		m.mode = modeList
		m.input.Focus()
		return m, nil, true
	}
	switch msg.Type {
	case tea.KeyEsc, tea.KeyLeft:
		return back()
	case tea.KeyUp:
		m.scrollPreview(-1)
		return m, nil, true
	case tea.KeyDown:
		m.scrollPreview(1)
		return m, nil, true
	case tea.KeyPgUp:
		m.scrollPreview(-m.listHeight())
		return m, nil, true
	case tea.KeyPgDown:
		m.scrollPreview(m.listHeight())
		return m, nil, true
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "k":
			m.scrollPreview(-1)
		case "j":
			m.scrollPreview(1)
		case "g":
			m.previewOff = 0
		case "G":
			m.scrollPreview(1 << 20)
		case "h", "q":
			return back()
		}
		return m, nil, true // swallow runes: they are not filter input here
	}
	return m, nil, false
}

// scrollPreview moves the preview offset, clamped to the rendered content.
func (m *Model) scrollPreview(delta int) {
	lines := m.renderPreviewLines(m.previewWidth() - 4)
	maxOff := max(0, len(lines)-m.listHeight())
	m.previewOff = min(max(0, m.previewOff+delta), maxOff)
}

// cycleScope steps the agent filter (all → claude → codex) and refilters.
func (m Model) cycleScope(delta int) (tea.Model, tea.Cmd) {
	m.scope = scope((int(m.scope) + delta + int(scopeCount)) % int(scopeCount))
	m.refilter()
	return m, m.requestPreview()
}

// inScope reports whether a project has sessions from the scoped agent.
func (m Model) inScope(p index.Project) bool {
	switch m.scope {
	case scopeClaude:
		return !p.ClaudeLast.IsZero()
	case scopeCodex:
		return !p.CodexLast.IsZero()
	}
	return true
}

// refilter recomputes the visible list from the scope and query, preserving
// base order (pinned first, then recency) and ranking better fuzzy matches
// higher.
func (m *Model) refilter() {
	query := strings.ToLower(strings.TrimSpace(m.input.Value()))
	m.filtered = m.filtered[:0]
	m.matchFor = m.matchFor[:0]
	if query == "" {
		for i := range m.all {
			if !m.inScope(m.all[i]) {
				continue
			}
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
			if !m.inScope(p) {
				continue
			}
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
	m.previewOff = 0
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

// listWidth leaves one gap column between the two panels when both show.
func (m Model) listWidth() int {
	if !m.previewVisible() {
		return m.width
	}
	return m.width - m.previewWidth() - 1
}

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
	if m.mode == modeSettings {
		body := panel(m.ic.settings, titleOn, m.renderSettings(), m.width, h, 0, 0)
		return m.renderHeader() + "\n" + strings.Join(body, "\n") + "\n" + m.renderHelp()
	}
	thumbStart, thumbLen := thumbFor(m.offset, h, len(m.filtered))
	title, titleSt := m.listTitle()
	if m.mode == modePreview {
		titleSt = titleOff
	}
	list := panel(title, titleSt, m.renderRows(h), m.listWidth(), h, thumbStart, thumbLen)
	if m.previewVisible() {
		plines := m.renderPreviewLines(m.previewWidth() - 4)
		off := min(m.previewOff, max(0, len(plines)-h))
		pThumbStart, pThumbLen := 0, 0
		pTitle := titleOff
		if m.mode == modePreview {
			pTitle = titleOn
			pThumbStart, pThumbLen = thumbFor(off, h, len(plines))
		}
		prev := panel(m.ic.preview, pTitle, plines[off:min(len(plines), off+h)], m.previewWidth(), h, pThumbStart, pThumbLen)
		for i := range list {
			list[i] += " " + prev[i]
		}
	}
	return m.renderHeader() + "\n" + strings.Join(list, "\n") + "\n" + m.renderHelp()
}

func (m Model) renderHeader() string {
	left := " " + m.input.View()
	counter := chipSt.Render(fmt.Sprintf(" %d/%d ", len(m.filtered), len(m.all)))
	if !m.cfg.SettingsHintSeen() && m.mode != modeSettings {
		counter = keySt.Render(" ^e settings ") + " " + counter
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(counter) - 2
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + counter
}

// listTitle names the list panel after the active scope, tinting the chip
// with the scoped agent's color.
func (m Model) listTitle() (string, lipgloss.Style) {
	switch m.scope {
	case scopeClaude:
		return m.ic.claude + "Claude", titleClaude
	case scopeCodex:
		return m.ic.codex + "Codex", titleCodex
	}
	return "Projects", titleOn
}

// panel wraps content lines in a rounded border with a chip-style title.
// When thumbLen > 0, the right border doubles as a scrollbar: rows within
// [thumbStart, thumbStart+thumbLen) get a thumb.
// Content lines must already be at most w-4 display cells wide.
func panel(title string, ts lipgloss.Style, content []string, w, h, thumbStart, thumbLen int) []string {
	inner := max(2, w-2)
	t := ts.Render(" " + title + " ")
	fill := max(0, inner-1-lipgloss.Width(t))
	out := make([]string, 0, h+2)
	out = append(out, borderSt.Render("╭─")+t+borderSt.Render(strings.Repeat("─", fill)+"╮"))
	side := borderSt.Render("│")
	thumb := thumbSt.Render("┃")
	for i := 0; i < h; i++ {
		line := ""
		if i < len(content) {
			line = content[i]
		}
		if pad := inner - 2 - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		right := side
		if thumbLen > 0 && i >= thumbStart && i < thumbStart+thumbLen {
			right = thumb
		}
		out = append(out, side+" "+line+" "+right)
	}
	out = append(out, borderSt.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return out
}

// thumbFor maps a scroll window onto a thumb on a panel's right border;
// zero length means everything fits and no thumb is drawn.
func thumbFor(offset, h, total int) (start, length int) {
	if h <= 0 || total <= h {
		return 0, 0
	}
	length = max(1, h*h/total)
	maxOff := total - h
	start = (offset*(h-length) + maxOff/2) / maxOff
	return start, length
}

func (m Model) renderRows(h int) []string {
	cw := max(10, m.listWidth()-4)
	out := make([]string, 0, h)
	if len(m.filtered) == 0 {
		what, hint := "nothing matches", "ctrl+u clears the filter"
		if m.input.Value() == "" {
			switch m.scope {
			case scopeClaude:
				what, hint = "no claude projects", "tab switches scope"
			case scopeCodex:
				what, hint = "no codex projects", "tab switches scope"
			}
		}
		out = append(out, "",
			dim.Render("  ◌  "+what),
			dim.Render("     "+hint))
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
	gitW := 15

	var b strings.Builder
	if selected {
		b.WriteString(seg(accent).Render("▌ "))
	} else {
		b.WriteString(seg(plain).Render("  "))
	}
	if p.Pinned {
		b.WriteString(seg(pinSt).Render(m.ic.pin))
	} else {
		b.WriteString(seg(dim).Render(m.ic.folder))
	}

	namePos := match.positions
	if !match.onName {
		namePos = nil
	}
	baseName := nameSt
	if selected {
		baseName = nameSelSt
	}
	name := highlight(p.Name, namePos, seg(baseName), seg(matchSt), nameW)
	b.WriteString(name)
	b.WriteString(seg(plain).Render(strings.Repeat(" ", max(0, nameW-lipgloss.Width(name))+1)))

	if showBadge {
		switch p.LastAgent {
		case index.AgentClaude:
			b.WriteString(seg(claudeSt).Render(m.ic.claude + "claude"))
		case index.AgentCodex:
			b.WriteString(seg(codexSt).Render(m.ic.codex + "codex "))
		default:
			b.WriteString(seg(plain).Render("        "))
		}
		when := index.RelTime(p.LastUsed, m.now)
		b.WriteString(seg(dim).Render(fmt.Sprintf(" %4s", when)))
	}

	if showGit {
		branch, dirty := "", ""
		if st, ok := m.git[p.Path]; ok && st.IsRepo {
			branch = m.ic.branch + truncate(st.Branch, 9)
			if st.Dirty > 0 {
				dirty = fmt.Sprintf(" +%d", min(st.Dirty, 99))
			}
		}
		b.WriteString(seg(plain).Render("  "))
		b.WriteString(seg(gitSt).Render(branch))
		b.WriteString(seg(dirtySt).Render(dirty))
		used := lipgloss.Width(branch) + lipgloss.Width(dirty)
		b.WriteString(seg(plain).Render(strings.Repeat(" ", max(0, gitW-used))))
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

// renderPreviewLines renders the full preview content (unclipped); View
// slices it by the scroll offset.
func (m Model) renderPreviewLines(cw int) []string {
	p := m.selectedProject()
	if p == nil || cw < 10 {
		return nil
	}
	wrap := lipgloss.NewStyle().Width(cw)
	var lines []string
	add := func(s string) { lines = append(lines, strings.Split(s, "\n")...) }

	title := truncate(p.Name, cw-2)
	if p.Pinned {
		title = pinSt.Render(m.ic.pin) + nameSt.Render(title)
	} else {
		title = nameSt.Render(title)
	}
	add(title)
	add(dim.Render(wrap.Render(index.TildePath(p.Path))))
	add("")

	var usage []string
	if !p.ClaudeLast.IsZero() {
		usage = append(usage, claudeSt.Render(m.ic.claude+"claude ")+dim.Render(index.RelTime(p.ClaudeLast, m.now)+" ago"))
	}
	if !p.CodexLast.IsZero() {
		usage = append(usage, codexSt.Render(m.ic.codex+"codex ")+dim.Render(index.RelTime(p.CodexLast, m.now)+" ago"))
	}
	if len(usage) > 0 {
		add(strings.Join(usage, dim.Render("  ·  ")))
	}
	if st, ok := m.git[p.Path]; ok && st.IsRepo {
		line := gitSt.Render(m.ic.branch + st.Branch)
		if st.Dirty > 0 {
			line += dirtySt.Render(fmt.Sprintf("  ● %d uncommitted", st.Dirty))
		}
		add(line)
	}
	add("")
	action := m.cfg.ActionFor(p.Path)
	add(keySt.Render(" enter ") + dim.Render(" → ") + actionStyle(action).Render(action))
	add(dim.Render(strings.Repeat("┄", cw)))

	if snip, ok := m.previews[p.Path]; ok {
		if snip.Text != "" {
			add(labelSt.Render(fmt.Sprintf("last session · %s · %s", snip.Agent, snip.Role)))
			quote := borderSt.Render("▏ ")
			for _, l := range strings.Split(lipgloss.NewStyle().Width(cw-2).Render(snip.Text), "\n") {
				add(quote + l)
			}
		} else {
			add(labelSt.Render("no session preview"))
		}
	} else if p.LastAgent != "" {
		add(labelSt.Render("loading preview…"))
	}
	return lines
}

// renderSettings lays out the settings page: one row per option with the
// current value highlighted in a segmented control.
func (m Model) renderSettings() []string {
	cw := max(10, m.width-4)
	labels := []string{"default action", "icons"}
	notes := []string{
		"what enter does when a project has no per-project action",
		"nerd needs a Nerd Font (nerdfonts.com); plain works everywhere",
	}
	out := []string{""}
	for i, label := range labels {
		marker := "  "
		if i == m.settingsRow {
			marker = accent.Render("▌ ")
		}
		var cells []string
		for _, v := range settingOptions[i] {
			switch {
			case v == m.settingValue(i) && i == m.settingsRow:
				cells = append(cells, titleOn.Render(" "+v+" "))
			case v == m.settingValue(i):
				cells = append(cells, keySt.Render(" "+v+" "))
			default:
				cells = append(cells, dim.Render(" "+v+" "))
			}
		}
		pad := strings.Repeat(" ", max(1, 18-lipgloss.Width(label)))
		out = append(out, marker+nameSt.Render(label)+pad+strings.Join(cells, " "),
			dim.Render("                    "+truncate(notes[i], max(0, cw-20))), "")
	}
	out = append(out, "",
		dim.Render("  config   ")+labelSt.Render(truncate(index.TildePath(config.Path()), max(0, cw-11))),
		dim.Render(truncate("  changes edit just these keys in that file — the rest of it,", cw)),
		dim.Render(truncate("  comments included, is left byte-for-byte intact", cw)))
	return out
}

// actionStyle colors an action hint with the same hue as its agent badge.
func actionStyle(action string) lipgloss.Style {
	switch action {
	case config.ActionClaude:
		return claudeSt
	case config.ActionCodex:
		return codexSt
	default:
		return accent
	}
}

func (m Model) renderHelp() string {
	type item struct{ key, label string }
	var items []item
	switch m.mode {
	case modeSettings:
		items = []item{{"↑↓", "select"}, {"←→", "change"}, {"esc", "back"}}
	case modePreview:
		items = []item{
			{"↑↓", "scroll"}, {"←", "back"}, {"enter", "default"},
			{"^a", "claude"}, {"^x", "codex"}, {"^r", "resume"},
		}
	default:
		items = []item{
			{"enter", "default"}, {"tab", "scope"}, {"→", "preview"}, {"^a", "claude"},
			{"^x", "codex"}, {"^r", "resume"}, {"^s", "pin"}, {"^e", "settings"}, {"esc", "quit"},
		}
	}
	var parts []string
	for _, it := range items {
		parts = append(parts, keySt.Render(" "+it.key+" ")+dim.Render(" "+it.label))
	}
	full := " " + strings.Join(parts, "  ")
	if lipgloss.Width(full) <= m.width {
		return full
	}
	var keys []string
	for _, it := range items {
		keys = append(keys, keySt.Render(" "+it.key+" "))
	}
	return " " + strings.Join(keys, " ")
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

// darkBackground guesses the terminal background from COLORFGBG (set by
// iTerm2, Konsole, rxvt: "fg;bg"). We deliberately avoid termenv's OSC 11
// query — on terminals that never answer it, it blocks startup for its full
// timeout and can swallow the first keypress. Dark is the safe default.
func darkBackground() bool {
	v := os.Getenv("COLORFGBG")
	i := strings.LastIndex(v, ";")
	if i < 0 {
		return true
	}
	switch v[i+1:] {
	case "7", "15":
		return false
	}
	return true
}

// Run shows the picker on /dev/tty and returns the user's choice, or nil if
// cancelled.
func Run(cfg *config.Config, projects []index.Project) (*Result, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("perch pick needs an interactive terminal: %w", err)
	}
	defer tty.Close()
	// lipgloss's default renderer sniffs os.Stdout for color support, but the
	// shell wrapper redirects stdout (only the tty shows the UI) — so styles
	// would silently degrade to plain Ascii. Detect against the tty instead.
	renderer := lipgloss.DefaultRenderer()
	renderer.SetColorProfile(termenv.NewOutput(tty).EnvColorProfile())
	renderer.SetHasDarkBackground(darkBackground())
	prog := tea.NewProgram(New(cfg, projects),
		tea.WithInput(tty), tea.WithOutput(tty),
		tea.WithAltScreen(), tea.WithMouseCellMotion())
	final, err := prog.Run()
	if err != nil {
		return nil, err
	}
	return final.(Model).result, nil
}
