package acpinspect

import (
	"encoding/json"
	"testing"
)

func TestExtract_SessionPrompt(t *testing.T) {
	raw := []byte(`{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":{"sessionId":"sess_x","prompt":[{"type":"text","text":"Hello, agent!"}]}}`)
	var msg AnyMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	sid, method, userText, agentText := Extract(msg)
	if sid != "sess_x" || method != "session/prompt" {
		t.Fatalf("unexpected sid/method: %q %q", sid, method)
	}
	if userText != "Hello, agent!" {
		t.Fatalf("unexpected userText: %q", userText)
	}
	if agentText != "" {
		t.Fatalf("unexpected agentText: %q", agentText)
	}
}

func TestExtract_SessionUpdateAgentMessageChunk(t *testing.T) {
	raw := []byte(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"sess_x","update":{"content":{"text":"hi","type":"text"},"sessionUpdate":"agent_message_chunk"}}}`)
	var msg AnyMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	sid, method, userText, agentText := Extract(msg)
	if sid != "sess_x" || method != "session/update" {
		t.Fatalf("unexpected sid/method: %q %q", sid, method)
	}
	if userText != "" {
		t.Fatalf("unexpected userText: %q", userText)
	}
	if agentText != "hi" {
		t.Fatalf("unexpected agentText: %q", agentText)
	}
}
