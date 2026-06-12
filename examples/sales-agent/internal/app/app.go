// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package app wires the CRM store and MCP tool servers to an agent ToolBridge.
// In Phase 0 everything runs in a single process: the agent loop (MCP client)
// talks to the tool servers (MCP servers) over in-memory transports, so there
// is no subprocess or socket. Swapping to NewCommandTransport / Streamable HTTP
// later changes only this file.
package app

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/agent"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/crm"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/mcpservers/comms"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/mcpservers/crmserver"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/mcpservers/intel"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/mcpservers/schedule"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tools holds a connected ToolBridge and a Close func that tears down all the
// in-memory client/server sessions.
type Tools struct {
	Bridge *agent.ToolBridge
	Close  func()
}

// BuildTools constructs the four MCP tool servers, connects an MCP client to
// each over an in-memory transport, and returns a ToolBridge over all of them.
func BuildTools(ctx context.Context, store crm.Store, dryRun bool) (*Tools, error) {
	servers := map[string]*mcp.Server{
		"comms":    comms.New(dryRun),
		"crm":      crmserver.New(store),
		"schedule": schedule.New(dryRun),
		"intel":    intel.New(store),
	}

	sessions := make(map[string]*mcp.ClientSession, len(servers))
	var closers []func() error
	cleanup := func() {
		for _, c := range closers {
			_ = c()
		}
	}

	for name, srv := range servers {
		clientT, serverT := mcp.NewInMemoryTransports()
		ss, err := srv.Connect(ctx, serverT)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("connect server %q: %w", name, err)
		}
		client := mcp.NewClient(&mcp.Implementation{Name: "sales-agent", Version: "0.1.0"}, nil)
		cs, err := client.Connect(ctx, clientT)
		if err != nil {
			_ = ss.Close()
			cleanup()
			return nil, fmt.Errorf("connect client %q: %w", name, err)
		}
		sessions[name] = cs
		closers = append(closers, cs.Close, ss.Close)
	}

	bridge, err := agent.NewToolBridge(ctx, sessions)
	if err != nil {
		cleanup()
		return nil, err
	}
	return &Tools{Bridge: bridge, Close: cleanup}, nil
}
