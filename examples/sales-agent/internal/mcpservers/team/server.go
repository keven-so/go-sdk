// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package team exposes the support agents (Researcher, Copywriter, RevOps,
// Sales Coach) as MCP tools using the agent-as-tool pattern: each tool runs a
// constrained sub-agent loop over the base CRM/intel tools and returns its
// output. Customer-facing agents call these tools; no tool calls itself.
package team

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/agent"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/llm"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// New returns the team MCP server. inner is a ToolBridge over the base tool
// servers (crm + intel) that support agents are allowed to use. live is the LLM
// used for sub-agents in live mode; in dry-run a deterministic Static LLM is
// used instead so nothing hits the network.
func New(dryRun bool, live llm.LLM, inner *agent.ToolBridge) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "team", Version: "0.1.0"}, nil)
	h := &handlers{dryRun: dryRun, live: live, inner: inner}
	mcp.AddTool(s, &mcp.Tool{Name: "research_account", Description: "Run the Researcher sub-agent to produce an account brief with intent signals."}, h.researchAccount)
	mcp.AddTool(s, &mcp.Tool{Name: "draft_message", Description: "Run the Copywriter sub-agent to draft personalized copy for a channel and intent."}, h.draftMessage)
	mcp.AddTool(s, &mcp.Tool{Name: "prioritize_lead", Description: "Run the RevOps sub-agent to score/tier a lead and recommend channel and timing."}, h.prioritizeLead)
	mcp.AddTool(s, &mcp.Tool{Name: "analyze_pipeline", Description: "Run the RevOps sub-agent to summarize pipeline health and the highest-leverage action."}, h.analyzePipeline)
	mcp.AddTool(s, &mcp.Tool{Name: "coach_review", Description: "Run the Sales Coach sub-agent to review a conversation and recommend the next best action."}, h.coachReview)
	return s
}

type handlers struct {
	dryRun bool
	live   llm.LLM
	inner  *agent.ToolBridge
}

// run executes a support role as a sub-agent. In dry-run it uses a deterministic
// canned reply; otherwise it runs a real sub-agent loop with the live LLM.
func (h *handlers) run(ctx context.Context, role agent.Role, input, canned string) (string, error) {
	var brain llm.LLM = h.live
	if h.dryRun {
		brain = llm.NewStatic(canned)
	}
	sa := agent.SubAgent{Role: role, LLM: brain, Bridge: h.inner, MaxTurns: 6}
	return sa.Run(ctx, input)
}

// --- research_account ---

type ResearchIn struct {
	LeadID string `json:"lead_id" jsonschema:"the lead id to research"`
}

type TextOut struct {
	Result string `json:"result"`
}

func (h *handlers) researchAccount(ctx context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[ResearchIn]) (*mcp.CallToolResultFor[TextOut], error) {
	in := p.Arguments
	canned := "Account brief (dry-run): mid-market software co.; contact is a senior ops decision-maker; likely pain = manual ops busywork; signal = recent scaling. Why now: hiring spike suggests growing pains."
	out, err := h.run(ctx, agent.Researcher, fmt.Sprintf("Research lead_id=%s and produce an account brief.", in.LeadID), canned)
	return finish(out, err)
}

// --- draft_message ---

type DraftIn struct {
	ConversationID string `json:"conversation_id" jsonschema:"the conversation id"`
	LeadID         string `json:"lead_id" jsonschema:"the lead id for personalization"`
	Channel        string `json:"channel" jsonschema:"email | sms | call"`
	Intent         string `json:"intent" jsonschema:"what the message should accomplish"`
	KeyPoints      string `json:"key_points,omitempty" jsonschema:"optional points to include"`
}

func (h *handlers) draftMessage(ctx context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[DraftIn]) (*mcp.CallToolResultFor[TextOut], error) {
	in := p.Arguments
	canned := fmt.Sprintf("Draft (dry-run, %s): Subject: A quick idea for your ops team — Hi there, following up on our note. Teams your size usually reclaim ~8 hrs/week. Open to a 20-min look this week?", in.Channel)
	prompt := fmt.Sprintf("Draft a %s message for lead_id=%s. Intent: %s. Key points: %s", in.Channel, in.LeadID, in.Intent, in.KeyPoints)
	out, err := h.run(ctx, agent.Copywriter, prompt, canned)
	return finish(out, err)
}

// --- prioritize_lead ---

func (h *handlers) prioritizeLead(ctx context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[ResearchIn]) (*mcp.CallToolResultFor[TextOut], error) {
	in := p.Arguments
	canned := "Priority (dry-run): tier B (score ~55). Best next channel: email now, SMS in 2 days. Recommendation: personalize on the scaling signal and offer a specific time."
	out, err := h.run(ctx, agent.RevOps, fmt.Sprintf("Prioritize lead_id=%s: score, tier, best channel and timing.", in.LeadID), canned)
	return finish(out, err)
}

// --- analyze_pipeline ---

type AnalyzeIn struct {
	Scope string `json:"scope,omitempty" jsonschema:"optional scope, e.g. 'this week' or a segment"`
}

func (h *handlers) analyzePipeline(ctx context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[AnalyzeIn]) (*mcp.CallToolResultFor[TextOut], error) {
	canned := "Pipeline (dry-run): healthy top-of-funnel, thin mid-stage. Bottleneck: discovery→proposal. Highest-leverage action: tighten MEDDICC on the 3 oldest open opps."
	prompt := "Analyze pipeline health."
	if p.Arguments.Scope != "" {
		prompt += fmt.Sprintf(" Scope: %s", p.Arguments.Scope)
	}
	out, err := h.run(ctx, agent.RevOps, prompt, canned)
	return finish(out, err)
}

// --- coach_review ---

type CoachIn struct {
	ConversationID string `json:"conversation_id" jsonschema:"the conversation id to review"`
	LeadID         string `json:"lead_id" jsonschema:"the lead id for context"`
}

func (h *handlers) coachReview(ctx context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[CoachIn]) (*mcp.CallToolResultFor[TextOut], error) {
	in := p.Arguments
	canned := "Coach review (dry-run): strong rapport, but Economic buyer and Metrics are unconfirmed. Next best action: ask for the success metric and who signs. Likely objection 'send info' — counter by proposing a 20-min tailored walkthrough."
	out, err := h.run(ctx, agent.SalesCoach, fmt.Sprintf("Review conversation_id=%s (lead_id=%s) and recommend the next best action.", in.ConversationID, in.LeadID), canned)
	return finish(out, err)
}

func finish(out string, err error) (*mcp.CallToolResultFor[TextOut], error) {
	if err != nil {
		return &mcp.CallToolResultFor[TextOut]{
			Content: []mcp.Content{&mcp.TextContent{Text: "sub-agent error: " + err.Error()}},
			IsError: true,
		}, nil
	}
	return &mcp.CallToolResultFor[TextOut]{
		Content:           []mcp.Content{&mcp.TextContent{Text: out}},
		StructuredContent: TextOut{Result: out},
	}, nil
}
