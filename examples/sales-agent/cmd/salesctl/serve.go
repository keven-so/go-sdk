// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/crm"
	"github.com/modelcontextprotocol/go-sdk/examples/sales-agent/internal/intake"
)

// handoffPath is where CustomAIze POSTs lead handoffs.
const handoffPath = "/webhooks/handoff"

// newStore returns a Supabase-backed store when SUPABASE_URL and
// SUPABASE_SERVICE_ROLE_KEY are set (namespaced by the optional
// SUPABASE_TABLE_PREFIX), otherwise the in-memory store. When Supabase is
// configured but initialization fails it returns the error rather than silently
// falling back, so handoffs are never accepted into a store that will be lost on
// restart. The second return value names the backend for logging.
func newStore() (crm.Store, string, error) {
	url, key := os.Getenv("SUPABASE_URL"), os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	if url != "" && key != "" {
		s, err := crm.NewSupabaseStore(url, key, os.Getenv("SUPABASE_TABLE_PREFIX"))
		if err != nil {
			return nil, "", fmt.Errorf("supabase store: %w", err)
		}
		return s, "supabase", nil
	}
	return crm.NewMemoryStore(), "memory", nil
}

// runServe starts the HTTP server that accepts CustomAIze handoffs. It selects
// the store backend from the environment and requires WEBHOOK_SIGNING_SECRET so
// intake is signed by default; pass insecure=true (dev only) to accept unsigned
// requests when the secret is unset.
func runServe(addr string, insecure bool) error {
	store, backend, err := newStore()
	if err != nil {
		return err
	}

	secret := os.Getenv("WEBHOOK_SIGNING_SECRET")
	if secret == "" && !insecure {
		return fmt.Errorf("WEBHOOK_SIGNING_SECRET is required; pass -insecure to accept unsigned requests (dev only)")
	}

	proc := intake.NewProcessor(store)
	handler := intake.NewHandler(proc, secret)

	mux := http.NewServeMux()
	mux.Handle(handoffPath, handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	fmt.Printf("salesctl serve: listening on %s (store: %s)\n  handoff: %s\n", addr, backend, intake.Describe(handoffPath))
	if secret == "" {
		fmt.Println("  warning: WEBHOOK_SIGNING_SECRET unset — signature checks disabled (-insecure, dev only)")
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return srv.ListenAndServe()
}
