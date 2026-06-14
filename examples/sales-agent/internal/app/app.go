// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package app wires the CRM store and MCP tool servers into the two-tier tool
// topology the team uses. Everything runs in one process over in-memory
// transports: the agent loops (MCP clients) talk to the tool servers (MCP
// servers) with no subprocess or socket. Swapping to NewCommandTransport /
// Streamable HTTP later changes only this file.
//
// Two tiers break the agent-as-tool cycle:
//   - inner bridge: crm + intel — the tools support sub-agents may use.
//   - outer bridge: comms + crm + schedule + intel + team + orchestrator — the
//     tools customer-facing agents and the supervisor use.
package app

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/agent"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/crm"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/llm"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/mcpservers/comms"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/mcpservers/crmserver"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/mcpservers/intel"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/mcpservers/orchestrator"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/mcpservers/schedule"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/mcpservers/team"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tools holds the outer ToolBridge (for customer-facing agents and the
// supervisor) and a Close func that tears down all in-process sessions.
type Tools struct {
	Bridge *agent.ToolBridge
	Close  func()
}

// connector connects MCP servers to in-process clients and tracks teardown.
type connector struct {
	ctx      context.Context
	sessions map[string]*mcp.ClientSession
	closers  []func() error
}

// add connects an MCP server to an in-process client session under the given
// name and registers both sides for teardown.
func (c *connector) add(name string, srv *mcp.Server) error {
	clientT, serverT := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(c.ctx, serverT)
	if err != nil {
		return fmt.Errorf("connect server %q: %w", name, err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "sales-agent", Version: "0.1.0"}, nil)
	cs, err := client.Connect(c.ctx, clientT)
	if err != nil {
		_ = ss.Close()
		return fmt.Errorf("connect client %q: %w", name, err)
	}
	c.sessions[name] = cs
	c.closers = append(c.closers, cs.Close, ss.Close)
	return nil
}

// bridge builds a ToolBridge over the subset of connected sessions named.
func (c *connector) bridge(names ...string) (*agent.ToolBridge, error) {
	subset := make(map[string]*mcp.ClientSession, len(names))
	for _, n := range names {
		subset[n] = c.sessions[n]
	}
	return agent.NewToolBridge(c.ctx, subset)
}

// cleanup closes every session opened by the connector, ignoring errors.
func (c *connector) cleanup() {
	for _, fn := range c.closers {
		_ = fn()
	}
}

// BuildTools constructs the tool servers and returns the outer ToolBridge.
// dryRun selects no-op comms providers and a deterministic LLM for support
// sub-agents; live is the LLM used for support sub-agents in live mode.
func BuildTools(ctx context.Context, store crm.Store, dryRun bool, live llm.LLM) (*Tools, error) {
	c := &connector{ctx: ctx, sessions: map[string]*mcp.ClientSession{}}

	// Tier 1: base tool servers.
	for name, srv := range map[string]*mcp.Server{
		"comms":    comms.New(dryRun),
		"crm":      crmserver.New(store),
		"schedule": schedule.New(dryRun),
		"intel":    intel.New(store),
	} {
		if err := c.add(name, srv); err != nil {
			c.cleanup()
			return nil, err
		}
	}

	// Inner bridge: the tools support sub-agents may call.
	inner, err := c.bridge("crm", "intel")
	if err != nil {
		c.cleanup()
		return nil, err
	}

	// Tier 2: higher-order servers that depend on the inner bridge / store.
	if err := c.add("team", team.New(dryRun, live, inner)); err != nil {
		c.cleanup()
		return nil, err
	}
	if err := c.add("orchestrator", orchestrator.New(store)); err != nil {
		c.cleanup()
		return nil, err
	}

	// Outer bridge: everything customer-facing agents and the supervisor use.
	outer, err := c.bridge("comms", "crm", "schedule", "intel", "team", "orchestrator")
	if err != nil {
		c.cleanup()
		return nil, err
	}
	return &Tools{Bridge: outer, Close: c.cleanup}, nil
}
