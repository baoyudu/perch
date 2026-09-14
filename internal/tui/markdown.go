package tui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// mdFlag marks inline emphasis on a cell; flags combine (bold code in a link).
type mdFlag uint8

const (
	mdBold mdFlag = 1 << iota
	mdItalic
	mdCode
	mdLink
)

type mdCell struct {
	r rune
	f mdFlag
}

// mdAtom is an unbreakable unit for wrapping: a Latin word, one CJK character
// (plus any closing punctuation glued to it), or a single space.
type mdAtom struct {
	cells []mdCell
	w     int
	space bool
}

// Punctuation that must not open a line; it sticks to whatever precedes it.
const noLineStart = "，。、；：？！）】》」』〕〉…”’,.;:!?)]}"

// renderMarkdown lays out a transcript snippet as styled lines no wider than
// w cells. It covers the markdown assistants actually emit — headings,
// bullet/numbered/task lists, fenced code, blockquotes, rules, and inline
// bold/italic/code/links — and passes anything else through as text. Parsing
// is line-oriented, so a snippet truncated mid-document renders sanely up to
// the cut.
func renderMarkdown(text string, w int) []string {
	var out []string
	inFence := false
	for _, raw := range strings.Split(text, "\n") {
		line := strings.ReplaceAll(raw, "\t", "  ")
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			indent := "  " + leadingSpaces(line)
			cells := plainCells(strings.TrimLeft(line, " "), mdCode)
			out = append(out, wrapCells(cells, w, indent, indent, plain)...)
			continue
		}
		out = append(out, renderBlock(line, trim, w)...)
	}
	return out
}

func renderBlock(line, trim string, w int) []string {
	indent := leadingSpaces(line)
	switch {
	case trim == "":
		return []string{""}
	case isRule(trim):
		return []string{dim.Render(strings.Repeat("─", w))}
	}
	if h, ok := heading(trim); ok {
		return wrapCells(parseInline(h), w, indent, indent, nameSt)
	}
	if q, ok := strings.CutPrefix(trim, ">"); ok {
		bar := indent + dim.Render("│ ")
		return wrapCells(parseInline(strings.TrimPrefix(q, " ")), w, bar, bar, dim)
	}
	if marker, rest, ok := listItem(trim); ok {
		hang := indent + strings.Repeat(" ", lipgloss.Width(marker))
		return wrapCells(parseInline(rest), w, indent+dim.Render(marker), hang, plain)
	}
	return wrapCells(parseInline(trim), w, indent, indent, plain)
}

func leadingSpaces(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " "))]
}

func isRule(trim string) bool {
	if len(trim) < 3 {
		return false
	}
	c := trim[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	return strings.Trim(trim, string(c)) == ""
}

func heading(trim string) (string, bool) {
	n := 0
	for n < len(trim) && trim[n] == '#' {
		n++
	}
	if n == 0 || n > 6 {
		return "", false
	}
	if n == len(trim) {
		return "", true
	}
	if trim[n] != ' ' {
		return "", false
	}
	return strings.TrimSpace(trim[n:]), true
}

// listItem recognises "- ", "* ", "+ ", "1. ", "1) " and task boxes, returning
// the marker to draw and the item text.
func listItem(trim string) (marker, rest string, ok bool) {
	if len(trim) >= 2 && strings.IndexByte("-*+", trim[0]) >= 0 && trim[1] == ' ' {
		rest = strings.TrimLeft(trim[2:], " ")
		marker = "• "
		switch {
		case strings.HasPrefix(rest, "[ ] "):
			marker, rest = "☐ ", rest[4:]
		case strings.HasPrefix(rest, "[x] "), strings.HasPrefix(rest, "[X] "):
			marker, rest = "☑ ", rest[4:]
		}
		return marker, rest, true
	}
	n := 0
	for n < len(trim) && n < 3 && trim[n] >= '0' && trim[n] <= '9' {
		n++
	}
	if n > 0 && n+1 < len(trim) && (trim[n] == '.' || trim[n] == ')') && trim[n+1] == ' ' {
		return trim[:n+1] + " ", strings.TrimLeft(trim[n+2:], " "), true
	}
	return "", "", false
}

func plainCells(s string, f mdFlag) []mdCell {
	cells := make([]mdCell, 0, len(s))
	for _, r := range s {
		cells = append(cells, mdCell{r, f})
	}
	return cells
}

func parseInline(s string) []mdCell { return inline([]rune(s), 0) }

func inline(r []rune, f mdFlag) []mdCell {
	var out []mdCell
	for i := 0; i < len(r); {
		switch {
		case r[i] == '`':
			n := 1
			for i+n < len(r) && r[i+n] == '`' {
				n++
			}
			if j := indexRun(r, i+n, '`', n); j >= 0 {
				for _, c := range r[i+n : j] {
					out = append(out, mdCell{c, f | mdCode})
				}
				i = j + n
				continue
			}
		case r[i] == '*' && i+1 < len(r) && r[i+1] == '*':
			if j := closeEmphasis(r, i+2, 2); j >= 0 {
				out = append(out, inline(r[i+2:j], f|mdBold)...)
				i = j + 2
				continue
			}
		case r[i] == '*':
			if j := closeEmphasis(r, i+1, 1); j >= 0 {
				out = append(out, inline(r[i+1:j], f|mdItalic)...)
				i = j + 1
				continue
			}
		case r[i] == '[':
			if txt, end, ok := link(r, i); ok {
				out = append(out, inline(txt, f|mdLink)...)
				i = end
				continue
			}
		case r[i] == '!' && i+1 < len(r) && r[i+1] == '[':
			if txt, end, ok := link(r, i+1); ok {
				out = append(out, inline(txt, f|mdLink)...)
				i = end
				continue
			}
		}
		out = append(out, mdCell{r[i], f})
		i++
	}
	return out
}

// indexRun finds a run of exactly n copies of c starting at or after from.
func indexRun(r []rune, from int, c rune, n int) int {
	for i := from; i+n <= len(r); i++ {
		if r[i] != c {
			continue
		}
		j := i
		for j < len(r) && r[j] == c {
			j++
		}
		if j-i == n {
			return i
		}
		i = j - 1
	}
	return -1
}

// closeEmphasis finds the closing run of n asterisks for an opener that ends
// just before from. The opener must hug text on its right and the closer on
// its left, so "2 * 3 * 4" and "a ** b" stay literal.
func closeEmphasis(r []rune, from, n int) int {
	if from >= len(r) || unicode.IsSpace(r[from]) || r[from] == '*' {
		return -1
	}
	for j := from + 1; j+n <= len(r); j++ {
		if r[j] != '*' || unicode.IsSpace(r[j-1]) {
			continue
		}
		k := j
		for k < len(r) && r[k] == '*' {
			k++
		}
		if k-j == n {
			return j
		}
		j = k - 1
	}
	return -1
}

// link parses "[text](target)" starting at the bracket; only the text is kept.
func link(r []rune, i int) (text []rune, end int, ok bool) {
	j := i + 1
	for j < len(r) && r[j] != ']' && r[j] != '[' {
		j++
	}
	if j+1 >= len(r) || r[j] != ']' || r[j+1] != '(' || j == i+1 {
		return nil, 0, false
	}
	k := j + 2
	for k < len(r) && r[k] != ')' && !unicode.IsSpace(r[k]) {
		k++
	}
	if k >= len(r) || r[k] != ')' {
		return nil, 0, false
	}
	return r[i+1 : j], k + 1, true
}

func cellWidth(r rune) int { return lipgloss.Width(string(r)) }

func tokenize(cells []mdCell) []mdAtom {
	var atoms []mdAtom
	push := func(a mdAtom) {
		// Closing punctuation hangs off the previous atom rather than opening a line.
		if n := len(atoms); n > 0 && !atoms[n-1].space && strings.ContainsRune(noLineStart, a.cells[0].r) {
			atoms[n-1].cells = append(atoms[n-1].cells, a.cells...)
			atoms[n-1].w += a.w
			return
		}
		atoms = append(atoms, a)
	}
	for i := 0; i < len(cells); {
		c := cells[i]
		switch {
		case unicode.IsSpace(c.r):
			atoms = append(atoms, mdAtom{cells: []mdCell{c}, w: 1, space: true})
			i++
		case cellWidth(c.r) > 1:
			push(mdAtom{cells: []mdCell{c}, w: cellWidth(c.r)})
			i++
		default:
			j, w := i, 0
			for j < len(cells) && !unicode.IsSpace(cells[j].r) && cellWidth(cells[j].r) <= 1 {
				w += cellWidth(cells[j].r)
				j++
			}
			push(mdAtom{cells: cells[i:j], w: w})
			i = j
		}
	}
	return atoms
}

// wrapCells word-wraps styled cells into lines of at most w cells. first and
// rest are pre-rendered prefixes for the opening and continuation lines. Latin
// words stay whole; CJK breaks between any two characters; an atom wider than
// a whole line is split by character.
func wrapCells(cells []mdCell, w int, first, rest string, base lipgloss.Style) []string {
	var lines []string
	var cur []mdCell
	curW := 0
	prefix := first
	avail := func() int { return max(1, w-lipgloss.Width(prefix)) }
	flush := func() {
		for len(cur) > 0 && unicode.IsSpace(cur[len(cur)-1].r) {
			cur = cur[:len(cur)-1]
		}
		lines = append(lines, strings.TrimRight(prefix+renderCells(cur, base), " "))
		cur, curW = nil, 0
		prefix = rest
	}
	for _, a := range tokenize(cells) {
		if a.space {
			if curW > 0 && curW+a.w <= avail() {
				cur = append(cur, a.cells...)
				curW += a.w
			}
			continue
		}
		if curW > 0 && curW+a.w > avail() {
			flush()
		}
		if a.w <= avail() {
			cur = append(cur, a.cells...)
			curW += a.w
			continue
		}
		for _, c := range a.cells {
			cw := cellWidth(c.r)
			if curW > 0 && curW+cw > avail() {
				flush()
			}
			cur = append(cur, c)
			curW += cw
		}
	}
	if curW > 0 || len(lines) == 0 {
		flush()
	}
	return lines
}

func renderCells(cells []mdCell, base lipgloss.Style) string {
	var b strings.Builder
	for i := 0; i < len(cells); {
		j := i
		var run strings.Builder
		for j < len(cells) && cells[j].f == cells[i].f {
			run.WriteRune(cells[j].r)
			j++
		}
		b.WriteString(mdStyle(base, cells[i].f).Render(run.String()))
		i = j
	}
	return b.String()
}

func mdStyle(base lipgloss.Style, f mdFlag) lipgloss.Style {
	st := base
	if f&mdCode != 0 {
		st = st.Foreground(codeC)
	}
	if f&mdBold != 0 {
		st = st.Bold(true)
	}
	if f&mdItalic != 0 {
		st = st.Italic(true)
	}
	if f&mdLink != 0 {
		st = st.Underline(true)
	}
	return st
}
