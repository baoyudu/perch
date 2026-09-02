#!/usr/bin/env bash
# Builds a fully fabricated $DEMO home for recording README media.
# No real project names or history are used — safe to publish.
set -euo pipefail

DEMO="${1:-/tmp/perch-demo}"
rm -rf "$DEMO"
H="$DEMO/home"
mkdir -p "$H/.claude/projects" "$H/.codex/sessions/2026/09/02" "$DEMO/config" "$DEMO/cache"

# ts <spec> — BSD/GNU-portable "N units ago" in the given format
ts() { date -v"$1" +"$2" 2>/dev/null || date -d "${1#-}${3:- ago}" +"$2"; }

# must match Go's EncodeClaudeDir: one '-' per non-alphanumeric *rune*
enc() { python3 -c 'import sys,re; print(re.sub(r"[^a-zA-Z0-9]", "-", sys.argv[1]))' "$1"; }

repo() { # repo <dir> <branch> <dirty-count>
  git -C "$1" init -q -b "$2"
  git -C "$1" -c user.email=demo@perch -c user.name=demo commit -q --allow-empty -m init
  for ((i = 1; i <= $3; i++)); do echo change > "$1/wip-$i.txt"; done # NB: BSD `seq 1 0` counts down
}

claude_proj() { # claude_proj <path> <ago(-2M style)> <preview text>
  local p="$1" ago="$2" text="$3" dir
  mkdir -p "$p"
  dir="$H/.claude/projects/$(enc "$p")"
  mkdir -p "$dir"
  printf '{"message":{"role":"user","content":"keep going"}}\n{"message":{"role":"assistant","content":[{"type":"text","text":"%s"}]}}\n' "$text" > "$dir/session.jsonl"
  touch -t "$(ts "$ago" %Y%m%d%H%M.%S)" "$dir/session.jsonl"
  CLAUDE_PATHS+=("$p")
}

codex_proj() { # codex_proj <path> <ago> <preview text>
  local p="$1" ago="$2" text="$3" f id
  mkdir -p "$p"
  id="0199aaaa-$(printf '%04x' $RANDOM)-7000-8000-demo00000000"
  f="$H/.codex/sessions/2026/09/02/rollout-$(ts "$ago" %Y-%m-%dT%H-%M-%S)-$id.jsonl"
  printf '{"type":"session_meta","payload":{"id":"%s","cwd":"%s"}}\n{"payload":{"role":"assistant","content":[{"type":"output_text","text":"%s"}]}}\n' "$id" "$p" "$text" > "$f"
}

CLAUDE_PATHS=()

claude_proj "$H/Code/orbit-dashboard" -2M \
  "Deploy is green — the widget cache invalidates on rotation now and all 34 tests pass. Next: wire the p95 latency panel to the new collector."
repo "$H/Code/orbit-dashboard" main 2

claude_proj "$H/Code/llm-eval-harness" -3H \
  "Scoring harness refactor done; judge prompts moved to templates."
codex_proj "$H/Code/llm-eval-harness" -40M \
  "Pairwise judge with position-swap is in. Agreement vs human labels: 0.83 on the dev split — good enough to gate merges."
repo "$H/Code/llm-eval-harness" feat/scoring 0

claude_proj "$H/Code/字幕翻译工具" -5H \
  "批量模式搞定了：300 条字幕 4.2 秒，缓存命中率 91%。下一步加术语表支持。"
repo "$H/Code/字幕翻译工具" main 1

codex_proj "$H/Code/recipe-api" -26H \
  "Cursor pagination now survives deletions mid-page; added 12 regression tests."
repo "$H/Code/recipe-api" fix/pagination 1

claude_proj "$H/Notes/research-notes" -49H \
  "Summarized the three retrieval papers — the reranker ablation is the one worth replicating."

claude_proj "$H/Code/dotfiles" -100H "Migrated the tmux config to the new syntax; everything sources cleanly."
repo "$H/Code/dotfiles" main 0

codex_proj "$H/Code/pixel-garden" -6d \
  "Seeded biome generation is deterministic now — same seed, same garden."
repo "$H/Code/pixel-garden" main 0

claude_proj "$H/Code/blog" -14d "Draft reads well; trimmed the intro and added the latency chart."
repo "$H/Code/blog" draft/agents-post 0

codex_proj "$H/Code/agent-playground" -8H \
  "Tool-call tracing works; every run writes a replayable trace file."
repo "$H/Code/agent-playground" main 3

codex_proj "$H/Code/market-scraper" -3d \
  "Switched to the bulk endpoint — full refresh went from 40 min to 6."
repo "$H/Code/market-scraper" main 0

claude_proj "$H/Docs/thesis-draft" -5d \
  "Chapter 3 restructured around the ablation results; figures regenerated."

claude_proj "$H/Code/home-server" -21d "Backups verified; the new compose file brings everything up in one shot."
repo "$H/Code/home-server" main 0

# ~/.claude.json lists every claude project
{
  printf '{"projects":{'
  sep=""
  for p in "${CLAUDE_PATHS[@]}"; do printf '%s"%s":{}' "$sep" "$p"; sep=","; done
  printf '}}'
} > "$H/.claude.json"

# perch config: one pinned project, first-run hint already dismissed
printf '{"pinned":{"%s":true},"seen_settings_hint":true}\n' "$H/Code/orbit-dashboard" > "$DEMO/config/state.json"

echo "demo home ready: $DEMO"
