// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package crm

import (
	"testing"
	"time"
)

// TestNewSupabaseStoreValidation checks constructor validation and table prefixing.
func TestNewSupabaseStoreValidation(t *testing.T) {
	if _, err := NewSupabaseStore("", "key", ""); err == nil {
		t.Error("expected error for empty url")
	}
	if _, err := NewSupabaseStore("https://x.supabase.co", "", ""); err == nil {
		t.Error("expected error for empty key")
	}
	s, err := NewSupabaseStore("https://x.supabase.co/", "key", "sa_")
	if err != nil {
		t.Fatalf("NewSupabaseStore: %v", err)
	}
	if s.rest != "https://x.supabase.co/rest/v1" {
		t.Errorf("rest = %q, want trailing slash trimmed + /rest/v1", s.rest)
	}
	if got := s.table("leads"); got != "sa_leads" {
		t.Errorf("table(leads) = %q, want sa_leads", got)
	}
}

// TestInsertMapStripsDefaults verifies insertMap drops id, zero timestamps, and null fields.
func TestInsertMapStripsDefaults(t *testing.T) {
	// A lead with no id/timestamps and a nil qualification map: those keys must
	// be dropped so Postgres applies its column defaults.
	m, err := insertMap(&Lead{Company: "Acme", Score: 0, Status: "new"})
	if err != nil {
		t.Fatalf("insertMap: %v", err)
	}
	for _, k := range []string{"id", "created_at", "updated_at", "qualification"} {
		if _, ok := m[k]; ok {
			t.Errorf("expected %q to be stripped, got %v", k, m[k])
		}
	}
	if m["company"] != "Acme" {
		t.Errorf("company = %v, want Acme", m["company"])
	}
	// Zero-valued but meaningful fields stay (score 0, status set).
	if _, ok := m["score"]; !ok {
		t.Error("score should be present even when 0")
	}
	if m["status"] != "new" {
		t.Errorf("status = %v, want new", m["status"])
	}

	// A set timestamp is preserved.
	when := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	mm, _ := insertMap(&Message{ConversationID: "c1", Direction: "out", Channel: "email", CreatedAt: when})
	if _, ok := mm["created_at"]; !ok {
		t.Error("non-zero created_at should be preserved")
	}
	if _, ok := mm["id"]; ok {
		t.Error("empty id should be stripped")
	}
}
