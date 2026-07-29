// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package crm

import "testing"

// TestMemoryStoreLeadLifecycle exercises create/get/update of a lead in the in-memory store.
func TestMemoryStoreLeadLifecycle(t *testing.T) {
	s := NewMemoryStore()

	lead, err := s.CreateLead(&Lead{Company: "Acme", Domain: "acme.com", Source: "marketing", Status: "new"})
	if err != nil {
		t.Fatal(err)
	}
	if lead.ID == "" {
		t.Fatal("expected generated id")
	}

	if _, err := s.UpdateLead(lead.ID, map[string]any{"status": "engaged", "tier": "A", "score": 72}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetLead(lead.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "engaged" || got.Tier != "A" || got.Score != 72 {
		t.Fatalf("update not applied: %+v", got)
	}

	if _, err := s.GetLead("nope"); err == nil {
		t.Fatal("expected error for missing lead")
	}
}

// TestMemoryStoreContactsAndActivities covers contact and activity storage and listing.
func TestMemoryStoreContactsAndActivities(t *testing.T) {
	s := NewMemoryStore()
	lead, _ := s.CreateLead(&Lead{Company: "Acme"})
	if _, err := s.CreateContact(&Contact{LeadID: lead.ID, Name: "Dana", Email: "d@acme.com"}); err != nil {
		t.Fatal(err)
	}
	contacts, _ := s.ListContacts(lead.ID)
	if len(contacts) != 1 {
		t.Fatalf("want 1 contact, got %d", len(contacts))
	}

	conv, _ := s.CreateConversation(&Conversation{LeadID: lead.ID})
	if _, err := s.AddActivity(&Activity{ConversationID: conv.ID, Type: "note", Summary: "hi"}); err != nil {
		t.Fatal(err)
	}
	acts, _ := s.ListActivities(conv.ID)
	if len(acts) != 1 || acts[0].Summary != "hi" {
		t.Fatalf("unexpected activities: %+v", acts)
	}
}
