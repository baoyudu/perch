package index

import (
	"testing"
	"time"
)

func TestIgnored(t *testing.T) {
	patterns := []string{
		"**/.worktrees/**",
		"**/.claude/worktrees/**",
		"**/.claude-worktrees/**",
	}
	ignored := []string{
		"/Users/bao/Code/Sotawise/.worktrees/twitter-signal-source",
		"/Users/bao/Code/x/.claude/worktrees/fix",
		"/Users/bao/Code/x/.claude-worktrees/fix/deep",
	}
	kept := []string{
		"/Users/bao/Code/Sotawise",
		"/Users/bao/Code/worktrees-tool",
	}
	for _, p := range ignored {
		if !Ignored(patterns, p) {
			t.Errorf("expected %q to be ignored", p)
		}
	}
	for _, p := range kept {
		if Ignored(patterns, p) {
			t.Errorf("expected %q to be kept", p)
		}
	}
}

func TestRelTime(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	cases := map[time.Time]string{
		{}:                              "-",
		now.Add(-30 * time.Second):      "now",
		now.Add(-5 * time.Minute):       "5m",
		now.Add(-3 * time.Hour):         "3h",
		now.Add(-49 * time.Hour):        "2d",
		now.Add(-21 * 24 * time.Hour):   "3w",
		now.Add(-2 * 365 * 24 * time.Hour): "2y",
	}
	for in, want := range cases {
		if got := RelTime(in, now); got != want {
			t.Errorf("RelTime(%v) = %q, want %q", in, got, want)
		}
	}
}
