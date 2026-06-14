// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent_test

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/agent"
)

func TestSupervisorRoute(t *testing.T) {
	cases := []struct {
		event  string
		status string
		want   string
	}{
		{agent.EventHandoff, "new", "sdr"},
		{agent.EventCadenceTouch, "working", "sdr"},
		{agent.EventInboundReply, "engaged", "inbound"},
		{agent.EventInboundCall, "engaged", "inbound"},
		{agent.EventTranscript, "working", "sdr"},
		{agent.EventInboundReply, "meeting", "closer"}, // stage override wins
		{agent.EventHandoff, "won", "closer"},
	}
	for _, c := range cases {
		if got := agent.Route(c.event, c.status); got != c.want {
			t.Errorf("Route(%q,%q)=%q, want %q", c.event, c.status, got, c.want)
		}
	}
}
