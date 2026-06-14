// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package intake

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// SignatureHeader carries the hex-encoded HMAC-SHA256 of the raw request body,
// prefixed with "sha256=". CustomAIze computes it with the shared signing secret.
const SignatureHeader = "X-CustomAIze-Signature"

// maxBodyBytes caps the request body to avoid unbounded reads.
const maxBodyBytes = 1 << 20 // 1 MiB

// Handler is the HTTP entry point for CustomAIze handoffs. Mount it at
// /webhooks/handoff. It verifies the body signature, decodes a HandoffPayload,
// and applies it via the Processor.
type Handler struct {
	proc   *Processor
	secret []byte
}

// NewHandler returns a handler that applies handoffs through proc. If secret is
// non-empty, requests must carry a valid SignatureHeader; if empty, signature
// verification is skipped (dev only) and a warning is logged on first use.
func NewHandler(proc *Processor, secret string) *Handler {
	return &Handler{proc: proc, secret: []byte(secret)}
}

// ServeHTTP handles POST /webhooks/handoff.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "body too large")
		return
	}

	if len(h.secret) == 0 {
		log.Printf("intake: WEBHOOK_SIGNING_SECRET is empty; skipping signature verification (dev only)")
	} else if !validSignature(h.secret, body, r.Header.Get(SignatureHeader)) {
		writeError(w, http.StatusUnauthorized, "invalid signature")
		return
	}

	var payload HandoffPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	res, err := h.proc.Process(&payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	status := http.StatusCreated
	if res.Duplicate {
		status = http.StatusOK // idempotent replay
	}
	writeJSON(w, status, res)
}

// Sign returns the SignatureHeader value for body under secret. Callers (and
// tests) use it to produce the header CustomAIze would send.
func Sign(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// validSignature reports whether got matches the expected HMAC for body. The
// comparison is constant-time to avoid leaking the signature via timing.
func validSignature(secret, body []byte, got string) bool {
	got = strings.TrimSpace(got)
	if got == "" {
		return false
	}
	want := Sign(secret, body)
	return hmac.Equal([]byte(want), []byte(got))
}

// writeJSON writes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON {"error": msg} body with the given status code.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// Describe returns a one-line summary of the handoff endpoint for logs/usage.
func Describe(path string) string {
	return fmt.Sprintf("POST %s  (header %s: sha256=<hmac>)", path, SignatureHeader)
}
