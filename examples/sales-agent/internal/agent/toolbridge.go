// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/llm"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolBridge is the load-bearing seam between the LLM and MCP. It enumerates
// tools from one or more connected MCP servers, exposes them to the model as
// llm.ToolDefs, and routes the model's tool_use calls to the owning
// mcp.ClientSession.
type ToolBridge struct {
	routing map[string]*mcp.ClientSession
	defs    []llm.ToolDef
}

// NewToolBridge discovers tools across the given sessions (keyed by server name
// for error context) and builds the routing table and tool definitions.
func NewToolBridge(ctx context.Context, sessions map[string]*mcp.ClientSession) (*ToolBridge, error) {
	b := &ToolBridge{routing: map[string]*mcp.ClientSession{}}
	for name, sess := range sessions {
		res, err := sess.ListTools(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("list tools on %q: %w", name, err)
		}
		for _, t := range res.Tools {
			schema, err := json.Marshal(t.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("marshal schema for %q: %w", t.Name, err)
			}
			if _, dup := b.routing[t.Name]; dup {
				return nil, fmt.Errorf("duplicate tool name %q across servers", t.Name)
			}
			b.routing[t.Name] = sess
			b.defs = append(b.defs, llm.ToolDef{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: schema,
			})
		}
	}
	return b, nil
}

// Defs returns all discovered tool definitions.
func (b *ToolBridge) Defs() []llm.ToolDef { return b.defs }

// FilteredDefs returns only the tools named in allowed. An empty allowed list
// returns all tools.
func (b *ToolBridge) FilteredDefs(allowed []string) []llm.ToolDef {
	if len(allowed) == 0 {
		return b.defs
	}
	set := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		set[a] = true
	}
	var out []llm.ToolDef
	for _, d := range b.defs {
		if set[d.Name] {
			out = append(out, d)
		}
	}
	return out
}

// Invoke calls a tool by name with raw JSON arguments and returns the flattened
// text result plus an isError flag, mirroring the MCP tool-result semantics so
// the model can see failures and self-correct.
func (b *ToolBridge) Invoke(ctx context.Context, name string, input json.RawMessage) (string, bool) {
	sess, ok := b.routing[name]
	if !ok {
		return fmt.Sprintf("unknown tool %q", name), true
	}
	var args map[string]any
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return fmt.Sprintf("invalid arguments for %q: %v", name, err), true
		}
	}
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return err.Error(), true
	}
	return flattenContent(res.Content), res.IsError
}

func flattenContent(content []mcp.Content) string {
	var sb strings.Builder
	for _, c := range content {
		if tc, ok := c.(*mcp.TextContent); ok {
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}
