// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package orchestrator exposes coordination tools: routing an event to a role,
// handing a conversation between customer-facing agents, and persisting
// qualification (BANT/MEDDICC). All transitions are recorded as activities so
// they are observable and auditable.
package orchestrator

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/agent"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/crm"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// New returns the orchestrator MCP server backed by the given store.
func New(store crm.Store) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "orchestrator", Version: "0.1.0"}, nil)
	h := &handlers{store: store}
	mcp.AddTool(s, &mcp.Tool{Name: "route_to_role", Description: "Assign a conversation to a customer-facing role (supervisor use)."}, h.routeToRole)
	mcp.AddTool(s, &mcp.Tool{Name: "handoff", Description: "Hand a conversation off to another customer-facing role with a reason."}, h.handoff)
	mcp.AddTool(s, &mcp.Tool{Name: "update_qualification", Description: "Persist qualification capture for a framework (bant or meddicc)."}, h.updateQualification)
	return s
}

type handlers struct {
	store crm.Store
}

type RouteIn struct {
	ConversationID string `json:"conversation_id" jsonschema:"the conversation id"`
	Role           string `json:"role" jsonschema:"target customer-facing role: sdr | inbound | voice | closer"`
	Reason         string `json:"reason" jsonschema:"why this role was chosen"`
}

type RouteOut struct {
	ConversationID string `json:"conversation_id"`
	Role           string `json:"role"`
}

func (h *handlers) routeToRole(_ context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[RouteIn]) (*mcp.CallToolResultFor[RouteOut], error) {
	return h.assign(p.Arguments.ConversationID, p.Arguments.Role, p.Arguments.Reason, "route")
}

func (h *handlers) handoff(_ context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[RouteIn]) (*mcp.CallToolResultFor[RouteOut], error) {
	return h.assign(p.Arguments.ConversationID, p.Arguments.Role, p.Arguments.Reason, "handoff")
}

func (h *handlers) assign(convID, role, reason, kind string) (*mcp.CallToolResultFor[RouteOut], error) {
	if _, ok := agent.Get(role); !ok {
		return errResult[RouteOut](fmt.Sprintf("unknown role %q", role)), nil
	}
	conv, err := h.store.UpdateConversation(convID, map[string]any{"current_role": role})
	if err != nil {
		return errResult[RouteOut](err.Error()), nil
	}
	_, _ = h.store.AddActivity(&crm.Activity{
		ConversationID: convID,
		Type:           kind,
		Summary:        fmt.Sprintf("%s → %s: %s", kind, role, reason),
	})
	return okResult(fmt.Sprintf("%s assigned to %s (%s)", convID, role, reason),
		RouteOut{ConversationID: conv.ID, Role: role}), nil
}

type QualifyIn struct {
	ConversationID string         `json:"conversation_id" jsonschema:"the conversation id"`
	Framework      string         `json:"framework" jsonschema:"bant | meddicc"`
	Fields         map[string]any `json:"fields" jsonschema:"captured qualification fields"`
}

func (h *handlers) updateQualification(_ context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[QualifyIn]) (*mcp.CallToolResultFor[*crm.Lead], error) {
	in := p.Arguments
	conv, err := h.store.GetConversation(in.ConversationID)
	if err != nil {
		return errResult[*crm.Lead](err.Error()), nil
	}
	lead, err := h.store.GetLead(conv.LeadID)
	if err != nil {
		return errResult[*crm.Lead](err.Error()), nil
	}
	qual := lead.Qualification
	if qual == nil {
		qual = map[string]any{}
	}
	qual[in.Framework] = in.Fields
	updated, err := h.store.UpdateLead(lead.ID, map[string]any{"qualification": qual})
	if err != nil {
		return errResult[*crm.Lead](err.Error()), nil
	}
	_, _ = h.store.AddActivity(&crm.Activity{
		ConversationID: in.ConversationID,
		Type:           "qualification",
		Summary:        fmt.Sprintf("Captured %s qualification", in.Framework),
		Payload:        in.Fields,
	})
	return okResult(fmt.Sprintf("Captured %s qualification for lead %s", in.Framework, lead.ID), updated), nil
}

func okResult[T any](text string, out T) *mcp.CallToolResultFor[T] {
	return &mcp.CallToolResultFor[T]{
		Content:           []mcp.Content{&mcp.TextContent{Text: text}},
		StructuredContent: out,
	}
}

func errResult[T any](text string) *mcp.CallToolResultFor[T] {
	return &mcp.CallToolResultFor[T]{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: true,
	}
}
