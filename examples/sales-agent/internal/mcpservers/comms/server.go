// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package comms is the side-effecting MCP tool server: email, SMS, and voice.
// In Phase 0 it ships only dry-run providers that log and return synthetic IDs,
// so the whole pipeline runs end-to-end without sending anything real. Live
// Gmail/Twilio/voice providers will implement the same Sender/Caller interfaces.
package comms

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Sender abstracts email/SMS delivery.
type Sender interface {
	SendEmail(ctx context.Context, to, subject, body string) (externalID string, err error)
	SendSMS(ctx context.Context, to, body string) (externalID string, err error)
}

// Caller abstracts placing an outbound phone call via a voice platform.
type Caller interface {
	PlaceCall(ctx context.Context, to, objective string) (callID string, err error)
}

// New returns the comms MCP server. When dryRun is true it uses no-op providers.
func New(dryRun bool) *mcp.Server {
	var sender Sender = dryRunProvider{}
	var caller Caller = dryRunProvider{}
	if !dryRun {
		// TODO(phase 2-4): wire Gmail (email), Twilio (SMS), and a voice
		// platform (Vapi/Bland/Retell) here. They satisfy Sender/Caller.
		panic("comms: live providers not yet configured; run with dry-run")
	}
	return newWith(sender, caller)
}

// newWith builds the comms MCP server over the given sender and caller providers.
func newWith(sender Sender, caller Caller) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "comms", Version: "0.1.0"}, nil)
	h := &handlers{sender: sender, caller: caller}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "send_email",
		Description: "Send an email to a contact within a conversation. Returns the message id.",
	}, h.sendEmail)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "send_sms",
		Description: "Send an SMS to a contact. Only use when the contact has consented to SMS.",
	}, h.sendSMS)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "place_call",
		Description: "Place an outbound phone call handled by the AI voice platform. Returns a call id; the transcript arrives asynchronously.",
	}, h.placeCall)
	return s
}

// handlers holds the comms providers shared by the tool handlers.
type handlers struct {
	sender Sender
	caller Caller
}

// --- send_email ---

// SendEmailIn is the input payload for the send_email tool.
type SendEmailIn struct {
	ConversationID string `json:"conversation_id" jsonschema:"the conversation this message belongs to"`
	To             string `json:"to" jsonschema:"recipient email address"`
	Subject        string `json:"subject" jsonschema:"email subject line"`
	BodyHTML       string `json:"body_html" jsonschema:"email body (HTML or plain text)"`
}

// SendResult is the output payload for the send_email and send_sms tools.
type SendResult struct {
	ExternalID string `json:"external_id"`
	Status     string `json:"status"`
}

// sendEmail implements the send_email tool: it sends an email and returns its id.
func (h *handlers) sendEmail(ctx context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[SendEmailIn]) (*mcp.CallToolResultFor[SendResult], error) {
	in := p.Arguments
	id, err := h.sender.SendEmail(ctx, in.To, in.Subject, in.BodyHTML)
	if err != nil {
		return errResult[SendResult](fmt.Sprintf("send_email failed: %v", err)), nil
	}
	out := SendResult{ExternalID: id, Status: "sent"}
	return okResult(fmt.Sprintf("Email sent to %s (subject %q), id=%s", in.To, in.Subject, id), out), nil
}

// --- send_sms ---

// SendSMSIn is the input payload for the send_sms tool.
type SendSMSIn struct {
	ConversationID string `json:"conversation_id" jsonschema:"the conversation this message belongs to"`
	To             string `json:"to" jsonschema:"recipient phone number in E.164 format"`
	Body           string `json:"body" jsonschema:"SMS text body"`
}

// sendSMS implements the send_sms tool: it sends an SMS and returns its id.
func (h *handlers) sendSMS(ctx context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[SendSMSIn]) (*mcp.CallToolResultFor[SendResult], error) {
	in := p.Arguments
	id, err := h.sender.SendSMS(ctx, in.To, in.Body)
	if err != nil {
		return errResult[SendResult](fmt.Sprintf("send_sms failed: %v", err)), nil
	}
	out := SendResult{ExternalID: id, Status: "sent"}
	return okResult(fmt.Sprintf("SMS sent to %s, id=%s", in.To, id), out), nil
}

// --- place_call ---

// PlaceCallIn is the input payload for the place_call tool.
type PlaceCallIn struct {
	ConversationID string `json:"conversation_id" jsonschema:"the conversation this call belongs to"`
	To             string `json:"to" jsonschema:"phone number to call in E.164 format"`
	Objective      string `json:"objective" jsonschema:"what the call should accomplish"`
	ScriptHint     string `json:"script_hint,omitempty" jsonschema:"optional talking points for the voice agent"`
}

// PlaceCallResult is the output payload for the place_call tool.
type PlaceCallResult struct {
	CallID string `json:"call_id"`
	Status string `json:"status"`
}

// placeCall implements the place_call tool: it starts an outbound call and returns its id.
func (h *handlers) placeCall(ctx context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[PlaceCallIn]) (*mcp.CallToolResultFor[PlaceCallResult], error) {
	in := p.Arguments
	id, err := h.caller.PlaceCall(ctx, in.To, in.Objective)
	if err != nil {
		return errResult[PlaceCallResult](fmt.Sprintf("place_call failed: %v", err)), nil
	}
	out := PlaceCallResult{CallID: id, Status: "dialing"}
	return okResult(fmt.Sprintf("Call to %s started (objective: %s), call_id=%s; transcript will arrive via webhook", in.To, in.Objective, id), out), nil
}

// okResult builds a successful tool result with text and structured content.
func okResult[T any](text string, out T) *mcp.CallToolResultFor[T] {
	return &mcp.CallToolResultFor[T]{
		Content:           []mcp.Content{&mcp.TextContent{Text: text}},
		StructuredContent: out,
	}
}

// errResult builds a tool result flagged as an error with the given message.
func errResult[T any](text string) *mcp.CallToolResultFor[T] {
	return &mcp.CallToolResultFor[T]{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: true,
	}
}
