// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/agent"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/app"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/crm"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/llm"
)

// TestSubAgentDrivesBaseTool proves the agent-as-tool core: a sub-agent loop
// drives a base MCP tool and its side effect lands in the store.
func TestSubAgentDrivesBaseTool(t *testing.T) {
	ctx := context.Background()
	store := crm.NewMemoryStore()
	lead, _ := store.CreateLead(&crm.Lead{Company: "Acme"})
	conv, _ := store.CreateConversation(&crm.Conversation{LeadID: lead.ID})

	tools, err := app.BuildTools(ctx, store, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tools.Close()

	role := agent.Role{Name: "tester", Kind: agent.KindSupport, SystemPrompt: "x", AllowedTools: []string{"log_activity"}}
	fake := llm.NewFake(
		llm.AssistantToolUse("x", "log_activity", map[string]any{
			"conversation_id": conv.ID, "type": "note", "summary": "from sub-agent",
		}),
		llm.AssistantText("sub-agent done"),
	)
	sa := agent.SubAgent{Role: role, LLM: fake, Bridge: tools.Bridge, MaxTurns: 4}
	out, err := sa.Run(ctx, "do the thing")
	if err != nil {
		t.Fatal(err)
	}
	if out != "sub-agent done" {
		t.Fatalf("unexpected sub-agent output: %q", out)
	}
	acts, _ := store.ListActivities(conv.ID)
	if len(acts) != 1 || acts[0].Summary != "from sub-agent" {
		t.Fatalf("sub-agent did not drive base tool: %+v", acts)
	}
}

// TestTeamToolsViaBridge exercises the team MCP server (agent-as-tool) end-to-end
// through the real ToolBridge in dry-run mode.
func TestTeamToolsViaBridge(t *testing.T) {
	ctx := context.Background()
	store := crm.NewMemoryStore()
	lead, _ := store.CreateLead(&crm.Lead{Company: "Acme", Domain: "acme.com", Source: "marketing"})
	conv, _ := store.CreateConversation(&crm.Conversation{LeadID: lead.ID})

	tools, err := app.BuildTools(ctx, store, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tools.Close()

	check := func(tool string, args map[string]any) {
		raw, _ := json.Marshal(args)
		out, isErr := tools.Bridge.Invoke(ctx, tool, raw)
		if isErr || strings.TrimSpace(out) == "" {
			t.Fatalf("%s returned isErr=%v out=%q", tool, isErr, out)
		}
	}
	check("research_account", map[string]any{"lead_id": lead.ID})
	check("prioritize_lead", map[string]any{"lead_id": lead.ID})
	check("draft_message", map[string]any{"conversation_id": conv.ID, "lead_id": lead.ID, "channel": "email", "intent": "book a meeting"})
	check("coach_review", map[string]any{"conversation_id": conv.ID, "lead_id": lead.ID})

	// orchestrator: handoff updates the conversation role and logs an activity.
	check("handoff", map[string]any{"conversation_id": conv.ID, "role": "closer", "reason": "qualified"})
	got, _ := store.GetConversation(conv.ID)
	if got.CurrentRole != "closer" {
		t.Fatalf("handoff did not set current_role: %q", got.CurrentRole)
	}
}
