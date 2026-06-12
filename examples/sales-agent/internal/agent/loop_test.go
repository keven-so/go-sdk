// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent_test

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/agent"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/app"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/crm"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/llm"
)

// TestLoopDrivesMCPTools verifies the load-bearing seam: a scripted model
// tool_use is routed through the ToolBridge to a real in-process MCP server, the
// result round-trips back, and the side effect (a logged activity) lands in the
// store.
func TestLoopDrivesMCPTools(t *testing.T) {
	ctx := context.Background()
	store := crm.NewMemoryStore()
	lead, _ := store.CreateLead(&crm.Lead{Company: "Acme", Domain: "acme.com", Source: "marketing"})
	conv, _ := store.CreateConversation(&crm.Conversation{LeadID: lead.ID})

	tools, err := app.BuildTools(ctx, store, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tools.Close()

	fake := llm.NewFake(
		llm.AssistantToolUse("a", "get_lead", map[string]any{"lead_id": lead.ID}),
		llm.AssistantToolUse("b", "log_activity", map[string]any{
			"conversation_id": conv.ID, "type": "note", "summary": "qualified",
		}),
		llm.AssistantText("done"),
	)

	var calls []string
	loop := &agent.Loop{
		LLM:    fake,
		Bridge: tools.Bridge,
		Observer: func(e agent.Event) {
			if e.Kind == "tool_call" {
				calls = append(calls, e.Tool)
			}
			if e.Kind == "tool_result" && e.IsError {
				t.Errorf("tool %s returned error: %s", e.Tool, e.Output)
			}
		},
	}

	if _, err := loop.Run(ctx, agent.SDR, nil); err != nil {
		t.Fatal(err)
	}

	if len(calls) != 2 || calls[0] != "get_lead" || calls[1] != "log_activity" {
		t.Fatalf("unexpected tool calls: %v", calls)
	}
	acts, _ := store.ListActivities(conv.ID)
	if len(acts) != 1 || acts[0].Summary != "qualified" {
		t.Fatalf("activity not persisted via MCP tool: %+v", acts)
	}
}

// TestFilteredDefsRespectsRole ensures a role only sees its allowed tools.
func TestFilteredDefsRespectsRole(t *testing.T) {
	ctx := context.Background()
	store := crm.NewMemoryStore()
	tools, err := app.BuildTools(ctx, store, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tools.Close()

	role := agent.Role{Name: "x", AllowedTools: []string{"get_lead"}}
	defs := tools.Bridge.FilteredDefs(role.AllowedTools)
	if len(defs) != 1 || defs[0].Name != "get_lead" {
		t.Fatalf("expected only get_lead, got %+v", defs)
	}
	// Unknown tool invocation surfaces as an error result, not a panic.
	if out, isErr := tools.Bridge.Invoke(ctx, "does_not_exist", nil); !isErr || out == "" {
		t.Fatalf("expected error for unknown tool, got %q isErr=%v", out, isErr)
	}
}
