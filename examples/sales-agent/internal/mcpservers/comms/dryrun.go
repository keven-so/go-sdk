// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package comms

import (
	"context"
	"log"
	"sync/atomic"
)

// dryRunProvider implements Sender and Caller without contacting any external
// service. It logs the intended action and returns a synthetic id, so dry runs
// and tests exercise the full agent loop with zero real sends.
type dryRunProvider struct{}

// dryRunSeq is a process-wide counter for synthetic dry-run ids.
var dryRunSeq atomic.Int64

// synthID returns a unique synthetic id with the given prefix.
func synthID(prefix string) string {
	n := dryRunSeq.Add(1)
	return prefix + "_dryrun_" + itoa(n)
}

// SendEmail logs the intended email and returns a synthetic message id.
func (dryRunProvider) SendEmail(_ context.Context, to, subject, _ string) (string, error) {
	id := synthID("email")
	log.Printf("[DRY-RUN] email -> %s | subject=%q | id=%s", to, subject, id)
	return id, nil
}

// SendSMS logs the intended SMS and returns a synthetic message id.
func (dryRunProvider) SendSMS(_ context.Context, to, body string) (string, error) {
	id := synthID("sms")
	log.Printf("[DRY-RUN] sms -> %s | body=%q | id=%s", to, body, id)
	return id, nil
}

// PlaceCall logs the intended call and returns a synthetic call id.
func (dryRunProvider) PlaceCall(_ context.Context, to, objective string) (string, error) {
	id := synthID("call")
	log.Printf("[DRY-RUN] call -> %s | objective=%q | id=%s", to, objective, id)
	return id, nil
}

// itoa formats a non-negative int64 as a decimal string without importing strconv.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
