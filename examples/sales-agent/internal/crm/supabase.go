// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package crm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SupabaseStore is a Store backed by a Supabase project over its PostgREST API
// (SUPABASE_URL/rest/v1). It authenticates with the service-role key, which
// bypasses row-level security, so it is a server-side component only — never ship
// the key to a client. The schema it expects is migrations/0001_init.sql.
//
// Prefix is prepended to every table name. Use "" for a dedicated database and a
// value like "sa_" to namespace the tables inside a shared project's public
// schema (public is the schema PostgREST exposes by default).
type SupabaseStore struct {
	rest   string // <url>/rest/v1
	key    string
	prefix string
	httpc  *http.Client
}

// compile-time check that SupabaseStore satisfies the Store interface.
var _ Store = (*SupabaseStore)(nil)

// NewSupabaseStore returns a Store backed by the Supabase project at url using
// the given service-role key. prefix namespaces the table names (may be empty).
func NewSupabaseStore(url, serviceKey, prefix string) (*SupabaseStore, error) {
	if url == "" || serviceKey == "" {
		return nil, fmt.Errorf("supabase: url and service-role key are required")
	}
	return &SupabaseStore{
		rest:   strings.TrimRight(url, "/") + "/rest/v1",
		key:    serviceKey,
		prefix: prefix,
		httpc:  &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// table returns the prefixed table name.
func (s *SupabaseStore) table(name string) string { return s.prefix + name }

// req performs a PostgREST request and returns the raw response body. A non-2xx
// status is turned into an error carrying the status and body.
func (s *SupabaseStore) req(method, table, query string, body any) ([]byte, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	u := s.rest + "/" + table
	if query != "" {
		u += "?" + query
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, err
	}
	r.Header.Set("apikey", s.key)
	r.Header.Set("Authorization", "Bearer "+s.key)
	r.Header.Set("Prefer", "return=representation")
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.httpc.Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("supabase %s %s: %s: %s", method, table, resp.Status, strings.TrimSpace(string(data)))
	}
	return data, nil
}

// insert POSTs one row and decodes the returned representation into out.
func (s *SupabaseStore) insert(table string, v, out any) error {
	row, err := insertMap(v)
	if err != nil {
		return err
	}
	data, err := s.req(http.MethodPost, s.table(table), "", []any{row})
	if err != nil {
		return err
	}
	return decodeOne(data, table, out)
}

// eq builds a URL-encoded PostgREST equality filter (column=eq.value). Encoding
// the value keeps a caller-supplied id from injecting extra query parameters.
func eq(column, value string) string {
	return column + "=eq." + url.QueryEscape(value)
}

// getOne GETs the single row where column == value into out.
func (s *SupabaseStore) getOne(table, column, value string, out any) error {
	data, err := s.req(http.MethodGet, s.table(table), eq(column, value)+"&select=*&limit=1", nil)
	if err != nil {
		return err
	}
	return decodeOne(data, table, out)
}

// patch PATCHes the row where column == value, applying fields, into out.
func (s *SupabaseStore) patch(table, column, value string, fields map[string]any, out any) error {
	data, err := s.req(http.MethodPatch, s.table(table), eq(column, value), fields)
	if err != nil {
		return err
	}
	return decodeOne(data, table, out)
}

// list GETs all rows where column == value into out (a pointer to a slice),
// optionally ordered (e.g. "created_at.asc") for a stable sequence.
func (s *SupabaseStore) list(table, column, value, order string, out any) error {
	q := eq(column, value) + "&select=*"
	if order != "" {
		q += "&order=" + order
	}
	data, err := s.req(http.MethodGet, s.table(table), q, nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

// insertMap marshals v to a column map, dropping an empty id, zero timestamps,
// and null values so Postgres column defaults apply instead.
func insertMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	for k, val := range m {
		if val == nil {
			delete(m, k)
		}
	}
	if id, ok := m["id"].(string); ok && id == "" {
		delete(m, "id")
	}
	for _, k := range []string{"created_at", "updated_at"} {
		if t, ok := m[k].(string); ok && (t == "" || t == "0001-01-01T00:00:00Z") {
			delete(m, k)
		}
	}
	return m, nil
}

// decodeOne unmarshals the first row of a PostgREST array response into out,
// returning a not-found error when the array is empty.
func decodeOne(data []byte, table string, out any) error {
	var rows []json.RawMessage
	if err := json.Unmarshal(data, &rows); err != nil {
		return fmt.Errorf("supabase %s: decode: %w", table, err)
	}
	if len(rows) == 0 {
		return fmt.Errorf("%s row not found", table)
	}
	return json.Unmarshal(rows[0], out)
}

// CreateLead inserts a lead and returns the stored row.
func (s *SupabaseStore) CreateLead(l *Lead) (*Lead, error) {
	var out Lead
	return &out, s.insert("leads", l, &out)
}

// GetLead returns the lead with the given id.
func (s *SupabaseStore) GetLead(id string) (*Lead, error) {
	var out Lead
	return &out, s.getOne("leads", "id", id, &out)
}

// UpdateLead applies the field updates to a lead and returns the result.
func (s *SupabaseStore) UpdateLead(id string, fields map[string]any) (*Lead, error) {
	var out Lead
	return &out, s.patch("leads", "id", id, fields, &out)
}

// CreateContact inserts a contact and returns the stored row.
func (s *SupabaseStore) CreateContact(c *Contact) (*Contact, error) {
	var out Contact
	return &out, s.insert("contacts", c, &out)
}

// ListContacts returns the contacts belonging to the given lead.
func (s *SupabaseStore) ListContacts(leadID string) ([]*Contact, error) {
	var out []*Contact
	return out, s.list("contacts", "lead_id", leadID, "", &out)
}

// CreateConversation inserts a conversation and returns the stored row.
func (s *SupabaseStore) CreateConversation(c *Conversation) (*Conversation, error) {
	var out Conversation
	return &out, s.insert("conversations", c, &out)
}

// GetConversation returns the conversation with the given id.
func (s *SupabaseStore) GetConversation(id string) (*Conversation, error) {
	var out Conversation
	return &out, s.getOne("conversations", "id", id, &out)
}

// UpdateConversation applies the field updates to a conversation.
func (s *SupabaseStore) UpdateConversation(id string, fields map[string]any) (*Conversation, error) {
	var out Conversation
	return &out, s.patch("conversations", "id", id, fields, &out)
}

// AddMessage inserts a message and returns the stored row.
func (s *SupabaseStore) AddMessage(m *Message) (*Message, error) {
	var out Message
	return &out, s.insert("messages", m, &out)
}

// ListMessages returns the messages in the given conversation, oldest first.
func (s *SupabaseStore) ListMessages(conversationID string) ([]*Message, error) {
	var out []*Message
	return out, s.list("messages", "conversation_id", conversationID, "created_at.asc", &out)
}

// AddActivity inserts an activity and returns the stored row.
func (s *SupabaseStore) AddActivity(a *Activity) (*Activity, error) {
	var out Activity
	return &out, s.insert("activities", a, &out)
}

// ListActivities returns the activities in the given conversation, oldest first.
func (s *SupabaseStore) ListActivities(conversationID string) ([]*Activity, error) {
	var out []*Activity
	return out, s.list("activities", "conversation_id", conversationID, "created_at.asc", &out)
}

// CreateDeal inserts a deal and returns the stored row.
func (s *SupabaseStore) CreateDeal(d *Deal) (*Deal, error) {
	var out Deal
	return &out, s.insert("deals", d, &out)
}

// UpdateDeal applies the field updates to a deal and returns the result.
func (s *SupabaseStore) UpdateDeal(id string, fields map[string]any) (*Deal, error) {
	var out Deal
	return &out, s.patch("deals", "id", id, fields, &out)
}

// CreateHandoff claims a handoff by inserting its unique external_id, returning
// ErrHandoffExists when the row already exists (Postgres unique violation 23505).
func (s *SupabaseStore) CreateHandoff(h *Handoff) (*Handoff, error) {
	var out Handoff
	if err := s.insert("handoffs", h, &out); err != nil {
		if strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate key") {
			return nil, fmt.Errorf("%w: %s", ErrHandoffExists, h.ExternalID)
		}
		return nil, err
	}
	return &out, nil
}

// GetHandoff returns the handoff with the given external id.
func (s *SupabaseStore) GetHandoff(externalID string) (*Handoff, error) {
	var out Handoff
	return &out, s.getOne("handoffs", "external_id", externalID, &out)
}

// UpdateHandoff applies the field updates to a handoff (keyed by external_id).
func (s *SupabaseStore) UpdateHandoff(externalID string, fields map[string]any) (*Handoff, error) {
	var out Handoff
	return &out, s.patch("handoffs", "external_id", externalID, fields, &out)
}
