// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/llm"
)

// SubAgent packages a support Role as a callable unit (the "agent-as-tool"
// pattern): it runs a constrained agent loop and returns the final text. It
// reuses the same Loop machinery the customer-facing agents run on.
type SubAgent struct {
	Role     Role
	LLM      llm.LLM
	Bridge   *ToolBridge
	MaxTurns int
}

// Run executes the sub-agent on the given input and returns its final answer.
func (s SubAgent) Run(ctx context.Context, input string) (string, error) {
	maxTurns := s.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 6
	}
	loop := &Loop{LLM: s.LLM, Bridge: s.Bridge, MaxTurns: maxTurns}
	history := []llm.Message{{
		Role:    "user",
		Content: []llm.Block{{Type: "text", Text: input}},
	}}
	msgs, err := loop.Run(ctx, s.Role, history)
	if err != nil {
		return "", err
	}
	return LastAssistantText(msgs), nil
}

// LastAssistantText returns the concatenated text of the final assistant turn.
func LastAssistantText(msgs []llm.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "assistant" {
			continue
		}
		var s string
		for _, b := range msgs[i].Content {
			if b.Type == "text" {
				if s != "" {
					s += "\n"
				}
				s += b.Text
			}
		}
		if s != "" {
			return s
		}
	}
	return ""
}
