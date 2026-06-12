// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package scoring implements a transparent, rules-based lead score. It is
// intentionally simple and dependency-free so it is easy to test and reason
// about; a later phase can swap in an ML/LLM scorer behind the same Score func.
package scoring

import "github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/crm"

// Result is a lead score and its explanation.
type Result struct {
	Score   int      `json:"score"` // 0..100
	Tier    string   `json:"tier"`  // A | B | C
	Factors []string `json:"factors"`
}

// Score combines a firmographic "fit" signal with a behavioral "intent" signal
// derived from logged messages/activities on the conversation.
func Score(lead *crm.Lead, contacts []*crm.Contact, messages []*crm.Message) Result {
	var score int
	var factors []string

	// Fit: known domain + a senior contact title.
	if lead.Domain != "" {
		score += 10
		factors = append(factors, "+10 known company domain")
	}
	for _, c := range contacts {
		if isSenior(c.Title) {
			score += 25
			factors = append(factors, "+25 senior decision-maker contact")
			break
		}
	}

	// Intent: inbound replies and engagement recency.
	var inbound int
	for _, m := range messages {
		if m.Direction == "in" {
			inbound++
		}
	}
	switch {
	case inbound >= 2:
		score += 40
		factors = append(factors, "+40 multiple inbound replies")
	case inbound == 1:
		score += 25
		factors = append(factors, "+25 replied at least once")
	default:
		factors = append(factors, "+0 no replies yet")
	}

	// Source weighting: inbound leads are hotter than cold outbound.
	switch lead.Source {
	case "inbound":
		score += 15
		factors = append(factors, "+15 inbound source")
	case "marketing":
		score += 10
		factors = append(factors, "+10 marketing-qualified source")
	}

	if score > 100 {
		score = 100
	}
	return Result{Score: score, Tier: tier(score), Factors: factors}
}

func tier(score int) string {
	switch {
	case score >= 70:
		return "A"
	case score >= 40:
		return "B"
	default:
		return "C"
	}
}

func isSenior(title string) bool {
	t := lower(title)
	for _, kw := range []string{"chief", "cxo", "ceo", "cfo", "cto", "coo", "vp", "vice president", "head", "director", "founder", "owner", "president"} {
		if contains(t, kw) {
			return true
		}
	}
	return false
}

// small dependency-free string helpers (avoid importing strings for two uses)
func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
