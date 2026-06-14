// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent

// Event kinds the supervisor routes. These mirror the queue job kinds in the
// fuller design (handoff, inbound_reply, cadence_touch, transcript).
const (
	EventHandoff      = "handoff"       // new lead from CustomAIze lead-gen
	EventInboundReply = "inbound_reply" // prospect replied (email/SMS)
	EventInboundCall  = "inbound_call"  // prospect called in
	EventCadenceTouch = "cadence_touch" // scheduled outbound step is due
	EventTranscript   = "transcript"    // a voice call finished
)

// Route is the rules-first supervisor: given an event kind and the lead's
// current status, it returns the customer-facing role that should own the next
// action. It is deterministic and explainable; an LLM supervisor (the Supervisor
// role) can be layered on for ambiguous cases.
func Route(eventKind, leadStatus string) string {
	// Stage overrides: a lead at/after a booked meeting belongs to the closer.
	switch leadStatus {
	case "meeting", "won":
		return Closer.Name
	}

	switch eventKind {
	case EventInboundReply, EventInboundCall:
		return Inbound.Name
	case EventTranscript:
		return SDR.Name // post-call follow-up by default
	case EventHandoff, EventCadenceTouch:
		return SDR.Name
	default:
		return SDR.Name
	}
}
