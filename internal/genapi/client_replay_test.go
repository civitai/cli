package genapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 🔴 The transparent-refresh replay is 401-ONLY, and that narrowness is the
// safety property — not an implementation detail.
//
// Surviving mutation: `status == http.StatusUnauthorized` → `status != 200`.
// TestAuthedDo_RefreshesOn401 stays green under it (a 401 still replays), so
// nothing in the suite could see the change. What it does is turn EVERY
// non-200 into a second request — including a 500, which is precisely the
// status the platform's own submit wrapper already retries 3× server-side. The
// idempotency key makes a replayed submit safe rather than a double charge
// (AGENTS.md item 16), but "safe" is not "free": it doubles the load a
// struggling orchestrator sees, and it burns a refresh on an error that has
// nothing to do with the credential.
//
// A 401 is the only status that a NEW TOKEN can plausibly fix. Everything else
// must be handed to the caller on the first response.
//
// Nothing here spends anything: WhatIfFromGraph is the read-only cost estimate,
// and it runs against httptest.

// replayProbe counts requests a server actually received and answers each with a
// scripted status.
type replayProbe struct {
	requests int
	statuses []int // one per request; the last is reused once exhausted
}

func (p *replayProbe) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		i := p.requests
		p.requests++
		st := p.statuses[len(p.statuses)-1]
		if i < len(p.statuses) {
			st = p.statuses[i]
		}
		if st == http.StatusOK {
			writeTRPC(w, map[string]any{"ready": true, "cost": map[string]any{"total": 60}})
			return
		}
		w.WriteHeader(st)
		_, _ = io.WriteString(w, `{"error":{"json":{"message":"scripted"}}}`)
	}
}

// TestAuthedDo_ReplaysOnlyOn401 measures the request count for a range of
// non-401 failures, and pairs every "1 request" with a 401 case in the SAME test
// that produces 2. A counter that never moved would report 1 everywhere.
func TestAuthedDo_ReplaysOnlyOn401(t *testing.T) {
	noReplay := []struct {
		name   string
		status int
	}{
		{"500 internal server error", http.StatusInternalServerError},
		{"502 bad gateway", http.StatusBadGateway},
		{"503 service unavailable", http.StatusServiceUnavailable},
		// 403 is the interesting one: it is auth-ADJACENT, so a "looks like an
		// auth problem" rewrite would sweep it in. A refresh cannot fix a token
		// that is valid but lacks the scope.
		{"403 forbidden", http.StatusForbidden},
		{"404 not found", http.StatusNotFound},
		{"400 bad request", http.StatusBadRequest},
		{"429 too many requests", http.StatusTooManyRequests},
	}

	for _, tc := range noReplay {
		t.Run(tc.name, func(t *testing.T) {
			probe := &replayProbe{statuses: []int{tc.status}}
			srv := httptest.NewServer(probe.handler())
			defer srv.Close()

			src := &refreshOnceSource{}
			c := NewWithSource(srv.URL, src)
			// The call fails; the failure is not what is under test.
			_, _, _ = c.WhatIfFromGraph(context.Background(), Graph{Workflow: "txt2img"})

			if probe.requests != 1 {
				t.Errorf("🔴 a %d was REPLAYED: the server saw %d requests, want exactly 1 — only a 401 may be retried",
					tc.status, probe.requests)
			}
			// The second, independent discriminator: a non-401 must not even ask
			// the token source to refresh.
			if src.calls != 0 {
				t.Errorf("🔴 a %d triggered %d token refresh(es), want 0", tc.status, src.calls)
			}
		})
	}

	// 🔴 POSITIVE CONTROL, same harness, same client, same call: a 401 DOES
	// replay. Without this, every "1 request" above is indistinguishable from a
	// probe that cannot count, a server never reached, or a client that never
	// retries anything at all.
	t.Run("positive control: 401 does replay", func(t *testing.T) {
		probe := &replayProbe{statuses: []int{http.StatusUnauthorized, http.StatusOK}}
		srv := httptest.NewServer(probe.handler())
		defer srv.Close()

		src := &refreshOnceSource{}
		c := NewWithSource(srv.URL, src)
		if _, _, err := c.WhatIfFromGraph(context.Background(), Graph{Workflow: "txt2img"}); err != nil {
			t.Fatalf("POSITIVE CONTROL FAILED: the 401→refresh→200 path errored: %v", err)
		}
		if probe.requests != 2 {
			t.Fatalf("POSITIVE CONTROL FAILED: a 401 produced %d requests, want 2 — the counts above prove nothing", probe.requests)
		}
		if src.calls != 1 {
			t.Fatalf("POSITIVE CONTROL FAILED: a 401 produced %d refreshes, want 1", src.calls)
		}
	})

	// A 401 that is STILL 401 after the refresh must stop at two requests, not
	// loop. Pins the "refresh once" half of the contract.
	t.Run("a persistent 401 replays exactly once", func(t *testing.T) {
		probe := &replayProbe{statuses: []int{http.StatusUnauthorized}}
		srv := httptest.NewServer(probe.handler())
		defer srv.Close()

		src := &refreshOnceSource{}
		c := NewWithSource(srv.URL, src)
		_, _, _ = c.WhatIfFromGraph(context.Background(), Graph{Workflow: "txt2img"})
		if probe.requests != 2 {
			t.Errorf("a persistent 401 produced %d requests, want exactly 2 (one replay, then give up)", probe.requests)
		}
		if src.calls != 1 {
			t.Errorf("a persistent 401 produced %d refreshes, want exactly 1", src.calls)
		}
	})
}
