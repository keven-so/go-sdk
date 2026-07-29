// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package intake

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/crm"
)

// samplePayload returns a valid handoff for tests.
func samplePayload() *HandoffPayload {
	return &HandoffPayload{
		HandoffID: "ho_123",
		Source:    "customaize",
		Lead:      LeadInput{Company: "Acme Corp", Domain: "acme.com"},
		Contact: ContactInput{
			Name: "Jane Doe", Email: "jane@acme.com", Phone: "+15550001111",
			Title: "VP Ops", Timezone: "America/New_York", ConsentEmail: true,
		},
		FirstEmail: &EmailInput{
			Subject: "A quick idea", Body: "Hi Jane...",
			GmailThreadID: "thread_1", ExternalID: "msg_1",
		},
		Context: map[string]any{"campaign": "q2-ops"},
	}
}

// TestProcessCreatesCRMState verifies a handoff creates the lead, contact, conversation, first email, and activity.
func TestProcessCreatesCRMState(t *testing.T) {
	store := crm.NewMemoryStore()
	p := NewProcessor(store)

	res, err := p.Process(samplePayload())
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if res.Duplicate {
		t.Error("first handoff marked duplicate")
	}
	if res.LeadID == "" || res.ContactID == "" || res.ConversationID == "" {
		t.Fatalf("missing ids in result: %+v", res)
	}
	if res.OwnerRole == "" {
		t.Error("expected a routed owner role")
	}

	lead, err := store.GetLead(res.LeadID)
	if err != nil {
		t.Fatalf("GetLead: %v", err)
	}
	if lead.Company != "Acme Corp" || lead.Status != "new" || lead.Source != "marketing" {
		t.Errorf("unexpected lead: %+v", lead)
	}
	if lead.OwnerRole != res.OwnerRole {
		t.Errorf("lead owner %q != result owner %q", lead.OwnerRole, res.OwnerRole)
	}

	contacts, _ := store.ListContacts(res.LeadID)
	if len(contacts) != 1 || contacts[0].Email != "jane@acme.com" {
		t.Errorf("unexpected contacts: %+v", contacts)
	}

	msgs, _ := store.ListMessages(res.ConversationID)
	if len(msgs) != 1 || msgs[0].Direction != "out" || msgs[0].Channel != "email" {
		t.Errorf("expected one outbound email, got: %+v", msgs)
	}

	acts, _ := store.ListActivities(res.ConversationID)
	if len(acts) != 1 || acts[0].Type != "handoff_received" {
		t.Errorf("expected one handoff_received activity, got: %+v", acts)
	}
}

// TestProcessIsIdempotent verifies a replayed handoff returns the original ids and writes nothing new.
func TestProcessIsIdempotent(t *testing.T) {
	store := crm.NewMemoryStore()
	p := NewProcessor(store)

	first, err := p.Process(samplePayload())
	if err != nil {
		t.Fatalf("Process #1: %v", err)
	}
	second, err := p.Process(samplePayload())
	if err != nil {
		t.Fatalf("Process #2: %v", err)
	}
	if !second.Duplicate {
		t.Error("replayed handoff not marked duplicate")
	}
	if second.LeadID != first.LeadID || second.ConversationID != first.ConversationID {
		t.Errorf("replay returned different ids: %+v vs %+v", first, second)
	}

	leads, _ := store.ListContacts(first.LeadID)
	if len(leads) != 1 {
		t.Errorf("replay created duplicate contacts: %d", len(leads))
	}
}

// TestProcessValidation checks required-field validation rejects malformed payloads.
func TestProcessValidation(t *testing.T) {
	store := crm.NewMemoryStore()
	p := NewProcessor(store)

	cases := map[string]func(*HandoffPayload){
		"missing handoff_id": func(p *HandoffPayload) { p.HandoffID = "" },
		"missing email":      func(p *HandoffPayload) { p.Contact.Email = "" },
		"missing company and domain": func(p *HandoffPayload) {
			p.Lead.Company, p.Lead.Domain = "", ""
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			payload := samplePayload()
			mutate(payload)
			if _, err := p.Process(payload); err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}
}

// TestWebhookSignature checks the handler accepts a valid HMAC signature and rejects a bad one.
func TestWebhookSignature(t *testing.T) {
	const secret = "shh"
	h := NewHandler(NewProcessor(crm.NewMemoryStore()), secret)
	body, _ := json.Marshal(samplePayload())

	// Valid signature → 201 Created.
	req := httptest.NewRequest(http.MethodPost, "/webhooks/handoff", bytes.NewReader(body))
	req.Header.Set(SignatureHeader, Sign([]byte(secret), body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("valid signature: got %d, want 201; body=%s", rec.Code, rec.Body)
	}
	var res Result
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.LeadID == "" {
		t.Error("expected a lead id in the response")
	}

	// Missing/invalid signature → 401.
	req = httptest.NewRequest(http.MethodPost, "/webhooks/handoff", bytes.NewReader(body))
	req.Header.Set(SignatureHeader, "sha256=deadbeef")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature: got %d, want 401", rec.Code)
	}
}

// TestWebhookMethodNotAllowed checks non-POST requests are rejected with 405.
func TestWebhookMethodNotAllowed(t *testing.T) {
	h := NewHandler(NewProcessor(crm.NewMemoryStore()), "")
	req := httptest.NewRequest(http.MethodGet, "/webhooks/handoff", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: got %d, want 405", rec.Code)
	}
}
