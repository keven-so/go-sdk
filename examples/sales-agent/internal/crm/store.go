// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package crm

import (
	"fmt"
	"sync"
	"time"
)

// Store is the persistence abstraction every layer depends on. The in-memory
// implementation below mirrors the shape used by examples/memory in this repo;
// a Supabase-backed implementation will satisfy the same interface in Phase 1.
type Store interface {
	CreateLead(*Lead) (*Lead, error)
	GetLead(id string) (*Lead, error)
	UpdateLead(id string, fields map[string]any) (*Lead, error)

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
	}
}

func (s *MemoryStore) id(prefix string) string {
	s.seq++
	return fmt.Sprintf("%s_%d", prefix, s.seq)
}

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
