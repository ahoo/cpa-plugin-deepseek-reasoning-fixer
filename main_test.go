package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFixReasoningContent(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		changed  bool
		expected int
	}{
		{
			name:     "no reasoning_effort with existing valid rc: untouched",
			body:     `{"model":"deepseek-v4-flash-free","messages":[{"role":"assistant","content":"hi","reasoning_content":"valid"}]}`,
			changed:  false,
			expected: 1,
		},
		{
			name: "reasoning_effort none: untouched",
			body: `{"model":"deepseek-v4-flash-free","reasoning_effort":"none","messages":[{"role":"assistant","content":"hi"}]}`,
		},
		{
			name:     "effort max fills missing assistant messages",
			body:     `{"model":"deepseek-v4-flash-free","reasoning_effort":"max","messages":[{"role":"user","content":"q"},{"role":"assistant","content":"a"},{"role":"assistant","tool_calls":[{"id":"t1"}],"content":null}]}`,
			changed:  true,
			expected: 2,
		},
		{
			name:     "existing reasoning_content kept",
			body:     `{"model":"deepseek-v4-flash-free","reasoning_effort":"max","messages":[{"role":"assistant","content":"a","reasoning_content":"real reasoning"}]}`,
			changed:  false,
			expected: 1,
		},
		{
			name:     "empty reasoning_content string filled",
			body:     `{"model":"deepseek-v4-flash-free","reasoning_effort":"high","messages":[{"role":"assistant","content":"a","reasoning_content":""}]}`,
			changed:  true,
			expected: 1,
		},
		{
			name:     "json null reasoning_content filled",
			body:     `{"model":"deepseek-v4-flash-free","reasoning_effort":"max","messages":[{"role":"assistant","content":"a","reasoning_content":null}]}`,
			changed:  true,
			expected: 1,
		},
		{
			name:     "empty reasoning array filled",
			body:     `{"model":"deepseek-v4-flash-free","reasoning_effort":"max","messages":[{"role":"assistant","content":"a","reasoning_content":[]}]}`,
			changed:  true,
			expected: 1,
		},
		{
			name:     "assistant missing reasoning_content and content only tool_calls filled",
			body:     `{"model":"deepseek-v4-flash-free","reasoning_effort":"max","messages":[{"role":"assistant","tool_calls":[{"id":"t1"}],"content":null}]}`,
			changed:  true,
			expected: 1,
		},
		{
			name:    "unknown reasoning shape kept",
			body:    `{"model":"deepseek-v4-flash-free","reasoning_effort":"max","messages":[{"role":"assistant","content":"a","reasoning_content":{"detail":1}}]}`,
			changed: false,
		},
		{
			name:     "no reasoning_effort field still fixed (claude-translated path)",
			body:     `{"model":"deepseek-v4-flash-free","messages":[{"role":"assistant","content":"a"},{"role":"assistant","tool_calls":[{"id":"t1"}]}]}`,
			changed:  true,
			expected: 2,
		},
		{
			name:     "reasoning level none skipped",
			body:     `{"model":"deepseek-v4-flash-free","reasoning":{"level":"none"},"messages":[{"role":"assistant","content":"a"}]}`,
			changed:  false,
		},
				{
			name:    "user messages untouched",
			body:    `{"model":"deepseek-v4-flash-free","reasoning_effort":"max","messages":[{"role":"user","content":"q"}]}`,
			changed: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, changed, err := fixReasoningContent([]byte(c.body))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if changed != c.changed {
				t.Fatalf("changed = %v, want %v", changed, c.changed)
			}
			if !c.changed {
				return
			}
			var payload map[string]any
			if err := json.Unmarshal(out, &payload); err != nil {
				t.Fatalf("invalid output json: %v", err)
			}
			msgs := payload["messages"].([]any)
			filled := 0
			for _, m := range msgs {
				msg := m.(map[string]any)
				if msg["role"] == "assistant" {
					rc, ok := msg["reasoning_content"].(string)
					if !ok || rc == "" {
						t.Fatalf("assistant message missing reasoning_content: %v", msg)
					}
					filled++
				}
			}
			if filled != c.expected {
				t.Fatalf("filled = %d, want %d", filled, c.expected)
			}
		})
	}
}

func TestNormalizeRequestModelFilter(t *testing.T) {
	nonDeepseek := `{"model":"gpt-5.6","reasoning_effort":"max","messages":[{"role":"assistant","content":"a"}]}`
	req, _ := json.Marshal(requestTransformRequest{
		FromFormat: "claude",
		ToFormat:   "openai",
		Model:      "gpt-5.6",
		Body:       []byte(nonDeepseek),
	})
	raw, err := normalizeRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("invalid envelope: %v", err)
	}
	var resp payloadResponse
	if err := json.Unmarshal(env.Result, &resp); err != nil {
		t.Fatalf("invalid result: %v", err)
	}
	if string(resp.Body) != nonDeepseek {
		t.Fatalf("non-deepseek body was modified")
	}

	deepseek := `{"model":"deepseek-v4-flash-free","reasoning_effort":"max","messages":[{"role":"assistant","content":"a"}]}`
	req2, _ := json.Marshal(requestTransformRequest{
		FromFormat: "claude",
		ToFormat:   "openai",
		Model:      "deepseek-v4-flash-free",
		Body:       []byte(deepseek),
	})
	raw2, err := normalizeRequest(req2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env2 envelope
	if err := json.Unmarshal(raw2, &env2); err != nil {
		t.Fatalf("invalid envelope: %v", err)
	}
	var resp2 payloadResponse
	if err := json.Unmarshal(env2.Result, &resp2); err != nil {
		t.Fatalf("invalid result: %v", err)
	}
	if !strings.Contains(string(resp2.Body), "reasoning_content") {
		t.Fatalf("deepseek body was not fixed")
	}
}