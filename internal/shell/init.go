// Package shell emits the wrapper function users eval in their rc file.
// The wrapper exists because a child process cannot change its parent
// shell's cwd: psw pick prints "<dir>\n<command>" and the function acts on it.
package shell

import (
	"fmt"
	"strings"
)

// Init returns the integration script for the given shell, defining a
// function named fn (default "p").
func Init(sh, fn string) (string, error) {
	if fn == "" {
		fn = "p"
	}
	switch sh {
	case "zsh", "bash":
		return strings.ReplaceAll(posixTemplate, "__FN__", fn), nil
	case "fish":
		return strings.ReplaceAll(fishTemplate, "__FN__", fn), nil
	default:
		return "", fmt.Errorf("unsupported shell %q (want zsh, bash, or fish)", sh)
	}
}

const posixTemplate = `# psw shell integration — add to your rc file:
#   eval "$(psw init zsh)"
__FN__() {
    local out dir cmd
    out="$(command psw pick "$@")" || return $?
    [ -z "$out" ] && return 0
    dir="${out%%$'\n'*}"
    cmd="${out#*$'\n'}"
    [ "$cmd" = "$out" ] && cmd="-"
    cd -- "$dir" || return $?
    if [ -n "$cmd" ] && [ "$cmd" != "-" ]; then
        eval "$cmd"
    fi
}
`

const fishTemplate = `# psw shell integration — add to config.fish:
#   psw init fish | source
function __FN__ --description "switch to a recent agent project"
    set -l out (command psw pick $argv)
    or return $status
    if test (count $out) -eq 0
        return 0
    end
    set -l dir $out[1]
    set -l cmd "-"
    if test (count $out) -ge 2
        set cmd $out[2]
    end
    cd -- $dir
    or return $status
    if test "$cmd" != "-"
        eval $cmd
    end
end
`

// Quote makes a string safe to embed in the emitted command line.
func Quote(s string) string {
	if s == "" {
		return "''"
	}
	if isSafe(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func isSafe(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("_@%+=:,./-", r):
		default:
			return false
		}
	}
	return true
}

// Command joins argv into a single safely-quoted command string.
func Command(argv ...string) string {
	quoted := make([]string, len(argv))
	for i, a := range argv {
		quoted[i] = Quote(a)
	}
	return strings.Join(quoted, " ")
}
