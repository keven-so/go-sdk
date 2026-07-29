// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package crm holds the data model for the sales system and a storage
// abstraction. Phase 0 ships an in-memory implementation; later phases swap in
// a Supabase/Postgres-backed Store implementing the same interface.
package crm

import "time"

// Lead is a company/opportunity handed off from marketing or generated outbound.
type Lead struct {
	ID        string `json:"id"`
	Source    string `json:"source"` // marketing | inbound | outbound
	Company   string `json:"company"`
	Domain    string `json:"domain"`
	Status    string `json:"status"` // new | working | engaged | meeting | won | lost
	Score     int    `json:"score"`
	Tier      string `json:"tier"` // A | B | C
	OwnerRole string `json:"owner_role"`
	// Qualification holds framework-keyed capture, e.g. {"bant": {...}, "meddicc": {...}}.
	Qualification map[string]any `json:"qualification,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// Contact is a person at a Lead's company.
type Contact struct {
	ID           string `json:"id"`
	LeadID       string `json:"lead_id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	Title        string `json:"title"`
	Timezone     string `json:"timezone"`
	ConsentEmail bool   `json:"consent_email"`
	ConsentSMS   bool   `json:"consent_sms"`
}

// Conversation is one ongoing thread with a contact across channels.
type Conversation struct {
	ID             string `json:"id"`
	LeadID         string `json:"lead_id"`
	ContactID      string `json:"contact_id"`
	ChannelPrimary string `json:"channel_primary"`
	CurrentRole    string `json:"current_role"` // sdr | inbound | closer | voice
	Status         string `json:"status"`
	GmailThreadID  string `json:"gmail_thread_id"`
}

// Message is a single inbound or outbound communication.
type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Direction      string    `json:"direction"` // in | out
	Channel        string    `json:"channel"`   // email | sms | voice
	ExternalID     string    `json:"external_id"`
	Subject        string    `json:"subject"`
	Body           string    `json:"body"`
	CreatedAt      time.Time `json:"created_at"`
}

// Activity is an audit-trail entry (tool call, stage change, note, call result).
type Activity struct {
	ID             string         `json:"id"`
	ConversationID string         `json:"conversation_id"`
	Type           string         `json:"type"`
	Summary        string         `json:"summary"`
	Payload        map[string]any `json:"payload,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

// Handoff is a durable record of an inbound marketing→sales handoff, keyed by a
// unique ExternalID (the sender's handoff id). Claiming it before any CRM writes
// makes intake idempotent: a replay of the same ExternalID is rejected by the
// unique index instead of creating duplicate leads.
type Handoff struct {
	ID             string         `json:"id"`
	ExternalID     string         `json:"external_id"`
	Source         string         `json:"source"`
	LeadID         string         `json:"lead_id"`
	ConversationID string         `json:"conversation_id"`
	Payload        map[string]any `json:"payload,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

// Deal is a sales opportunity tracked through stages.
type Deal struct {
	ID            string         `json:"id"`
	LeadID        string         `json:"lead_id"`
	Stage         string         `json:"stage"` // discovery | proposal | negotiation | closed_won | closed_lost
	Amount        float64        `json:"amount"`
	Won           bool           `json:"won"`
	Qualification map[string]any `json:"qualification,omitempty"`
}
