package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/baoyudu/perch/internal/gitinfo"
	"github.com/baoyudu/perch/internal/preview"
)

// TestBodyLinesFillWidth renders with git + preview data present and asserts
// every panel line is exactly as wide as the terminal, so the two panels'
// borders always line up.
func TestBodyLinesFillWidth(t *testing.T) {
	for _, ic := range []iconSet{nerdIcons, plainIcons} {
		for _, size := range []struct{ w, h int }{{120, 30}, {100, 24}, {80, 20}, {60, 16}} {
			m := testModel(t)
			m.ic = ic
			next, _ := m.Update(tea.WindowSizeMsg{Width: size.w, Height: size.h})
			m = next.(Model)
			next, _ = m.Update(gitMsg{"/w/prior-analyst", gitinfo.Status{IsRepo: true, Branch: "main", Dirty: 3}})
			m = next.(Model)
			next, _ = m.Update(previewMsg{"/w/prior-analyst", preview.Snippet{
				Agent: "claude", Role: "assistant",
				Text: strings.Repeat("a fairly long snippet of assistant text ", 6),
			}})
			m = next.(Model)

			check := func(label string) {
				lines := strings.Split(m.View(), "\n")
				// Body lines sit between the header and the help bar.
				for i, line := range lines[1 : len(lines)-1] {
					if got := lipgloss.Width(line); got != size.w {
						t.Errorf("%s %dx%d: body line %d is %d cells wide, want %d:\n%q",
							label, size.w, size.h, i+1, got, size.w, line)
					}
				}
			}
			check("list")
			if m.previewVisible() {
				next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
				m = next.(Model)
				next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
				m = next.(Model)
				check("preview-focused")
			}
			next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
			m = next.(Model)
			check("settings")
		}
	}
}
