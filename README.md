<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/logo-dark.png">
    <img src="docs/logo.png" alt="perch" width="400">
  </picture>
</p>

<p align="center"><i>a perch for your agent projects</i></p>

<p align="center"><b>English</b> | <a href="README.zh-CN.md">中文</a></p>

Jump straight from a fresh terminal into the project you were just working on
with **Claude Code** or **Codex** — and optionally relaunch the agent, or even
resume the last conversation.

`perch` needs **zero configuration**: it reads the session history both agents
already keep on disk (`~/.claude.json` + `~/.claude/projects`,
`~/.codex/sessions`), so the moment you install it, every project you've ever
opened an agent in is one keystroke away.

![perch demo](docs/demo/demo.gif)


## Install

```sh
brew install --cask baoyudu/tap/perch   # macOS
# or from source:
go install github.com/baoyudu/perch/cmd/perch@latest
```

Add one line to your shell rc file:

```sh
eval "$(perch init zsh)"     # ~/.zshrc
eval "$(perch init bash)"    # ~/.bashrc
perch init fish | source     # ~/.config/fish/config.fish
```

Then type **`p`** in any terminal.

## Keys

| Key | Action |
|---|---|
| type | fuzzy-filter projects (name and path) |
| `Tab` / `Shift+Tab` | cycle source scope: all → claude → codex |
| `Enter` | project's default action (global default: just `cd`) |
| `^O` | `cd` only |
| `^A` | `cd` + launch **claude** |
| `^X` | `cd` + launch **codex** |
| `^R` | `cd` + **resume** the last session (`claude --continue` / `codex resume <id>`) |
| `^S` | pin/unpin project to the top |
| `^T` | create a new project under `projects_dir` |
| `^E` | settings page (default action, icon set, projects dir) |
| `→` | focus the preview pane: `↑↓`/`jk` scroll, `←`/`Esc` back |
| `↑↓` `^P^N` `^K^J` | navigate |
| `Esc` / `^C` | cancel |

The list shows each project's last-used agent, relative time, git branch and
dirty-file count; the preview pane shows the tail of your last conversation so
you remember where you left off. `Tab` narrows the list to projects with
claude or codex sessions — the panel title changes color to show the scope.

Icons default to [Nerd Font](https://www.nerdfonts.com) glyphs; if your
terminal font isn't patched, set `icons = "plain"` under `[ui]`.

## Configuration

Optional, at `~/.config/perch/config.toml`:

```toml
# top-level keys must come before any [table]
ignore = ["**/.worktrees/**", "**/.claude/worktrees/**", "**/.claude-worktrees/**", "**/node_modules/**"]

[defaults]
action = "cd"          # what Enter does: cd | claude | codex | resume
command = "p"          # name of the shell function
projects_dir = "~/Code" # where ^T creates new projects
claude_args = []       # extra args whenever claude is launched
codex_args = []        # extra args whenever codex is launched

[ui]
icons = "nerd"         # "nerd" (default, needs a Nerd Font) | "plain"

[projects."/Users/you/Code/my-app"]
action = "claude"      # Enter here means: cd + claude
args = ["--dangerously-skip-permissions"]
pinned = true
```

You rarely need to edit the file: `^E` inside the picker opens a settings
page for the common options. It edits `config.toml` in place, touching only
the keys it owns — comments, formatting, and everything else in the file are
preserved. Pins toggled with `^S` are runtime state and live in
`~/.config/perch/state.json`.

## Commands

| Command | Purpose |
|---|---|
| `perch init <zsh\|bash\|fish>` | print the shell wrapper function |
| `perch pick` | the interactive picker (called by the wrapper) |
| `perch list [--json]` | print the merged project list |
| `perch pin <path>` / `perch unpin <path>` | manage pins from the CLI |
| `perch doctor` | check data sources, binaries, and shell integration |

## How it works

A child process can't change its parent shell's directory, so `perch pick`
renders the TUI on `/dev/tty` and prints two lines to stdout — the chosen
directory and the command to run (`-` for none). The tiny function from
`perch init` does the actual `cd` and launch. Codex session metadata is indexed
incrementally into `~/.cache/perch/` so startup stays instant.

## License

MIT
