// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent_test

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/agent"
)

// TestRegistryWellFormed checks the role registry and stable order are internally consistent.
func TestRegistryWellFormed(t *testing.T) {
	if len(agent.Order) != len(agent.Registry) {
		t.Fatalf("Order (%d) and Registry (%d) sizes differ", len(agent.Order), len(agent.Registry))
	}
	for _, name := range agent.Order {
		r, ok := agent.Get(name)
		if !ok {
			t.Fatalf("Order references missing role %q", name)
		}
		if r.Name == "" || r.Title == "" || r.Kind == "" || r.SystemPrompt == "" {
			t.Errorf("role %q missing required metadata: %+v", name, r)
		}
		switch r.Kind {
		case agent.KindCustomerFacing, agent.KindSupport, agent.KindSupervisor:
		default:
			t.Errorf("role %q has invalid kind %q", name, r.Kind)
		}
		// Handoff targets must reference real customer-facing roles.
		for _, tgt := range r.HandoffTargets {
			ht, ok := agent.Get(tgt)
			if !ok || ht.Kind != agent.KindCustomerFacing {
				t.Errorf("role %q hands off to invalid target %q", name, tgt)
			}
		}
	}
}

// TestExpectedRolesPresent asserts every expected sales role is registered.
func TestExpectedRolesPresent(t *testing.T) {
	want := []string{"supervisor", "sdr", "inbound", "voice", "closer", "researcher", "copywriter", "revops", "coach"}
	for _, name := range want {
		if _, ok := agent.Get(name); !ok {
			t.Errorf("expected role %q in registry", name)
		}
	}
}
