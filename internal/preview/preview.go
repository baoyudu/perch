// Package preview extracts a "where was I?" snippet from the most recent
// Claude Code or Codex session transcript of a project.
package preview

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/baoyudu/psw/internal/index"
)

type Snippet struct {
	Agent string // which agent's transcript the text came from
	Role  string // "assistant" or "user"
	Text  string
}

const tailBytes = 256 * 1024

// Load returns the last meaningful message of the project's newest session,
// preferring the assistant's final text. Empty Snippet means nothing usable.
func Load(p index.Project) Snippet {
	file, agent := p.ClaudeSessionFile, index.AgentClaude
	if p.CodexLast.After(p.ClaudeLast) && p.CodexSessionFile != "" {
		file, agent = p.CodexSessionFile, index.AgentCodex
	}
	if file == "" {
		return Snippet{}
	}
	lines := readTailLines(file)
	var userFallback string
	for i := len(lines) - 1; i >= 0; i-- {
		role, text := extract(lines[i])
		if text == "" {
			continue
		}
		if role == "assistant" {
			return Snippet{Agent: agent, Role: role, Text: clean(text)}
		}
		if role == "user" && userFallback == "" {
			userFallback = text
		}
	}
	if userFallback != "" {
		return Snippet{Agent: agent, Role: "user", Text: clean(userFallback)}
	}
	return Snippet{}
}

func readTailLines(path string) [][]byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil
	}
	offset := info.Size() - tailBytes
	if offset < 0 {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil
	}
	lines := splitLines(data)
	if offset > 0 && len(lines) > 0 {
		lines = lines[1:] // first line is truncated mid-record
	}
	return lines
}

func splitLines(data []byte) [][]byte {
	var out [][]byte
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, []byte(l))
		}
	}
	return out
}

// extract pulls (role, text) from one transcript line of either agent.
func extract(line []byte) (string, string) {
	var m map[string]any
	if json.Unmarshal(line, &m) != nil {
		return "", ""
	}
	// Codex wraps records in payload; Claude puts the API message in message.
	if payload, ok := m["payload"].(map[string]any); ok {
		return fromMessage(payload)
	}
	if msg, ok := m["message"].(map[string]any); ok {
		return fromMessage(msg)
	}
	return "", ""
}

func fromMessage(m map[string]any) (string, string) {
	role, _ := m["role"].(string)
	if role != "assistant" && role != "user" {
		return "", ""
	}
	switch content := m["content"].(type) {
	case string:
		return role, usable(content)
	case []any:
		// Last text-ish block wins: for interleaved thinking/tool_use blocks
		// the final text is the actual reply.
		var text string
		for _, block := range content {
			b, ok := block.(map[string]any)
			if !ok {
				continue
			}
			switch b["type"] {
			case "text", "output_text", "input_text":
				if t, _ := b["text"].(string); usable(t) != "" {
					text = t
				}
			}
		}
		return role, usable(text)
	}
	return "", ""
}

// usable filters out synthetic/system content that would make a useless preview.
func usable(text string) string {
	t := strings.TrimSpace(text)
	if t == "" || strings.HasPrefix(t, "<") || strings.HasPrefix(t, "Caveat:") {
		return ""
	}
	return t
}

func clean(text string) string {
	text = strings.TrimSpace(text)
	if n := utf8.RuneCountInString(text); n > 700 {
		runes := []rune(text)
		text = string(runes[:700]) + "…"
	}
	return text
}
