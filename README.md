# psw — ProjectSwitcher

Jump straight from a fresh terminal into the project you were just working on
with **Claude Code** or **Codex** — and optionally relaunch the agent, or even
resume the last conversation.

`psw` needs **zero configuration**: it reads the session history both agents
already keep on disk (`~/.claude.json` + `~/.claude/projects`,
`~/.codex/sessions`), so the moment you install it, every project you've ever
opened an agent in is one keystroke away.

```
❯ learn                                          │ learnitall
  3/55 projects                                  │ ~/Code/learnitall
▸ learnitall      ✳ claude  5d  main    ~/Code/… │ ✳ claude 5d ago · ◆ codex 8d ago
  learn-notes     ◆ codex   2w          ~/Doc/…  │  main  3 uncommitted
                                                 │ ────────────────────────────
                                                 │ last session · claude (assistant)
                                                 │ Done — the review scheduler now
                                                 │ retries failed fetches and all
                                                 │ 12 tests pass.
 enter default · ^o cd · ^a claude · ^x codex · ^r resume · ^s pin · esc quit
```

## Install

```sh
brew install baoyudu/tap/psw        # once released
# or from source:
go install github.com/baoyudu/psw/cmd/psw@latest
```

Add one line to your shell rc file:

```sh
eval "$(psw init zsh)"     # ~/.zshrc
eval "$(psw init bash)"    # ~/.bashrc
psw init fish | source     # ~/.config/fish/config.fish
```

Then type **`p`** in any terminal.

## Keys

| Key | Action |
|---|---|
| type | fuzzy-filter projects (name and path) |
| `Enter` | project's default action (global default: just `cd`) |
| `^O` | `cd` only |
| `^A` | `cd` + launch **claude** |
| `^X` | `cd` + launch **codex** |
| `^R` | `cd` + **resume** the last session (`claude --continue` / `codex resume <id>`) |
| `^S` | pin/unpin project to the top |
| `↑↓` `^P^N` `^K^J` | navigate |
| `Esc` / `^C` | cancel |

The list shows each project's last-used agent, relative time, git branch and
dirty-file count; the preview pane shows the tail of your last conversation so
you remember where you left off.

## Configuration

Optional, at `~/.config/psw/config.toml`:

```toml
# top-level keys must come before any [table]
ignore = ["**/.worktrees/**", "**/.claude/worktrees/**", "**/.claude-worktrees/**", "**/node_modules/**"]

[defaults]
action = "cd"          # what Enter does: cd | claude | codex
command = "p"          # name of the shell function
claude_args = []       # extra args whenever claude is launched
codex_args = []        # extra args whenever codex is launched

[projects."/Users/you/Code/my-app"]
action = "claude"      # Enter here means: cd + claude
args = ["--dangerously-skip-permissions"]
pinned = true
```

Pins toggled with `^S` are stored in `~/.config/psw/state.json` and override
the TOML value, so your hand-written config is never rewritten.

## Commands

| Command | Purpose |
|---|---|
| `psw init <zsh\|bash\|fish>` | print the shell wrapper function |
| `psw pick` | the interactive picker (called by the wrapper) |
| `psw list [--json]` | print the merged project list |
| `psw pin <path>` / `psw unpin <path>` | manage pins from the CLI |
| `psw doctor` | check data sources, binaries, and shell integration |

## How it works

A child process can't change its parent shell's directory, so `psw pick`
renders the TUI on `/dev/tty` and prints two lines to stdout — the chosen
directory and the command to run (`-` for none). The tiny function from
`psw init` does the actual `cd` and launch. Codex session metadata is indexed
incrementally into `~/.cache/psw/` so startup stays instant.

## 中文速览

打开新终端后输入 `p`，即可模糊搜索最近用 Claude Code / Codex 工作过的项目，
回车直达；`^A`/`^X` 在进入目录的同时启动对应 agent，`^R` 直接续接上次对话。
无需任何配置——项目列表来自两个工具自己的会话历史。安装后在 `~/.zshrc`
里加一行 `eval "$(psw init zsh)"` 即可。

## License

MIT
