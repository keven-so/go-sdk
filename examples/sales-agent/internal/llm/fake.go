// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llm

import (
	"context"
	"encoding/json"
)

// Fake is a scripted LLM that replays a fixed sequence of responses, ignoring
// the request. It lets the full agent loop + MCP tool bridge run end-to-end with
// no API key and no network — the backbone of the dry-run mode and loop tests.
type Fake struct {
	turns []*Response
	i     int
}

// NewFake builds a Fake that returns the given responses in order. Once the
// script is exhausted it returns a terminal end_turn response.
func NewFake(turns ...*Response) *Fake {
	return &Fake{turns: turns}
}

// Generate returns the next scripted response.
func (f *Fake) Generate(_ context.Context, _ Request) (*Response, error) {
	if f.i >= len(f.turns) {
		return AssistantText("(end of scripted conversation)"), nil
	}
	r := f.turns[f.i]
	f.i++
	return r, nil
}

// AssistantToolUse builds a scripted response whose assistant turn calls one
// tool. input is marshaled to JSON as the tool arguments.
func AssistantToolUse(id, name string, input any) *Response {
	raw, _ := json.Marshal(input)
	return &Response{
		StopReason: "tool_use",
		Content:    []Block{{Type: "tool_use", ID: id, Name: name, Input: raw}},
	}
}

// AssistantText builds a scripted terminal response with plain text.
func AssistantText(text string) *Response {
	return &Response{
		StopReason: "end_turn",
		Content:    []Block{{Type: "text", Text: text}},
	}
}
