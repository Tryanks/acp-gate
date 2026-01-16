package acpinspect

import (
	"encoding/json"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

// AnyMessage matches the ACP SDK's internal JSON-RPC envelope.
// We re-declare it here so the proxy can remain fully transparent,
// including for unknown/experimental methods.
//
// Note: we intentionally keep Params/Result/Raw as json.RawMessage.
type AnyMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   json.RawMessage  `json:"error,omitempty"`
}

// Extract returns (sessionID, method, userText, agentText).
// It best-effort parses known ACP methods and leaves everything else empty.
func Extract(msg AnyMessage) (string, string, string, string) {
	method := msg.Method

	// Requests/notifications carry method+params.
	if method != "" {
		sid, userText, agentText := extractFromParams(method, msg.Params)
		return sid, method, userText, agentText
	}

	// Responses don't carry method. We still persist raw, but extraction is empty.
	return "", "", "", ""
}

func extractFromParams(method string, params json.RawMessage) (string, string, string) {
	switch method {
	case acp.AgentMethodSessionPrompt:
		var p acp.PromptRequest
		if json.Unmarshal(params, &p) != nil {
			return "", "", ""
		}
		return string(p.SessionId), joinTextFromContentBlocks(p.Prompt), ""

	case acp.ClientMethodSessionUpdate:
		var n acp.SessionNotification
		if json.Unmarshal(params, &n) != nil {
			return "", "", ""
		}
		switch {
		case n.Update.AgentMessageChunk != nil:
			return string(n.SessionId), "", textFromContentBlock(n.Update.AgentMessageChunk.Content)
		case n.Update.UserMessageChunk != nil:
			return string(n.SessionId), textFromContentBlock(n.Update.UserMessageChunk.Content), ""
		case n.Update.AgentThoughtChunk != nil:
			return string(n.SessionId), "", textFromContentBlock(n.Update.AgentThoughtChunk.Content)
		case n.Update.ToolCall != nil:
			// Best-effort: capture any text content produced by tools.
			return string(n.SessionId), "", joinTextFromToolCallContent(n.Update.ToolCall.Content)
		case n.Update.ToolCallUpdate != nil:
			return string(n.SessionId), "", joinTextFromToolCallContent(n.Update.ToolCallUpdate.Content)
		default:
			return string(n.SessionId), "", ""
		}

	default:
		// Many ACP requests include sessionId; try a generic parse.
		var any struct {
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(params, &any) == nil {
			return any.SessionID, "", ""
		}
		return "", "", ""
	}
}

func joinTextFromContentBlocks(blocks []acp.ContentBlock) string {
	var parts []string
	for _, b := range blocks {
		t := textFromContentBlock(b)
		if t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n")
}

func joinTextFromToolCallContent(items []acp.ToolCallContent) string {
	var parts []string
	for _, it := range items {
		if it.Content != nil {
			t := textFromContentBlock(it.Content.Content)
			if t != "" {
				parts = append(parts, t)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func textFromContentBlock(block acp.ContentBlock) string {
	if block.Text == nil {
		return ""
	}
	return block.Text.Text
}
