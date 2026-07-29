// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package crm

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrHandoffExists is returned by CreateHandoff when a handoff with the same
// ExternalID already exists. Intake treats it as a duplicate delivery.
var ErrHandoffExists = errors.New("handoff already exists")

// Store is the persistence abstraction every layer depends on. The in-memory
// implementation below mirrors the shape used by examples/memory in this repo;
// a Supabase-backed implementation will satisfy the same interface in Phase 1.
type Store interface {
	CreateLead(*Lead) (*Lead, error)
	GetLead(id string) (*Lead, error)
	UpdateLead(id string, fields map[string]any) (*Lead, error)

	// CreateHandoff claims a handoff, returning ErrHandoffExists if one with the
	// same ExternalID was already claimed (the durable idempotency guard).
	CreateHandoff(*Handoff) (*Handoff, error)
	GetHandoff(externalID string) (*Handoff, error)
	UpdateHandoff(externalID string, fields map[string]any) (*Handoff, error)

	CreateContact(*Contact) (*Contact, error)
	ListContacts(leadID string) ([]*Contact, error)

	CreateConversation(*Conversation) (*Conversation, error)
	GetConversation(id string) (*Conversation, error)
	UpdateConversation(id string, fields map[string]any) (*Conversation, error)

	AddMessage(*Message) (*Message, error)
	ListMessages(conversationID string) ([]*Message, error)

	AddActivity(*Activity) (*Activity, error)
	ListActivities(conversationID string) ([]*Activity, error)

	CreateDeal(*Deal) (*Deal, error)
	UpdateDeal(id string, fields map[string]any) (*Deal, error)
}

// MemoryStore is a thread-safe in-memory Store for dev, tests, and dry runs.
type MemoryStore struct {
	mu            sync.Mutex
	seq           int
	leads         map[string]*Lead
	contacts      map[string]*Contact
	conversations map[string]*Conversation
	messages      map[string]*Message
	activities    map[string]*Activity
	deals         map[string]*Deal
	handoffs      map[string]*Handoff // keyed by ExternalID
}

// NewMemoryStore returns an empty in-memory Store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		leads:         map[string]*Lead{},
		contacts:      map[string]*Contact{},
		conversations: map[string]*Conversation{},
		messages:      map[string]*Message{},
		activities:    map[string]*Activity{},
		deals:         map[string]*Deal{},
		handoffs:      map[string]*Handoff{},
	}
}

// id returns a new unique id with the given prefix.
func (s *MemoryStore) id(prefix string) string {
	s.seq++
	return fmt.Sprintf("%s_%d", prefix, s.seq)
}

// CreateLead stores a copy of the lead, assigning an id and timestamps.
func (s *MemoryStore) CreateLead(l *Lead) (*Lead, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *l
	if cp.ID == "" {
		cp.ID = s.id("lead")
	}
	now := time.Now().UTC()
	cp.CreatedAt, cp.UpdatedAt = now, now
	s.leads[cp.ID] = &cp
	return &cp, nil
}

// GetLead returns a copy of the lead with the given id.
func (s *MemoryStore) GetLead(id string) (*Lead, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.leads[id]
	if !ok {
		return nil, fmt.Errorf("lead %q not found", id)
	}
	cp := *l
	return &cp, nil
}

// UpdateLead applies the named field updates to a lead and returns the result.
func (s *MemoryStore) UpdateLead(id string, fields map[string]any) (*Lead, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.leads[id]
	if !ok {
		return nil, fmt.Errorf("lead %q not found", id)
	}
	for k, v := range fields {
		switch k {
		case "status":
			l.Status, _ = v.(string)
		case "tier":
			l.Tier, _ = v.(string)
		case "owner_role":
			l.OwnerRole, _ = v.(string)
		case "company":
			l.Company, _ = v.(string)
		case "domain":
			l.Domain, _ = v.(string)
		case "score":
			l.Score = toInt(v)
		case "qualification":
			if m, ok := v.(map[string]any); ok {
				l.Qualification = m
			}
		}
	}
	l.UpdatedAt = time.Now().UTC()
	cp := *l
	return &cp, nil
}

// CreateContact stores a copy of the contact, assigning an id if unset.
func (s *MemoryStore) CreateContact(c *Contact) (*Contact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *c
	if cp.ID == "" {
		cp.ID = s.id("contact")
	}
	s.contacts[cp.ID] = &cp
	return &cp, nil
}

// ListContacts returns copies of all contacts belonging to the given lead.
func (s *MemoryStore) ListContacts(leadID string) ([]*Contact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Contact
	for _, c := range s.contacts {
		if c.LeadID == leadID {
			cp := *c
			out = append(out, &cp)
		}
	}
	return out, nil
}

// CreateConversation stores a copy of the conversation, assigning an id if unset.
func (s *MemoryStore) CreateConversation(c *Conversation) (*Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *c
	if cp.ID == "" {
		cp.ID = s.id("conv")
	}
	s.conversations[cp.ID] = &cp
	return &cp, nil
}

// GetConversation returns a copy of the conversation with the given id.
func (s *MemoryStore) GetConversation(id string) (*Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.conversations[id]
	if !ok {
		return nil, fmt.Errorf("conversation %q not found", id)
	}
	cp := *c
	return &cp, nil
}

// UpdateConversation applies the named field updates to a conversation.
func (s *MemoryStore) UpdateConversation(id string, fields map[string]any) (*Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.conversations[id]
	if !ok {
		return nil, fmt.Errorf("conversation %q not found", id)
	}
	for k, v := range fields {
		switch k {
		case "current_role":
			c.CurrentRole, _ = v.(string)
		case "status":
			c.Status, _ = v.(string)
		case "channel_primary":
			c.ChannelPrimary, _ = v.(string)
		}
	}
	cp := *c
	return &cp, nil
}

// AddMessage stores a copy of the message, assigning an id and timestamp if unset.
func (s *MemoryStore) AddMessage(m *Message) (*Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *m
	if cp.ID == "" {
		cp.ID = s.id("msg")
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now().UTC()
	}
	s.messages[cp.ID] = &cp
	return &cp, nil
}

// ListMessages returns copies of all messages in the given conversation.
func (s *MemoryStore) ListMessages(conversationID string) ([]*Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Message
	for _, m := range s.messages {
		if m.ConversationID == conversationID {
			cp := *m
			out = append(out, &cp)
		}
	}
	return out, nil
}

// AddActivity stores a copy of the activity, assigning an id and timestamp if unset.
func (s *MemoryStore) AddActivity(a *Activity) (*Activity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *a
	if cp.ID == "" {
		cp.ID = s.id("act")
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now().UTC()
	}
	s.activities[cp.ID] = &cp
	return &cp, nil
}

// ListActivities returns copies of all activities in the given conversation.
func (s *MemoryStore) ListActivities(conversationID string) ([]*Activity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Activity
	for _, a := range s.activities {
		if a.ConversationID == conversationID {
			cp := *a
			out = append(out, &cp)
		}
	}
	return out, nil
}

// CreateDeal stores a copy of the deal, assigning an id if unset.
func (s *MemoryStore) CreateDeal(d *Deal) (*Deal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *d
	if cp.ID == "" {
		cp.ID = s.id("deal")
	}
	s.deals[cp.ID] = &cp
	return &cp, nil
}

// UpdateDeal applies the named field updates to a deal and returns the result.
func (s *MemoryStore) UpdateDeal(id string, fields map[string]any) (*Deal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.deals[id]
	if !ok {
		return nil, fmt.Errorf("deal %q not found", id)
	}
	for k, v := range fields {
		switch k {
		case "stage":
			d.Stage, _ = v.(string)
		case "amount":
			d.Amount = toFloat(v)
		case "won":
			d.Won, _ = v.(bool)
		}
	}
	cp := *d
	return &cp, nil
}

// CreateHandoff claims a handoff, returning ErrHandoffExists if its ExternalID
// was already claimed.
func (s *MemoryStore) CreateHandoff(h *Handoff) (*Handoff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if h.ExternalID == "" {
		return nil, fmt.Errorf("handoff external_id is required")
	}
	if _, ok := s.handoffs[h.ExternalID]; ok {
		return nil, ErrHandoffExists
	}
	cp := *h
	if cp.ID == "" {
		cp.ID = s.id("handoff")
	}
	cp.CreatedAt = time.Now().UTC()
	s.handoffs[cp.ExternalID] = &cp
	return &cp, nil
}

// GetHandoff returns the handoff with the given external id.
func (s *MemoryStore) GetHandoff(externalID string) (*Handoff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.handoffs[externalID]
	if !ok {
		return nil, fmt.Errorf("handoff %q not found", externalID)
	}
	cp := *h
	return &cp, nil
}

// UpdateHandoff applies the named field updates to a handoff and returns it.
func (s *MemoryStore) UpdateHandoff(externalID string, fields map[string]any) (*Handoff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.handoffs[externalID]
	if !ok {
		return nil, fmt.Errorf("handoff %q not found", externalID)
	}
	for k, v := range fields {
		switch k {
		case "lead_id":
			h.LeadID, _ = v.(string)
		case "conversation_id":
			h.ConversationID, _ = v.(string)
		}
	}
	cp := *h
	return &cp, nil
}

// toInt coerces a JSON-decoded numeric value to an int, returning 0 otherwise.
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

// toFloat coerces a JSON-decoded numeric value to a float64, returning 0 otherwise.
func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}
