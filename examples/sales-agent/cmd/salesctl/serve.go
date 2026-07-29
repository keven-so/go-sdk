// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/crm"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/intake"
)

// handoffPath is where CustomAIze POSTs lead handoffs.
const handoffPath = "/webhooks/handoff"

// newStore returns a Supabase-backed store when SUPABASE_URL and
// SUPABASE_SERVICE_ROLE_KEY are set (namespaced by the optional
// SUPABASE_TABLE_PREFIX), otherwise the in-memory store. The second return value
// names the backend for logging.
func newStore() (crm.Store, string) {
	url, key := os.Getenv("SUPABASE_URL"), os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	if url != "" && key != "" {
		s, err := crm.NewSupabaseStore(url, key, os.Getenv("SUPABASE_TABLE_PREFIX"))
		if err == nil {
			return s, "supabase"
		}
		log.Printf("intake: falling back to in-memory store: %v", err)
	}
	return crm.NewMemoryStore(), "memory"
}

// runServe starts the HTTP server that accepts CustomAIze handoffs. It reads the
// signing secret from WEBHOOK_SIGNING_SECRET, selects the store backend from the
// environment, and listens on addr.
func runServe(addr string) error {
	store, backend := newStore()
	proc := intake.NewProcessor(store)
	handler := intake.NewHandler(proc, os.Getenv("WEBHOOK_SIGNING_SECRET"))

	mux := http.NewServeMux()
	mux.Handle(handoffPath, handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	fmt.Printf("salesctl serve: listening on %s (store: %s)\n  handoff: %s\n", addr, backend, intake.Describe(handoffPath))
	if os.Getenv("WEBHOOK_SIGNING_SECRET") == "" {
		fmt.Println("  warning: WEBHOOK_SIGNING_SECRET unset — signature checks disabled (dev only)")
	}
	return http.ListenAndServe(addr, mux)
}
