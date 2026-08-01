package shell

import (
	"strings"
	"testing"
)

func TestQuote(t *testing.T) {
	cases := map[string]string{
		"claude":                  "claude",
		"--continue":              "--continue",
		"/a/b c":                  "'/a/b c'",
		"it's":                    `'it'\''s'`,
		"":                        "''",
		"$HOME":                   "'$HOME'",
		"a;rm -rf /":              "'a;rm -rf /'",
	}
	for in, want := range cases {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCommand(t *testing.T) {
	got := Command("claude", "--continue", "--add-dir", "/tmp/x y")
	want := "claude --continue --add-dir '/tmp/x y'"
	if got != want {
		t.Errorf("Command = %q, want %q", got, want)
	}
}

func TestInit(t *testing.T) {
	for _, sh := range []string{"zsh", "bash", "fish"} {
		out, err := Init(sh, "pj")
		if err != nil {
			t.Fatalf("Init(%s): %v", sh, err)
		}
		if !strings.Contains(out, "pj") || !strings.Contains(out, "psw pick") {
			t.Errorf("Init(%s) missing function name or pick call:\n%s", sh, out)
		}
	}
	if _, err := Init("powershell", "p"); err == nil {
		t.Error("expected error for unsupported shell")
	}
}
