// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package intel exposes enrichment and lead-scoring tools. score_lead is backed
// by the real rules-based scorer; enrich_lead is a stub in Phase 0 and will be
// backed by a data-enrichment provider later.
package intel

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/crm"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/scoring"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// New returns the intel MCP server backed by the given store.
func New(store crm.Store) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "intel", Version: "0.1.0"}, nil)
	h := &handlers{store: store}
	mcp.AddTool(s, &mcp.Tool{Name: "enrich_lead", Description: "Enrich a lead with firmographic data (stub in Phase 0)."}, h.enrichLead)
	mcp.AddTool(s, &mcp.Tool{Name: "score_lead", Description: "Score a lead 0-100 and assign a tier (A/B/C) with explanation."}, h.scoreLead)
	return s
}

type handlers struct {
	store crm.Store
}

type LeadIDIn struct {
	LeadID string `json:"lead_id" jsonschema:"the lead id"`
}

type EnrichOut struct {
	Domain      string `json:"domain"`
	Industry    string `json:"industry"`
	SizeBucket  string `json:"size_bucket"`
	Enriched    bool   `json:"enriched"`
	Provenance  string `json:"provenance"`
}

func (h *handlers) enrichLead(_ context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[LeadIDIn]) (*mcp.CallToolResultFor[EnrichOut], error) {
	lead, err := h.store.GetLead(p.Arguments.LeadID)
	if err != nil {
		return errResult[EnrichOut](err.Error()), nil
	}
	// Phase 0 stub: derive a plausible industry/size from the domain.
	out := EnrichOut{
		Domain:     lead.Domain,
		Industry:   guessIndustry(lead.Domain),
		SizeBucket: "11-50",
		Enriched:   true,
		Provenance: "stub",
	}
	return okResult(fmt.Sprintf("Enriched %s: industry=%s size=%s (stub)", lead.Domain, out.Industry, out.SizeBucket), out), nil
}

func (h *handlers) scoreLead(_ context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[LeadIDIn]) (*mcp.CallToolResultFor[scoring.Result], error) {
	lead, err := h.store.GetLead(p.Arguments.LeadID)
	if err != nil {
		return errResult[scoring.Result](err.Error()), nil
	}
	contacts, _ := h.store.ListContacts(lead.ID)
	// Gather messages across this lead's conversations is out of scope for the
	// store interface here; Phase 0 scores on lead + contacts (intent signal
	// grows once conversation message history is wired in).
	res := scoring.Score(lead, contacts, nil)
	text := fmt.Sprintf("Lead %s scored %d (tier %s): %s", lead.ID, res.Score, res.Tier, strings.Join(res.Factors, "; "))
	return okResult(text, res), nil
}

func guessIndustry(domain string) string {
	switch {
	case strings.Contains(domain, "shop"), strings.Contains(domain, "store"):
		return "Retail / E-commerce"
	case strings.Contains(domain, "health"), strings.Contains(domain, "care"):
		return "Healthcare"
	case strings.Contains(domain, "soft"), strings.Contains(domain, "tech"), strings.Contains(domain, "ai"):
		return "Software / Technology"
	default:
		return "General Business"
	}
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
