// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Command salesctl is the Phase 0 driver for the multi-agent sales system. Its
// "dry-run" subcommand seeds a marketing-handoff lead and runs the SDR agent
// end-to-end against the in-process MCP tool servers, sending nothing real.
//
// Usage:
//
//	go run ./cmd/salesctl dry-run            # scripted fake LLM, no API key
//	go run ./cmd/salesctl dry-run -live      # real Claude (needs ANTHROPIC_API_KEY)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/agent"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/app"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/crm"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/llm"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "dry-run" {
		fmt.Fprintln(os.Stderr, "usage: salesctl dry-run [-live]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("dry-run", flag.ExitOnError)
	live := fs.Bool("live", false, "use the real Claude API (requires ANTHROPIC_API_KEY)")
	_ = fs.Parse(os.Args[2:])

	if err := runDryRun(*live); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runDryRun(live bool) error {
	ctx := context.Background()
	store := crm.NewMemoryStore()

	// 1) Simulate the marketing -> sales handoff: a lead, a contact, a
	//    conversation, and the first (marketing) email already in the thread.
	lead, _ := store.CreateLead(&crm.Lead{
		Source: "marketing", Company: "Northwind Software", Domain: "northwind.tech",
		Status: "new", OwnerRole: "sdr",
	})
	contact, _ := store.CreateContact(&crm.Contact{
		LeadID: lead.ID, Name: "Dana Reyes", Email: "dana@northwind.tech",
		Phone: "+15551234567", Title: "VP of Operations", Timezone: "America/New_York",
		ConsentEmail: true, ConsentSMS: true,
	})
	conv, _ := store.CreateConversation(&crm.Conversation{
		LeadID: lead.ID, ContactID: contact.ID, ChannelPrimary: "email",
		CurrentRole: "sdr", Status: "engaged", GmailThreadID: "thread_marketing_001",
	})
	_, _ = store.AddMessage(&crm.Message{
		ConversationID: conv.ID, Direction: "out", Channel: "email",
		ExternalID: "mkt_email_001", Subject: "Cut ops busywork at Northwind",
		Body: "Hi Dana — saw Northwind is scaling ops. We help teams like yours automate the busywork. Worth a quick look?",
	})

	fmt.Printf("=== Handoff ===\nlead=%s company=%q contact=%q (%s)\nconversation=%s thread=%s\n\n",
		lead.ID, lead.Company, contact.Name, contact.Email, conv.ID, conv.GmailThreadID)

	// 2) Build the in-process MCP tool servers + bridge (dry-run providers).
	tools, err := app.BuildTools(ctx, store, true)
	if err != nil {
		return err
	}
	defer tools.Close()

	// 3) Choose the brain: scripted fake (default) or real Claude (-live).
	var brain llm.LLM
	if live {
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return fmt.Errorf("-live requires ANTHROPIC_API_KEY")
		}
		brain = llm.NewAnthropic(key, os.Getenv("ANTHROPIC_MODEL"), 1024)
		fmt.Println("=== Running with live Claude ===")
	} else {
		brain = sdrScript(lead.ID, conv.ID, contact.Email, contact.Phone)
		fmt.Println("=== Running with scripted fake LLM (no API calls) ===")
	}

	// 4) Run the SDR loop, printing every step.
	loop := &agent.Loop{
		LLM:      brain,
		Bridge:   tools.Bridge,
		Model:    os.Getenv("ANTHROPIC_MODEL"),
		MaxTurns: 16,
		Observer: printEvent,
	}
	history := []llm.Message{{
		Role: "user",
		Content: []llm.Block{{Type: "text", Text: fmt.Sprintf(
			"New marketing handoff. lead_id=%s conversation_id=%s contact_email=%s contact_phone=%s. "+
				"Marketing already sent the first email. Take it from here.",
			lead.ID, conv.ID, contact.Email, contact.Phone)}},
	}}
	if _, err := loop.Run(ctx, agent.SDR, history); err != nil {
		return err
	}

	// 5) Show the resulting CRM state.
	fmt.Println("\n=== CRM activity log ===")
	acts, _ := store.ListActivities(conv.ID)
	if len(acts) == 0 {
		fmt.Println("(none)")
	}
	for _, a := range acts {
		fmt.Printf("- [%s] %s\n", a.Type, a.Summary)
	}
	return nil
}

// sdrScript is the canned SDR conversation exercised in dry-run: load context,
// score, enrich, follow up by email, offer a call, and book a meeting — touching
// all four tool servers with zero external sends.
func sdrScript(leadID, convID, email, phone string) llm.LLM {
	return llm.NewFake(
		llm.AssistantToolUse("t1", "get_lead", map[string]any{"lead_id": leadID}),
		llm.AssistantToolUse("t2", "score_lead", map[string]any{"lead_id": leadID}),
		llm.AssistantToolUse("t3", "enrich_lead", map[string]any{"lead_id": leadID}),
		llm.AssistantToolUse("t4", "send_email", map[string]any{
			"conversation_id": convID, "to": email,
			"subject":   "Re: Cut ops busywork at Northwind",
			"body_html": "Hi Dana — following up on my note. Teams your size usually claw back ~8 hrs/week. Open to a 20-min look this week?",
		}),
		llm.AssistantToolUse("t5", "log_activity", map[string]any{
			"conversation_id": convID, "type": "note", "summary": "Sent personalized email follow-up #1",
		}),
		llm.AssistantToolUse("t6", "send_sms", map[string]any{
			"conversation_id": convID, "to": phone,
			"body": "Hi Dana, it's Sam from Acme — just emailed you re: saving your ops team time. Happy to find 20 min this week!",
		}),
		llm.AssistantToolUse("t7", "book_meeting", map[string]any{
			"conversation_id": convID, "contact_email": email,
			"start": "2026-06-16T14:00:00Z", "duration_min": 20, "title": "Northwind <> Acme intro",
		}),
		llm.AssistantText("Follow-up email + SMS sent and a meeting slot offered. Awaiting Dana's reply; will continue the cadence if no response."),
	)
}

func printEvent(e agent.Event) {
	switch e.Kind {
	case "assistant_text":
		fmt.Printf("[%s] 💬 %s\n", e.Role, e.Text)
	case "tool_call":
		fmt.Printf("[%s] 🔧 %s(%s)\n", e.Role, e.Tool, string(e.Input))
	case "tool_result":
		marker := "✅"
		if e.IsError {
			marker = "❌"
		}
		fmt.Printf("[%s]    %s %s\n", e.Role, marker, e.Output)
	case "done":
		// handled by trailing assistant_text
	}
}
