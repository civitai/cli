package scaffold

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// GUARD B — the runtime half of the ready-ack contract.
//
// Guard A (ready_ack_contract_test.go) proves a byte-identical copy of the
// canonical emitter is present and loaded. It CANNOT prove the ack ever fires:
// an emitter whose source check is inverted, or whose listener is registered
// on `document`, or inside a slow async chain, satisfies every static
// assertion and is completely inert in a browser. That is the shape of issue
// #206 — the templates' JS ran fine and simply never said the right thing.
//
// So this guard executes the emitter. It renders each template, loads the
// shipped emitter under a `window` stub in node, plays the host's side of the
// handshake at it, and asserts on what actually came back out:
//
//   - exactly ONE outbound message, and only after a real BLOCK_INIT;
//   - `{ type: 'BLOCK_READY', payload: { height: 0 } }` — the envelope, not
//     top-level fields;
//   - addressed at the host's origin, never `'*'`;
//   - a second BLOCK_INIT produces no second ack.
//
// It needs node, so it is env-gated exactly like the network guard in
// pins_guard_test.go: `make ci` / `go test ./...` stays Go-only and offline.
// It has TWO runners, and the per-PR one is the load-bearing half:
//   - .github/workflows/ci.yml, job `ready-ack-runtime` — runs on every pull
//     request, so a regression BLOCKS the merge. The first version of this
//     guard had only the scheduled runner below, which meant the very failure
//     the PR advertised it caught (inverting the `event.source` check) would
//     have merged green and been reported up to 24h later, with nothing gated
//     on the result.
//   - .github/workflows/bump-scaffold-pins.yml, job `ready-ack-runtime` — the
//     daily sweep, which catches drift on `main` between PRs.
//
// WHAT IT STILL DOES NOT PROVE: that the REAL host accepts the ack. The stub
// is this repo's reading of the contract, not the contract. The end-to-end
// evidence is a browser run against the real PageBlockHost, recorded in the
// PR that introduced this.
// ---------------------------------------------------------------------------

const readyAckRuntimeEnv = "CIVITAI_CHECK_SCAFFOLD_RUNTIME"

// driverResult mirrors the JSON testdata/readyack/driver.mjs prints.
type driverResult struct {
	EmitterPath            string   `json:"emitterPath"`
	LoadError              string   `json:"loadError"`
	HostOrigin             string   `json:"hostOrigin"`
	WindowMessageListeners int      `json:"windowMessageListeners"`
	RegisteredTypes        []string `json:"registeredTypes"`
	Posted                 []struct {
		Window  string `json:"window"`
		Message struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
			Height  *int            `json:"height"`
		} `json:"message"`
		TargetOrigin string `json:"targetOrigin"`
	} `json:"posted"`
	Threw []struct {
		TargetOrigin string `json:"targetOrigin"`
	} `json:"threw"`
	ListenerErrors []string `json:"listenerErrors"`
	Steps          []struct {
		Label string `json:"label"`
		Posts int    `json:"posts"`
	} `json:"steps"`
}

func (r driverResult) postsAt(label string) (int, bool) {
	for _, s := range r.Steps {
		if s.Label == label {
			return s.Posts, true
		}
	}
	return 0, false
}

// handshakeProblems is the whole verdict, as data. Returning problems instead
// of calling t.Errorf directly is what lets the controls below feed a
// known-BAD emitter through the very same assertions and require them to
// speak up — a check that only ever runs against the thing it approves of has
// never been shown to be able to disapprove.
func handshakeProblems(r driverResult) []string {
	var p []string
	if r.LoadError != "" {
		p = append(p, "emitter threw while loading: "+r.LoadError)
	}
	if r.WindowMessageListeners < 1 {
		p = append(p, fmt.Sprintf("registered no `message` listener on window (types seen: %v) — "+
			"a listener on `document`, or one attached inside an async chain, never sees the host's BLOCK_INIT",
			r.RegisteredTypes))
	}
	for _, label := range []string{"after-load", "after-foreign-source", "after-unrelated-type"} {
		n, ok := r.postsAt(label)
		if !ok {
			p = append(p, "driver reported no `"+label+"` step — the driver and this test disagree")
			continue
		}
		if n != 0 {
			p = append(p, fmt.Sprintf("posted %d message(s) %s — the ack must fire only for a BLOCK_INIT from window.parent", n, label))
		}
	}
	if n, ok := r.postsAt("after-first-init"); ok && n != 1 {
		p = append(p, fmt.Sprintf("posted %d message(s) after the host's first BLOCK_INIT, want exactly 1", n))
	}
	first, _ := r.postsAt("after-first-init")
	if n, ok := r.postsAt("after-second-init"); ok && n != first {
		p = append(p, fmt.Sprintf("a repeated BLOCK_INIT produced %d more message(s) — the host re-posts "+
			"BLOCK_INIT until it sees the ack and rate-limits inbound traffic, so repeats must be a no-op", n-first))
	}

	if len(r.Posted) == 0 {
		p = append(p, "posted NOTHING — the host would never reveal this app (issue #206)")
		return p
	}
	m := r.Posted[0]
	if m.Window != "parent" {
		p = append(p, "first message went to the "+m.Window+" window, not window.parent")
	}
	if m.Message.Type != "BLOCK_READY" {
		p = append(p, "first message type is "+m.Message.Type+", want BLOCK_READY")
	}
	switch {
	case len(m.Message.Payload) == 0:
		p = append(p, "BLOCK_READY carries no `payload` — the host reads event.data.payload, so "+
			"top-level fields arrive as undefined and teach every later message the wrong envelope")
	default:
		var pl struct {
			Height *int `json:"height"`
		}
		if err := json.Unmarshal(m.Message.Payload, &pl); err != nil || pl.Height == nil {
			p = append(p, "BLOCK_READY payload has no numeric `height`: "+string(m.Message.Payload))
		} else if *pl.Height != 0 {
			p = append(p, fmt.Sprintf("BLOCK_READY payload height is %d, want the 0 placeholder", *pl.Height))
		}
	}
	if m.Message.Height != nil {
		p = append(p, "BLOCK_READY puts `height` at the TOP LEVEL — it belongs inside `payload`")
	}
	if m.TargetOrigin == "*" {
		p = append(p, "BLOCK_READY was broadcast to '*' — BLOCK_INIT hands over the real host origin, use it")
	} else if m.TargetOrigin != r.HostOrigin {
		p = append(p, "BLOCK_READY was addressed at "+m.TargetOrigin+", want the host origin "+r.HostOrigin)
	}
	return p
}

func runReadyAckDriver(t *testing.T, node, emitter string) driverResult {
	t.Helper()
	return runReadyAckDriverArgs(t, node, emitter)
}

func runReadyAckDriverArgs(t *testing.T, node, emitter string, extra ...string) driverResult {
	t.Helper()
	driver := filepath.Join("testdata", "readyack", "driver.mjs")
	cmd := exec.Command(node, append([]string{driver, emitter}, extra...)...)
	// Capture stdout and stderr SEPARATELY: a node warning on stderr must not
	// corrupt the JSON, and a decode failure must be able to show the stderr.
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("running %s %s: %v\nstdout:\n%s\nstderr:\n%s", driver, emitter, err, stdout.String(), stderr.String())
	}
	var res driverResult
	if err := json.Unmarshal([]byte(stdout.String()), &res); err != nil {
		t.Fatalf("driver output is not the JSON this test expects (%v).\nstdout:\n%s\nstderr:\n%s",
			err, stdout.String(), stderr.String())
	}
	return res
}

func TestScaffoldedReadyAckActuallyFires(t *testing.T) {
	if os.Getenv(readyAckRuntimeEnv) != "1" {
		t.Skip("runtime guard; set " + readyAckRuntimeEnv + "=1 to run (the daily bump-scaffold-pins workflow does)")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		// Deliberately NOT a skip. The gate is set by a runner that is supposed
		// to provide node; skipping here would turn a broken runner into a
		// green run.
		t.Fatalf("%s=1 but node is not on PATH: %v", readyAckRuntimeEnv, err)
	}

	// ---- CONTROLS FIRST -----------------------------------------------------
	// Validate the harness against known-BAD emitters before reading its
	// verdict on the real ones. Until these have been watched producing
	// problems, a clean report below is a fact about the harness, not the code.
	t.Run("control: an inert emitter is reported as broken", func(t *testing.T) {
		res := runReadyAckDriver(t, node, filepath.Join("testdata", "readyack", "inert-control.js"))
		problems := handshakeProblems(res)
		if len(problems) == 0 {
			t.Fatal("the assertions passed an emitter that does NOTHING — they are wired to nothing, " +
				"and every other result in this file is meaningless")
		}
		if !anyContains(problems, "posted NOTHING") {
			t.Fatalf("inert emitter rejected for the wrong reason: %v", problems)
		}
		t.Logf("inert control correctly reported: %s", strings.Join(problems, "; "))
	})

	t.Run("control: a naive ack is reported as broken", func(t *testing.T) {
		res := runReadyAckDriver(t, node, filepath.Join("testdata", "readyack", "naive-control.js"))
		problems := handshakeProblems(res)
		if len(problems) == 0 {
			t.Fatal("the assertions passed a naive ack with the wrong envelope, a '*' target, no " +
				"BLOCK_INIT gate and no dedupe — they cannot detect any of those")
		}
		// COUNT POSITIVE CONTROL: the driver's counter must be able to move to
		// something other than the reassuring number. This fixture posts on
		// EVERY delivered message, so a non-zero count here proves the "0 posts
		// before BLOCK_INIT" assertion above is an observation, not a constant.
		if n, ok := res.postsAt("after-foreign-source"); !ok || n == 0 {
			t.Fatalf("the driver observed %d posts from a fixture that posts on every message — "+
				"its counter never moves, so the zeros it reports elsewhere prove nothing", n)
		}
		for _, want := range []string{"broadcast to '*'", "TOP LEVEL", "no-op"} {
			if !anyContains(problems, want) {
				t.Errorf("the naive control was not faulted for %q — that assertion is unproven.\nreported: %v",
					want, problems)
			}
		}
		t.Logf("naive control correctly reported: %s", strings.Join(problems, "; "))
	})

	// ---- THE REAL TEMPLATES -------------------------------------------------
	examined := 0
	for _, tmpl := range AllTemplates() {
		rel := tmpl.ReadyAckPath()
		if rel == "" {
			continue // covered by Guard A, which decides WHICH templates need one
		}
		dest := filepath.Join(t.TempDir(), string(tmpl))
		if _, err := Render(tmpl, dest, Data{Slug: "ready-block", Name: "Ready Block"}); err != nil {
			t.Fatalf("render %s: %v", tmpl, err)
		}
		examined++

		t.Run(string(tmpl), func(t *testing.T) {
			emitter := filepath.Join(dest, filepath.FromSlash(rel))
			res := runReadyAckDriver(t, node, emitter)
			for _, p := range handshakeProblems(res) {
				t.Errorf("%s's shipped %s: %s", tmpl, rel, p)
			}
			t.Logf("%s: %d window message listener(s); posted %d message(s); first -> %s",
				tmpl, res.WindowMessageListeners, len(res.Posted),
				func() string {
					if len(res.Posted) == 0 {
						return "<nothing>"
					}
					return res.Posted[0].TargetOrigin
				}())

			// The "already acked" flag must latch AFTER a successful post, not
			// before. A browser throws SyntaxError when targetOrigin cannot be
			// parsed as a URL, and `event.origin` is the string "null" whenever
			// the sender sits at an opaque origin — an emitter that latched
			// first would be permanently silent from then on, with the host
			// still retrying at a listener that has given up.
			//
			// Not reachable through today's PageBlockHost (it is not at an
			// opaque origin), so this pins a property, not a live bug.
			t.Run("recovers when the first post throws", func(t *testing.T) {
				res := runReadyAckDriverArgs(t, node, emitter, "--throw-first-post")
				if len(res.Threw) == 0 {
					t.Fatal("the driver was asked to make the first post throw and nothing threw — " +
						"the control did not fire, so this sub-test proves nothing")
				}
				if n, _ := res.postsAt("after-first-init"); n != 0 {
					t.Fatalf("expected the first ack to be lost to the throw, saw %d post(s)", n)
				}
				n, _ := res.postsAt("after-second-init")
				if n != 1 {
					t.Errorf("after a throwing first post, the host's NEXT BLOCK_INIT produced %d ack(s), want 1 — "+
						"the emitter latched its acked flag before posting, so a throw silences it forever", n)
				}
				t.Logf("%s: first post threw (%d), recovered on the retry (%d post(s))", tmpl, len(res.Threw), n)
			})
		})
	}

	if examined < minPageTemplatesNeedingAck {
		t.Fatalf("the runtime guard executed only %d emitter(s), expected at least %d — "+
			"it stopped observing rather than the templates getting safer",
			examined, minPageTemplatesNeedingAck)
	}
}

func anyContains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if strings.Contains(h, needle) {
			return true
		}
	}
	return false
}
