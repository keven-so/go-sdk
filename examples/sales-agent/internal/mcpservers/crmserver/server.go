// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package crmserver exposes the CRM store as MCP tools so the agent can read and
// write lead/deal state.
package crmserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/crm"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// New returns the crm MCP server backed by the given store.
func New(store crm.Store) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "crm", Version: "0.1.0"}, nil)
	h := &handlers{store: store}
	mcp.AddTool(s, &mcp.Tool{Name: "get_lead", Description: "Fetch a lead with its contacts."}, h.getLead)
	mcp.AddTool(s, &mcp.Tool{Name: "update_lead", Description: "Update fields on a lead (status, tier, score, owner_role)."}, h.updateLead)
	mcp.AddTool(s, &mcp.Tool{Name: "log_activity", Description: "Record an activity (note, stage change, call result) on a conversation."}, h.logActivity)
	mcp.AddTool(s, &mcp.Tool{Name: "create_deal", Description: "Create a sales opportunity for a lead at a given stage."}, h.createDeal)
	mcp.AddTool(s, &mcp.Tool{Name: "advance_stage", Description: "Advance a deal to a new stage with a reason."}, h.advanceStage)
	return s
}

type handlers struct {
	store crm.Store
}

type GetLeadIn struct {
	LeadID string `json:"lead_id" jsonschema:"the lead id"`
}

type GetLeadOut struct {
	Lead     *crm.Lead      `json:"lead"`
	Contacts []*crm.Contact `json:"contacts"`
}

func (h *handlers) getLead(_ context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[GetLeadIn]) (*mcp.CallToolResultFor[GetLeadOut], error) {
	lead, err := h.store.GetLead(p.Arguments.LeadID)
	if err != nil {
		return errResult[GetLeadOut](err.Error()), nil
	}
	contacts, err := h.store.ListContacts(lead.ID)
	if err != nil {
		return errResult[GetLeadOut](fmt.Sprintf("failed to list contacts: %v", err)), nil
	}
	out := GetLeadOut{Lead: lead, Contacts: contacts}
	text := fmt.Sprintf("Lead %s: %s (%s), status=%s tier=%s score=%d, %d contact(s)",
		lead.ID, lead.Company, lead.Domain, lead.Status, lead.Tier, lead.Score, len(contacts))
	return okResult(text, out), nil
}

type UpdateLeadIn struct {
	LeadID string         `json:"lead_id" jsonschema:"the lead id"`
	Fields map[string]any `json:"fields" jsonschema:"map of fields to update, e.g. status, tier, score"`
}

func (h *handlers) updateLead(_ context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[UpdateLeadIn]) (*mcp.CallToolResultFor[*crm.Lead], error) {
	lead, err := h.store.UpdateLead(p.Arguments.LeadID, p.Arguments.Fields)
	if err != nil {
		return errResult[*crm.Lead](err.Error()), nil
	}
	return okResult(fmt.Sprintf("Updated lead %s", lead.ID), lead), nil
}

type LogActivityIn struct {
	ConversationID string         `json:"conversation_id" jsonschema:"the conversation id"`
	Type           string         `json:"type" jsonschema:"activity type: note | stage_change | call_completed"`
	Summary        string         `json:"summary" jsonschema:"short human-readable summary"`
	Payload        map[string]any `json:"payload,omitempty" jsonschema:"optional structured detail"`
}

func (h *handlers) logActivity(_ context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[LogActivityIn]) (*mcp.CallToolResultFor[*crm.Activity], error) {
	in := p.Arguments
	a, err := h.store.AddActivity(&crm.Activity{
		ConversationID: in.ConversationID,
		Type:           in.Type,
		Summary:        in.Summary,
		Payload:        in.Payload,
	})
	if err != nil {
		return errResult[*crm.Activity](err.Error()), nil
	}
	return okResult(fmt.Sprintf("Logged activity %s: %s", a.ID, a.Summary), a), nil
}

type CreateDealIn struct {
	LeadID string  `json:"lead_id" jsonschema:"the lead id"`
	Stage  string  `json:"stage" jsonschema:"initial stage, e.g. discovery"`
	Amount float64 `json:"amount,omitempty" jsonschema:"optional deal amount"`
}

func (h *handlers) createDeal(_ context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[CreateDealIn]) (*mcp.CallToolResultFor[*crm.Deal], error) {
	in := p.Arguments
	stage := in.Stage
	if stage == "" {
		stage = "discovery"
	}
	d, err := h.store.CreateDeal(&crm.Deal{LeadID: in.LeadID, Stage: stage, Amount: in.Amount})
	if err != nil {
		return errResult[*crm.Deal](err.Error()), nil
	}
	return okResult(fmt.Sprintf("Created deal %s for lead %s at stage %s", d.ID, d.LeadID, d.Stage), d), nil
}

type AdvanceStageIn struct {
	DealID string `json:"deal_id" jsonschema:"the deal id"`
	Stage  string `json:"to_stage" jsonschema:"the new stage"`
	Reason string `json:"reason" jsonschema:"why the deal advanced"`
}

func (h *handlers) advanceStage(_ context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[AdvanceStageIn]) (*mcp.CallToolResultFor[*crm.Deal], error) {
	in := p.Arguments
	fields := map[string]any{"stage": in.Stage}
	if in.Stage == "closed_won" {
		fields["won"] = true
	}
	d, err := h.store.UpdateDeal(in.DealID, fields)
	if err != nil {
		return errResult[*crm.Deal](err.Error()), nil
	}
	return okResult(fmt.Sprintf("Deal %s -> %s (%s)", d.ID, d.Stage, in.Reason), d), nil
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
