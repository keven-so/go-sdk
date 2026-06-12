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

var dryRunSeq atomic.Int64

func synthID(prefix string) string {
	n := dryRunSeq.Add(1)
	return prefix + "_dryrun_" + itoa(n)
}

func (dryRunProvider) SendEmail(_ context.Context, to, subject, _ string) (string, error) {
	id := synthID("email")
	log.Printf("[DRY-RUN] email -> %s | subject=%q | id=%s", to, subject, id)
	return id, nil
}

func (dryRunProvider) SendSMS(_ context.Context, to, body string) (string, error) {
	id := synthID("sms")
	log.Printf("[DRY-RUN] sms -> %s | body=%q | id=%s", to, body, id)
	return id, nil
}

func (dryRunProvider) PlaceCall(_ context.Context, to, objective string) (string, error) {
	id := synthID("call")
	log.Printf("[DRY-RUN] call -> %s | objective=%q | id=%s", to, objective, id)
	return id, nil
}

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
