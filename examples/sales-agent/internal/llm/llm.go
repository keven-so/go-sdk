// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package llm defines a minimal, provider-agnostic chat+tools interface for the
// agent loop. It is deliberately shaped to map cleanly onto the Anthropic
// Messages API (see anthropic.go) while remaining trivially fakeable for tests
// and dry runs (see fake.go).
package llm

import (
	"context"
	"encoding/json"
)

// Block is a single content block in a message or response. Type is one of
// "text", "tool_use", or "tool_result".
type Block struct {
	Type string `json:"type"`

	// text
	Text string `json:"text,omitempty"`

	// tool_use (assistant asks to call a tool)
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result (we return a tool's output to the model)
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// Message is one turn in the conversation. Role is "user" or "assistant".
type Message struct {
	Role    string  `json:"role"`
	Content []Block `json:"content"`
}

// ToolDef describes a tool available to the model. InputSchema is a JSON Schema
// object (typically marshaled straight from an mcp.Tool's InputSchema).
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// Request is a single generation request.
type Request struct {
	Model     string
	System    string
	MaxTokens int
	Tools     []ToolDef
	Messages  []Message
}

// Response is the model's reply for one turn.
type Response struct {
	Content    []Block
	StopReason string // "end_turn" | "tool_use" | ...
}

// LLM is the brain behind the agent loop.
type LLM interface {
	Generate(ctx context.Context, req Request) (*Response, error)
}

// Text concatenates all text blocks in the response.
func (r *Response) Text() string {
	var s string
	for _, b := range r.Content {
		if b.Type == "text" {
			if s != "" {
				s += "\n"
			}
			s += b.Text
		}
	}
	return s
}

// ToolUses returns the tool_use blocks in the response.
func (r *Response) ToolUses() []Block {
	var out []Block
	for _, b := range r.Content {
		if b.Type == "tool_use" {
			out = append(out, b)
		}
	}
	return out
}
