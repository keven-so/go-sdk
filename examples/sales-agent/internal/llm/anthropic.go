// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Anthropic is the production LLM backed by the Claude Messages API. It is a
// thin translation layer between this package's provider-agnostic types and the
// Anthropic Go SDK; the agent loop never imports the Anthropic SDK directly.
type Anthropic struct {
	client    anthropic.Client
	model     anthropic.Model
	maxTokens int64
}

// NewAnthropic constructs an Anthropic LLM. An empty model defaults to a current
// Claude model; an empty maxTokens defaults to 1024.
func NewAnthropic(apiKey, model string, maxTokens int) *Anthropic {
	m := anthropic.Model(model)
	if model == "" {
		m = anthropic.ModelClaudeSonnet4_6
	}
	mt := int64(maxTokens)
	if mt == 0 {
		mt = 1024
	}
	return &Anthropic{
		client:    anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:     m,
		maxTokens: mt,
	}
}

// Generate implements LLM by calling the Anthropic Messages API.
func (a *Anthropic) Generate(ctx context.Context, req Request) (*Response, error) {
	params := anthropic.MessageNewParams{
		Model:     a.model,
		MaxTokens: a.maxTokens,
	}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}
	for _, t := range req.Tools {
		schema, err := toInputSchema(t.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("tool %q schema: %w", t.Name, err)
		}
		tp := anthropic.ToolParam{Name: t.Name, InputSchema: schema}
		if t.Description != "" {
			tp.Description = anthropic.String(t.Description)
		}
		params.Tools = append(params.Tools, anthropic.ToolUnionParam{OfTool: &tp})
	}
	for _, m := range req.Messages {
		params.Messages = append(params.Messages, toAnthropicMessage(m))
	}

	msg, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return nil, err
	}

	out := &Response{StopReason: string(msg.StopReason)}
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			out.Content = append(out.Content, Block{Type: "text", Text: block.Text})
		case "tool_use":
			out.Content = append(out.Content, Block{
				Type:  "tool_use",
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			})
		}
	}
	return out, nil
}

// toInputSchema converts a JSON Schema object into the Anthropic tool schema
// param, preserving the properties/required fields the model needs.
func toInputSchema(raw json.RawMessage) (anthropic.ToolInputSchemaParam, error) {
	var parsed struct {
		Properties any      `json:"properties"`
		Required   []string `json:"required"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return anthropic.ToolInputSchemaParam{}, err
		}
	}
	return anthropic.ToolInputSchemaParam{
		Properties: parsed.Properties,
		Required:   parsed.Required,
	}, nil
}

func toAnthropicMessage(m Message) anthropic.MessageParam {
	var blocks []anthropic.ContentBlockParamUnion
	for _, b := range m.Content {
		switch b.Type {
		case "text":
			blocks = append(blocks, anthropic.NewTextBlock(b.Text))
		case "tool_use":
			blocks = append(blocks, anthropic.NewToolUseBlock(b.ID, json.RawMessage(b.Input), b.Name))
		case "tool_result":
			blocks = append(blocks, anthropic.NewToolResultBlock(b.ToolUseID, b.Content, b.IsError))
		}
	}
	if m.Role == "assistant" {
		return anthropic.NewAssistantMessage(blocks...)
	}
	return anthropic.NewUserMessage(blocks...)
}
