// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package intake turns an inbound CustomAIze handoff into CRM state. CustomAIze
// runs marketing and sends the first email; when it hands a lead to sales it
// POSTs a handoff to this service (see webhook.go). A Processor maps that payload
// onto the crm.Store — creating the lead, contact, conversation, the already-sent
// first email, and a handoff activity — and routes the lead to a customer-facing
// role via the supervisor. Delivery is idempotent on the handoff id.
package intake

import (
	"fmt"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/agent"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/crm"
)

// HandoffPayload is the contract CustomAIze POSTs to /webhooks/handoff. It is
// intentionally small and stable; unknown fields are ignored, and freeform
// marketing context travels in Context so the schema need not change to carry it.
type HandoffPayload struct {
	// HandoffID is the sender's unique id for this handoff. It is required and
	// used as the idempotency key: replaying the same HandoffID is a no-op.
	HandoffID string `json:"handoff_id"`
	// OccurredAt is when CustomAIze produced the handoff (optional).
	OccurredAt time.Time `json:"occurred_at"`
	// Source identifies the sender, e.g. "customaize" (optional).
	Source string `json:"source"`
	// Lead describes the company/opportunity.
	Lead LeadInput `json:"lead"`
	// Contact describes the person to engage.
	Contact ContactInput `json:"contact"`
	// FirstEmail is the marketing email CustomAIze already sent, if any.
	FirstEmail *EmailInput `json:"first_email,omitempty"`
	// Context carries arbitrary marketing metadata (campaign, notes, signals).
	Context map[string]any `json:"context,omitempty"`
}

// LeadInput is the company/opportunity portion of a handoff.
type LeadInput struct {
	Company string `json:"company"`
	Domain  string `json:"domain"`
	// Source is the lead origin; defaults to "marketing" when empty.
	Source string `json:"source"`
}

// ContactInput is the person portion of a handoff, including channel consent.
type ContactInput struct {
	Name         string `json:"name"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	Title        string `json:"title"`
	Timezone     string `json:"timezone"`
	ConsentEmail bool   `json:"consent_email"`
	ConsentSMS   bool   `json:"consent_sms"`
}

// EmailInput is the first (marketing) email already sent on the thread.
type EmailInput struct {
	Subject       string    `json:"subject"`
	Body          string    `json:"body"`
	GmailThreadID string    `json:"gmail_thread_id"`
	ExternalID    string    `json:"external_id"`
	SentAt        time.Time `json:"sent_at"`
}

// Result reports what a handoff created (or matched, on replay).
type Result struct {
	HandoffID      string `json:"handoff_id"`
	LeadID         string `json:"lead_id"`
	ContactID      string `json:"contact_id"`
	ConversationID string `json:"conversation_id"`
	OwnerRole      string `json:"owner_role"`
	// Duplicate is true when this HandoffID was already processed.
	Duplicate bool `json:"duplicate"`
}

// validate reports the first problem with a payload, or nil if it is usable.
func (p *HandoffPayload) validate() error {
	switch {
	case p.HandoffID == "":
		return fmt.Errorf("handoff_id is required")
	case p.Contact.Email == "":
		return fmt.Errorf("contact.email is required")
	case p.Lead.Company == "" && p.Lead.Domain == "":
		return fmt.Errorf("lead.company or lead.domain is required")
	}
	return nil
}

// Processor applies handoffs to a crm.Store. It is safe for concurrent use.
type Processor struct {
	store crm.Store

	mu   sync.Mutex
	seen map[string]Result // idempotency cache keyed by HandoffID
}

// NewProcessor returns a Processor backed by the given store.
//
// Idempotency here is in-process (a seen-id cache), which is enough for the
// in-memory store and tests. A Supabase-backed store gets durable exactly-once
// delivery for free via the unique handoffs.external_id index (see
// migrations/0001_init.sql).
func NewProcessor(store crm.Store) *Processor {
	return &Processor{store: store, seen: map[string]Result{}}
}

// Process records a handoff: it creates the lead, contact, conversation, the
// already-sent first email, and a handoff activity, and routes the lead to a
// customer-facing role. Replaying a HandoffID returns the original Result with
// Duplicate set and writes nothing.
func (p *Processor) Process(payload *HandoffPayload) (Result, error) {
	if err := payload.validate(); err != nil {
		return Result{}, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if prev, ok := p.seen[payload.HandoffID]; ok {
		prev.Duplicate = true
		return prev, nil
	}

	// The supervisor routes a fresh handoff to a customer-facing role; the lead
	// arrives already owned so the agent loop can pick it up immediately.
	owner := agent.Route(agent.EventHandoff, "new")

	source := payload.Lead.Source
	if source == "" {
		source = "marketing"
	}
	lead, err := p.store.CreateLead(&crm.Lead{
		Source:    source,
		Company:   payload.Lead.Company,
		Domain:    payload.Lead.Domain,
		Status:    "new",
		OwnerRole: owner,
	})
	if err != nil {
		return Result{}, fmt.Errorf("create lead: %w", err)
	}

	contact, err := p.store.CreateContact(&crm.Contact{
		LeadID:       lead.ID,
		Name:         payload.Contact.Name,
		Email:        payload.Contact.Email,
		Phone:        payload.Contact.Phone,
		Title:        payload.Contact.Title,
		Timezone:     payload.Contact.Timezone,
		ConsentEmail: payload.Contact.ConsentEmail,
		ConsentSMS:   payload.Contact.ConsentSMS,
	})
	if err != nil {
		return Result{}, fmt.Errorf("create contact: %w", err)
	}

	conv := &crm.Conversation{
		LeadID:         lead.ID,
		ContactID:      contact.ID,
		ChannelPrimary: "email",
		CurrentRole:    owner,
		Status:         "engaged",
	}
	if payload.FirstEmail != nil {
		conv.GmailThreadID = payload.FirstEmail.GmailThreadID
	}
	conv, err = p.store.CreateConversation(conv)
	if err != nil {
		return Result{}, fmt.Errorf("create conversation: %w", err)
	}

	// Record the marketing email that's already on the thread so the agent has
	// the full history from turn one.
	if e := payload.FirstEmail; e != nil {
		if _, err := p.store.AddMessage(&crm.Message{
			ConversationID: conv.ID,
			Direction:      "out",
			Channel:        "email",
			ExternalID:     e.ExternalID,
			Subject:        e.Subject,
			Body:           e.Body,
			CreatedAt:      e.SentAt,
		}); err != nil {
			return Result{}, fmt.Errorf("add first email: %w", err)
		}
	}

	if _, err := p.store.AddActivity(&crm.Activity{
		ConversationID: conv.ID,
		Type:           "handoff_received",
		Summary:        fmt.Sprintf("Handoff %s from %s → routed to %s", payload.HandoffID, orElse(payload.Source, "customaize"), owner),
		Payload: map[string]any{
			"handoff_id": payload.HandoffID,
			"source":     payload.Source,
			"context":    payload.Context,
		},
	}); err != nil {
		return Result{}, fmt.Errorf("log handoff activity: %w", err)
	}

	res := Result{
		HandoffID:      payload.HandoffID,
		LeadID:         lead.ID,
		ContactID:      contact.ID,
		ConversationID: conv.ID,
		OwnerRole:      owner,
	}
	p.seen[payload.HandoffID] = res
	return res, nil
}

// orElse returns s, or def when s is empty.
func orElse(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
