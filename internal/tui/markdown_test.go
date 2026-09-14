package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Tests run without a tty, so lipgloss renders in Ascii: no escape codes,
// which lets these compare the laid-out text directly.

func TestMarkdownInlineMarkersStripped(t *testing.T) {
	cases := map[string]string{
		"是**私有仓库**，最后一次":              "是私有仓库，最后一次",
		"用 `pip install anybase` 装":   "用 pip install anybase 装",
		"看 [perch](https://x.y/z) 项目": "看 perch 项目",
		"*强调*一下":                      "强调一下",
		"a **b":                       "a **b",
		"2 * 3 * 4":                   "2 * 3 * 4",
		"a ** b **":                   "a ** b **",
		"`unclosed":                   "`unclosed",
		"**bold with `code`**":        "bold with code",
	}
	for in, want := range cases {
		got := renderMarkdown(in, 80)
		if len(got) != 1 || got[0] != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

func TestMarkdownBlocks(t *testing.T) {
	cases := map[string]string{
		"## 结论":        "结论",
		"- 本地有提交":      "• 本地有提交",
		"1. **只给**":    "1. 只给",
		"12) 项":        "12) 项",
		"- [x] done":   "☑ done",
		"- [ ] todo":   "☐ todo",
		"> 引用":         "│ 引用",
		"  - nested":   "  • nested",
		"---":          strings.Repeat("─", 30),
		"2026. a year": "2026. a year",
	}
	for in, want := range cases {
		got := renderMarkdown(in, 30)
		if len(got) != 1 || got[0] != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

func TestMarkdownFencesDroppedAndTruncationSafe(t *testing.T) {
	got := renderMarkdown("先装：\n```bash\nbrew install perch\n  eval \"$(perch init zsh)\"\n```\n然后 **跑** p", 60)
	want := []string{"先装：", "  brew install perch", "    eval \"$(perch init zsh)\"", "然后 跑 p"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q, want %q", got, want)
	}
	// A snippet cut off inside a fence keeps rendering the rest as code.
	got = renderMarkdown("```\ngo test ./...\n- not a bullet…", 60)
	want = []string{"  go test ./...", "  - not a bullet…"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("truncated fence: got %q, want %q", got, want)
	}
}

func TestMarkdownListHangingIndent(t *testing.T) {
	got := renderMarkdown("- 本地有 4 个提交没推上去，包括写入边界、批量撤回和冲突任务开关。", 20)
	if len(got) < 3 {
		t.Fatalf("expected wrapping, got %q", got)
	}
	if !strings.HasPrefix(got[0], "• ") {
		t.Errorf("first line %q should start with the bullet", got[0])
	}
	for _, l := range got[1:] {
		if !strings.HasPrefix(l, "  ") || strings.HasPrefix(l, "   ") {
			t.Errorf("continuation %q should hang under the text, not the bullet", l)
		}
	}
	got = renderMarkdown("10. "+strings.Repeat("word ", 12), 24)
	for _, l := range got[1:] {
		if !strings.HasPrefix(l, "    w") {
			t.Errorf("numbered continuation %q should indent by the marker width", l)
		}
	}
}

func TestMarkdownWrapRespectsWidthAndWords(t *testing.T) {
	text := "所以 `pip install anybase` 装不到，`git clone` 也会被拒。README 里 `npx skills add https://github.com/baoyudu/AnyBase` 那条也要等仓库公开才能用。"
	for _, w := range []int{12, 20, 33, 48} {
		for _, l := range renderMarkdown(text, w) {
			if lipgloss.Width(l) > w {
				t.Errorf("w=%d: line %q is %d wide", w, l, lipgloss.Width(l))
			}
			if strings.HasPrefix(l, " ") || strings.HasSuffix(l, " ") {
				t.Errorf("w=%d: line %q has dangling space", w, l)
			}
		}
	}
	got := renderMarkdown("alpha beta gamma delta", 11)
	if strings.Join(got, "|") != "alpha beta|gamma delta" {
		t.Errorf("latin words should wrap at spaces, got %q", got)
	}
	got = renderMarkdown(strings.Repeat("x", 25), 10)
	if strings.Join(got, "|") != "xxxxxxxxxx|xxxxxxxxxx|xxxxx" {
		t.Errorf("overlong word should hard-split, got %q", got)
	}
}

func TestMarkdownCJKPunctuationNeverOpensLine(t *testing.T) {
	got := renderMarkdown("你好世界，再见", 8)
	if strings.Join(got, "|") != "你好世|界，再见" {
		t.Fatalf("got %q", got)
	}
	for _, l := range renderMarkdown(strings.Repeat("字，", 30), 13) {
		if strings.HasPrefix(l, "，") {
			t.Errorf("line %q opens with closing punctuation", l)
		}
	}
}

func TestMarkdownEmptyLinesKept(t *testing.T) {
	got := renderMarkdown("a\n\nb", 20)
	if strings.Join(got, "|") != "a||b" {
		t.Fatalf("got %q", got)
	}
}

func TestMarkdownStylesInTrueColor(t *testing.T) {
	r := lipgloss.DefaultRenderer()
	prev := r.ColorProfile()
	r.SetColorProfile(termenv.TrueColor)
	r.SetHasDarkBackground(true)
	t.Cleanup(func() { r.SetColorProfile(prev) })
	got := renderMarkdown("run `go test` **now**", 40)[0]
	if code := lipgloss.NewStyle().Foreground(codeC).Render("go test"); !strings.Contains(got, code) {
		t.Errorf("code span should use the teal palette entry: %q", got)
	}
	if !strings.Contains(got, "\x1b[1m") {
		t.Errorf("bold span should be bold: %q", got)
	}
	if lipgloss.Width(got) != len("run go test now") {
		t.Errorf("styling must not change the measured width: %d", lipgloss.Width(got))
	}
}

func BenchmarkRenderMarkdown(b *testing.B) {
	text := strings.Repeat("- 本地有 **4 个提交**没推上去，包括 `0.6` 的\"合并改为 claim\"、写入边界、批量撤回和冲突任务开关。\n", 7)
	for i := 0; i < b.N; i++ {
		renderMarkdown(text, 44)
	}
}
