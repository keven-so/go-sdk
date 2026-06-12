// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/llm"
)

// Loop is the Claude agent loop: it drives an LLM under a Role's system prompt,
// executes any tools the model requests via the ToolBridge, and feeds results
// back until the model stops requesting tools (or MaxTurns is hit).
type Loop struct {
	LLM       llm.LLM
	Bridge    *ToolBridge
	Model     string
	MaxTokens int
	MaxTurns  int
	// Observer, if set, is notified of each step for logging/persistence.
	Observer func(Event)
}

// Event is an observable step in a loop run.
type Event struct {
	Kind    string // assistant_text | tool_call | tool_result | done
	Role    string
	Text    string
	Tool    string
	Input   json.RawMessage
	Output  string
	IsError bool
}

const defaultMaxTurns = 16

// Run executes the loop for one role starting from the given conversation
// history, and returns the updated history (including the assistant turns and
// tool results produced during the run).
func (l *Loop) Run(ctx context.Context, role Role, history []llm.Message) ([]llm.Message, error) {
	msgs := append([]llm.Message(nil), history...)
	maxTurns := l.MaxTurns
	if maxTurns <= 0 {
		maxTurns = defaultMaxTurns
	}

	for turn := 0; turn < maxTurns; turn++ {
		resp, err := l.LLM.Generate(ctx, llm.Request{
			Model:     l.Model,
			System:    role.SystemPrompt,
			MaxTokens: l.MaxTokens,
			Tools:     l.Bridge.FilteredDefs(role.AllowedTools),
			Messages:  msgs,
		})
		if err != nil {
			return msgs, err
		}

		msgs = append(msgs, llm.Message{Role: "assistant", Content: resp.Content})
		for _, b := range resp.Content {
			if b.Type == "text" && b.Text != "" {
				l.emit(Event{Kind: "assistant_text", Role: role.Name, Text: b.Text})
			}
		}

		toolUses := resp.ToolUses()
		if len(toolUses) == 0 {
			l.emit(Event{Kind: "done", Role: role.Name, Text: resp.Text()})
			return msgs, nil
		}

		results := make([]llm.Block, 0, len(toolUses))
		for _, tu := range toolUses {
			l.emit(Event{Kind: "tool_call", Role: role.Name, Tool: tu.Name, Input: tu.Input})
			out, isErr := l.Bridge.Invoke(ctx, tu.Name, tu.Input)
			l.emit(Event{Kind: "tool_result", Role: role.Name, Tool: tu.Name, Output: out, IsError: isErr})
			results = append(results, llm.Block{
				Type:      "tool_result",
				ToolUseID: tu.ID,
				Content:   out,
				IsError:   isErr,
			})
		}
		msgs = append(msgs, llm.Message{Role: "user", Content: results})
	}
	return msgs, nil
}

func (l *Loop) emit(e Event) {
	if l.Observer != nil {
		l.Observer(e)
	}
}
