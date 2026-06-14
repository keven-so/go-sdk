// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llm

import "context"

// Static is a deterministic LLM that always returns the same text with no tool
// calls. It backs support sub-agents in dry-run mode so the agent-as-tool wiring
// runs end-to-end without any network calls.
type Static struct {
	reply func(Request) string
}

// NewStatic returns a Static that always replies with text.
func NewStatic(text string) *Static {
	return &Static{reply: func(Request) string { return text }}
}

// NewStaticFunc returns a Static whose reply is derived from the request.
func NewStaticFunc(f func(Request) string) *Static {
	return &Static{reply: f}
}

// Generate implements LLM.
func (s *Static) Generate(_ context.Context, req Request) (*Response, error) {
	return AssistantText(s.reply(req)), nil
}
