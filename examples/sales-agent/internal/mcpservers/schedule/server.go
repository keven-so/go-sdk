// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package schedule exposes meeting-booking tools. Phase 0 returns synthetic
// availability and a fake meeting link; a later phase backs these with Google
// Calendar behind the same tool shapes.
package schedule

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// New returns the schedule MCP server.
func New(dryRun bool) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "schedule", Version: "0.1.0"}, nil)
	h := &handlers{dryRun: dryRun}
	mcp.AddTool(s, &mcp.Tool{Name: "list_availability", Description: "List candidate meeting slots over a window."}, h.listAvailability)
	mcp.AddTool(s, &mcp.Tool{Name: "book_meeting", Description: "Book a meeting and return a calendar event id and join link."}, h.bookMeeting)
	return s
}

// handlers holds the schedule server's configuration shared by tool handlers.
type handlers struct {
	dryRun bool
}

// ListAvailabilityIn is the input payload for the list_availability tool.
type ListAvailabilityIn struct {
	DurationMin int `json:"duration_min" jsonschema:"meeting length in minutes"`
	Days        int `json:"days,omitempty" jsonschema:"how many days ahead to search (default 5)"`
}

// Slot is a single candidate meeting time range.
type Slot struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// ListAvailabilityOut is the output payload for the list_availability tool.
type ListAvailabilityOut struct {
	Slots []Slot `json:"slots"`
}

// listAvailability implements the list_availability tool: it returns candidate meeting slots.
func (h *handlers) listAvailability(_ context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[ListAvailabilityIn]) (*mcp.CallToolResultFor[ListAvailabilityOut], error) {
	dur := p.Arguments.DurationMin
	if dur <= 0 {
		dur = 30
	}
	days := p.Arguments.Days
	if days <= 0 {
		days = 5
	}
	// Offer 10:00 and 14:00 local-ish slots for the next `days` weekdays.
	var slots []Slot
	day := time.Now().UTC().Truncate(24 * time.Hour)
	for i := 1; len(slots) < days*2 && i <= days*2; i++ {
		d := day.AddDate(0, 0, i)
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		for _, hr := range []int{10, 14} {
			start := time.Date(d.Year(), d.Month(), d.Day(), hr, 0, 0, 0, time.UTC)
			slots = append(slots, Slot{
				Start: start.Format(time.RFC3339),
				End:   start.Add(time.Duration(dur) * time.Minute).Format(time.RFC3339),
			})
		}
	}
	return okResult(fmt.Sprintf("%d slots available", len(slots)), ListAvailabilityOut{Slots: slots}), nil
}

// BookMeetingIn is the input payload for the book_meeting tool.
type BookMeetingIn struct {
	ConversationID string `json:"conversation_id" jsonschema:"the conversation id"`
	ContactEmail   string `json:"contact_email" jsonschema:"attendee email"`
	Start          string `json:"start" jsonschema:"RFC3339 start time"`
	DurationMin    int    `json:"duration_min" jsonschema:"meeting length in minutes"`
	Title          string `json:"title" jsonschema:"meeting title"`
}

// BookMeetingOut is the output payload for the book_meeting tool.
type BookMeetingOut struct {
	EventID  string `json:"event_id"`
	JoinLink string `json:"join_link"`
	Start    string `json:"start"`
}

// bookMeeting implements the book_meeting tool: it books a meeting and returns its id and join link.
func (h *handlers) bookMeeting(_ context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[BookMeetingIn]) (*mcp.CallToolResultFor[BookMeetingOut], error) {
	in := p.Arguments
	id := fmt.Sprintf("evt_%d", time.Now().UnixNano())
	out := BookMeetingOut{
		EventID:  id,
		JoinLink: "https://meet.example.com/" + id,
		Start:    in.Start,
	}
	return okResult(fmt.Sprintf("Booked %q with %s at %s, event=%s", in.Title, in.ContactEmail, in.Start, id), out), nil
}

// okResult builds a successful tool result with text and structured content.
func okResult[T any](text string, out T) *mcp.CallToolResultFor[T] {
	return &mcp.CallToolResultFor[T]{
		Content:           []mcp.Content{&mcp.TextContent{Text: text}},
		StructuredContent: out,
	}
}
