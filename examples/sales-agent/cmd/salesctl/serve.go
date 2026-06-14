// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/crm"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/intake"
)

// handoffPath is where CustomAIze POSTs lead handoffs.
const handoffPath = "/webhooks/handoff"

// runServe starts the HTTP server that accepts CustomAIze handoffs. It backs the
// intake with an in-memory store (a Supabase-backed store satisfying crm.Store
// will replace it once migrations are applied), reads the signing secret from
// WEBHOOK_SIGNING_SECRET, and listens on addr.
func runServe(addr string) error {
	store := crm.NewMemoryStore()
	proc := intake.NewProcessor(store)
	handler := intake.NewHandler(proc, os.Getenv("WEBHOOK_SIGNING_SECRET"))

	mux := http.NewServeMux()
	mux.Handle(handoffPath, handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	fmt.Printf("salesctl serve: listening on %s\n  handoff: %s\n", addr, intake.Describe(handoffPath))
	if os.Getenv("WEBHOOK_SIGNING_SECRET") == "" {
		fmt.Println("  warning: WEBHOOK_SIGNING_SECRET unset — signature checks disabled (dev only)")
	}
	return http.ListenAndServe(addr, mux)
}
