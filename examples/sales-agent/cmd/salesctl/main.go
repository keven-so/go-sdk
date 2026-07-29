// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Command salesctl drives the multi-agent sales system. Its "dry-run" subcommand
// seeds a CustomAIze lead handoff and runs the Supervisor → SDR flow end-to-end
// against the in-process MCP tool servers, sending nothing real. "serve" runs the
// CustomAIze handoff webhook. "roles" prints the agent roster.
//
// Usage:
//
//	go run ./cmd/salesctl roles               # print the agent roster
//	go run ./cmd/salesctl dry-run             # scripted fake LLM, no API key
//	go run ./cmd/salesctl dry-run -live       # real Claude (needs ANTHROPIC_API_KEY)
//	go run ./cmd/salesctl serve               # run the CustomAIze handoff webhook
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

// main dispatches the salesctl subcommand (roles, dry-run, or serve).
func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "roles":
		runRoles()
	case "dry-run":
		fs := flag.NewFlagSet("dry-run", flag.ExitOnError)
		live := fs.Bool("live", false, "use the real Claude API (requires ANTHROPIC_API_KEY)")
		_ = fs.Parse(os.Args[2:])
		if err := runDryRun(*live); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		addr := fs.String("addr", "", "listen address (defaults to $HTTP_ADDR or :8080)")
		insecure := fs.Bool("insecure", false, "accept unsigned requests when WEBHOOK_SIGNING_SECRET is unset (dev only)")
		_ = fs.Parse(os.Args[2:])
		listen := *addr
		if listen == "" {
			if listen = os.Getenv("HTTP_ADDR"); listen == "" {
				listen = ":8080"
			}
		}
		if err := runServe(listen, *insecure); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	default:
		usage()
	}
}

// usage prints the command synopsis and exits with a non-zero status.
func usage() {
	fmt.Fprintln(os.Stderr, "usage: salesctl <roles | dry-run [-live] | serve [-addr :8080] [-insecure]>")
	os.Exit(2)
}

// runRoles prints the agent roster in stable display order.
func runRoles() {
	fmt.Println("Agent roster:")
	for _, name := range agent.Order {
		r := agent.Registry[name]
		fmt.Printf("\n• %s (%s) [%s]\n  %s\n  tools: %v\n", r.Title, r.Name, r.Kind, r.Mission, r.AllowedTools)
		if len(r.HandoffTargets) > 0 {
			fmt.Printf("  hands off to: %v\n", r.HandoffTargets)
		}
		if len(r.KPIs) > 0 {
			fmt.Printf("  KPIs: %v\n", r.KPIs)
		}
	}
}

// runDryRun seeds a CustomAIze handoff and runs the Supervisor → SDR flow
// end-to-end against the in-process tool servers, sending nothing real. When
// live is true it uses the real Claude API instead of the scripted fake LLM.
func runDryRun(live bool) error {
	ctx := context.Background()
	store := crm.NewMemoryStore()

	// 1) Simulate the CustomAIze → sales handoff: lead, contact, conversation,
	//    and the first (marketing) email already in the thread.
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
		CurrentRole: "", Status: "engaged", GmailThreadID: "thread_customaize_001",
	})
	_, _ = store.AddMessage(&crm.Message{
		ConversationID: conv.ID, Direction: "out", Channel: "email",
		ExternalID: "customaize_email_001", Subject: "Cut ops busywork at Northwind",
		Body: "Hi Dana — saw Northwind is scaling ops. We help teams automate the busywork. Worth a quick look?",
	})

	fmt.Printf("=== CustomAIze handoff ===\nlead=%s company=%q contact=%q (%s)\nconversation=%s thread=%s\n\n",
		lead.ID, lead.Company, contact.Name, contact.Email, conv.ID, conv.GmailThreadID)

	// 2) Supervisor routes the event (rules-first) to a customer-facing role.
	routed := agent.Route(agent.EventHandoff, lead.Status)
	fmt.Printf("=== Supervisor ===\nevent=%s lead_status=%s → routed to %q\n\n", agent.EventHandoff, lead.Status, routed)
	role, _ := agent.Get(routed)

	// 3) Build the two-tier in-process tool topology (dry-run providers).
	var brain llm.LLM
	if live {
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return fmt.Errorf("-live requires ANTHROPIC_API_KEY")
		}
		brain = llm.NewAnthropic(key, os.Getenv("ANTHROPIC_MODEL"), 1024)
	}
	tools, err := app.BuildTools(ctx, store, !live, brain)
	if err != nil {
		return err
	}
	defer tools.Close()

	if live {
		fmt.Println("=== Running with live Claude ===")
	} else {
		brain = sdrScript(lead.ID, conv.ID, contact.Email, contact.Phone)
		fmt.Println("=== Running with scripted fake LLM (no API calls) ===")
	}

	// 4) Run the routed customer-facing agent, printing every step.
	loop := &agent.Loop{
		LLM:      brain,
		Bridge:   tools.Bridge,
		Model:    os.Getenv("ANTHROPIC_MODEL"),
		MaxTurns: 20,
		Observer: printEvent,
	}
	history := []llm.Message{{
		Role: "user",
		Content: []llm.Block{{Type: "text", Text: fmt.Sprintf(
			"New CustomAIze handoff. lead_id=%s conversation_id=%s contact_email=%s contact_phone=%s. "+
				"The first email is already sent. Take it from here.",
			lead.ID, conv.ID, contact.Email, contact.Phone)}},
	}}
	if _, err := loop.Run(ctx, role, history); err != nil {
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
	updated, _ := store.GetConversation(conv.ID)
	finalLead, _ := store.GetLead(lead.ID)
	fmt.Printf("\nconversation now owned by role=%q; lead qualification=%v\n", updated.CurrentRole, finalLead.Qualification)
	return nil
}

// sdrScript is the canned SDR conversation exercised in dry-run: load context,
// prioritize + research + draft via support agent-as-tools, send, capture BANT,
// then hand off to the closer — touching base, team, and orchestrator servers
// with zero external sends.
func sdrScript(leadID, convID, email, phone string) llm.LLM {
	return llm.NewFake(
		llm.AssistantToolUse("t1", "get_lead", map[string]any{"lead_id": leadID}),
		llm.AssistantToolUse("t2", "prioritize_lead", map[string]any{"lead_id": leadID}),
		llm.AssistantToolUse("t3", "research_account", map[string]any{"lead_id": leadID}),
		llm.AssistantToolUse("t4", "draft_message", map[string]any{
			"conversation_id": convID, "lead_id": leadID, "channel": "email",
			"intent": "book a 20-minute intro meeting",
		}),
		llm.AssistantToolUse("t5", "send_email", map[string]any{
			"conversation_id": convID, "to": email,
			"subject":   "Re: Cut ops busywork at Northwind",
			"body_html": "Hi Dana — following up on my note. Teams your size usually claw back ~8 hrs/week. Open to a 20-min look this week?",
		}),
		llm.AssistantToolUse("t6", "update_qualification", map[string]any{
			"conversation_id": convID, "framework": "bant",
			"fields": map[string]any{
				"budget": "~$50k confirmed", "authority": "VP Ops, decision-maker",
				"need": "manual ops busywork", "timeline": "this quarter",
			},
		}),
		llm.AssistantToolUse("t7", "log_activity", map[string]any{
			"conversation_id": convID, "type": "note", "summary": "Sent follow-up; BANT confirmed",
		}),
		llm.AssistantToolUse("t8", "handoff", map[string]any{
			"conversation_id": convID, "role": "closer",
			"reason": "BANT satisfied — budget, authority, need, and timeline all confirmed",
		}),
		llm.AssistantText("Prioritized, researched, and emailed Dana; captured BANT and handed off to the closer to run MEDDICC and close."),
	)
}

// printEvent renders a loop event to stdout for the dry-run trace.
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
	}
}
