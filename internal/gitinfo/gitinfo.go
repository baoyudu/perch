// Package gitinfo fetches lightweight git status for project rows.
package gitinfo

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

type Status struct {
	IsRepo bool
	Branch string
	Dirty  int // changed/untracked entries in porcelain output
}

// sem bounds concurrent git subprocesses across async TUI loads.
var sem = make(chan struct{}, 8)

// Load runs a single `git status --porcelain --branch` with a timeout.
// Non-repos and errors return a zero Status.
func Load(dir string) Status {
	sem <- struct{}{}
	defer func() { <-sem }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain", "--branch")
	out, err := cmd.Output()
	if err != nil {
		return Status{}
	}
	return parse(string(out))
}

func parse(out string) Status {
	s := Status{IsRepo: true}
	for i, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if i == 0 && strings.HasPrefix(line, "## ") {
			branch := strings.TrimPrefix(line, "## ")
			if idx := strings.Index(branch, "..."); idx >= 0 {
				branch = branch[:idx]
			}
			// Detached HEAD renders as "HEAD (no branch)".
			branch = strings.TrimSuffix(branch, " (no branch)")
			if idx := strings.Index(branch, " "); idx >= 0 {
				branch = branch[:idx]
			}
			s.Branch = branch
			continue
		}
		if line != "" {
			s.Dirty++
		}
	}
	return s
}
