package preview

import (
	"testing"
)

func TestExtractClaudeFormats(t *testing.T) {
	assistant := []byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"Done, tests pass."}]}}`)
	role, text := extract(assistant)
	if role != "assistant" || text != "Done, tests pass." {
		t.Errorf("got (%q, %q)", role, text)
	}

	user := []byte(`{"type":"user","message":{"role":"user","content":"fix the bug"}}`)
	role, text = extract(user)
	if role != "user" || text != "fix the bug" {
		t.Errorf("got (%q, %q)", role, text)
	}

	toolResult := []byte(`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"stuff"}]}}`)
	if _, text = extract(toolResult); text != "" {
		t.Errorf("tool_result should not produce text, got %q", text)
	}

	reminder := []byte(`{"type":"user","message":{"role":"user","content":"<system-reminder>x</system-reminder>"}}`)
	if _, text = extract(reminder); text != "" {
		t.Errorf("system-reminder should be filtered, got %q", text)
	}
}

func TestExtractCodexFormats(t *testing.T) {
	line := []byte(`{"timestamp":"t","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Refactor complete."}]}}`)
	role, text := extract(line)
	if role != "assistant" || text != "Refactor complete." {
		t.Errorf("got (%q, %q)", role, text)
	}

	meta := []byte(`{"type":"session_meta","payload":{"id":"x","cwd":"/y"}}`)
	if _, text = extract(meta); text != "" {
		t.Errorf("session_meta should not produce text, got %q", text)
	}
}
