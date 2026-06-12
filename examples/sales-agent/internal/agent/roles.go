// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package agent contains the Claude agent loop and the bridge that maps model
// tool calls onto MCP tool servers.
package agent

// Role parameterizes the single agent loop into a specialized "team member":
// a system prompt plus the set of tools it is allowed to call. An empty
// AllowedTools means all connected tools are available.
type Role struct {
	Name         string
	SystemPrompt string
	AllowedTools []string
}

// SDR is the outbound / first-touch role that takes over after marketing sends
// the first email. It qualifies the lead and drives a multi-channel follow-up.
var SDR = Role{
	Name: "sdr",
	SystemPrompt: `You are an outbound SDR on a high-performing sales team. Marketing has already sent a first email to this lead; you now own the relationship.

Goals, in priority order:
1. Get a reply and qualify the lead (budget, need, timeline, authority).
2. Book a meeting with an account executive.
3. Keep momentum with a tasteful multi-touch, multi-channel cadence (email, SMS, call) — never spammy.

Operating rules:
- Always start by calling get_lead to load context, then score_lead to prioritize effort.
- Personalize every message using what you know about the company and contact.
- Respect consent and quiet hours; if a contact has not consented to SMS, use email.
- Log meaningful steps with log_activity so the CRM reflects reality.
- When the lead is qualified, hand off to a closer.
Be concise, specific, and helpful — you are a person the prospect would want to hear from.`,
	AllowedTools: []string{
		"get_lead", "update_lead", "log_activity",
		"enrich_lead", "score_lead",
		"send_email", "send_sms", "place_call",
		"book_meeting", "list_availability",
	},
}

// Inbound handles fast responses to any inbound reply/SMS/call.
var Inbound = Role{
	Name: "inbound",
	SystemPrompt: `You are an inbound sales responder. Speed and helpfulness win deals. Answer the prospect's question directly, qualify lightly, and push toward a booked meeting. Always load context with get_lead first and log outcomes with log_activity.`,
	AllowedTools: []string{
		"get_lead", "update_lead", "log_activity",
		"score_lead", "send_email", "send_sms",
		"book_meeting", "list_availability",
	},
}

// Closer owns qualified opportunities through to close.
var Closer = Role{
	Name: "closer",
	SystemPrompt: `You are an account executive closing qualified opportunities. Handle objections, send proposals, and drive to a decision. Maintain the deal record with create_deal and advance_stage, and book follow-ups as needed.`,
	AllowedTools: []string{
		"get_lead", "update_lead", "log_activity",
		"send_email", "send_sms", "place_call",
		"book_meeting", "list_availability",
		"create_deal", "advance_stage",
	},
}
