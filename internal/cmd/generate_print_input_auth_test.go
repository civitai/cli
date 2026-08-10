package cmd

// `--print-input` WITHOUT `--image` REACHES NO REQUEST, SO IT MUST NOT DEMAND A
// CREDENTIAL — ISSUE #257.
//
// `generate`'s RunE refused unconditionally when no token was configured, before
// runGenerate was ever entered. That contradicted the invariant stated in the
// command's OWN comment at the --print-input short-circuit: "With no
// --checkpoint/--lora there is no request of any kind." The reporter measured it
// from the other side — with a garbage token the same invocation completes fully
// offline, which is what proves the credential was never used on that path.
//
// 🔴 THE GATE IS NARROWED, NOT DELETED, AND THIS FILE HAS TO SEE THE DIFFERENCE.
// Only `--image` genuinely needs a credential on the print path: hop 1 of the
// upload (`getConsumerBlobUploadUrl`) is AUTHED (AGENTS.md item 19(e)) and
// `--print-input` really does upload local files (item 19(f)). `--checkpoint` /
// `--lora` resolve through the PUBLIC model-version route (item 13), so gating on
// those would keep refusing a case that works with no credential. The rows below
// therefore pin BOTH directions — three refusals (--image, --dry-run, plain
// submit) and two permissions (bare --print-input, --print-input --checkpoint) —
// so neither "delete the gate" nor "widen the gate" passes.
//
// 🔴 CLASSIFICATION IS ASSERTED WITH errors.Is, NEVER MESSAGE TEXT (item 7). The
// sentinels leave Error() byte-identical, so a message assertion says nothing at
// all about the published exit code.
//
// 🔴 "STILL ErrUnauthorized" IS NOT ENOUGH TO PIN THE GATE, AND THAT WAS
// MEASURED. `internal/auth`'s token source refuses a credential-less request too,
// with its own ErrUnauthorized tag and without dialing — so with the top-level
// gate DELETED OUTRIGHT, every refusal row below still exits 3 and still records
// zero requests. The whole "delete the gate" mutation survived a table built on
// exit code plus offline-ness alone: a control satisfied by a bystander.
// The discriminator is the gate's PLACE IN THE ORDER, asserted structurally
// rather than by message. The gate runs BEFORE validateGenerateOpts, so an
// invocation that is credential-less AND usage-invalid comes back
// ErrUnauthorized today and flips to ErrUsage the moment the gate is gone.
// Those rows call `assertRefusedByTheGate`, and they are what makes deleting
// the gate a failing change rather than a passing one. There are THREE of them
// on the --image path alone, reached through three different usage-error sites:
// an audit measured the single-row version and found all three WIDENING mutants
// reddening exactly one leaf, so reshaping that row would have unguarded the
// `--image` half of the condition entirely.
//
// 🔴 "NO NETWORK" IS MEASURED, NOT INFERRED. Every run points CIVITAI_BASE_URL at
// a live httptest server that RECORDS each request, and the offline rows assert
// the recorder saw zero. A zero from a recorder wired to nothing would look
// identical, so TestGeneratePrintInput_NetworkRecorderPositiveControl drives the
// SAME recorder above zero through the same command and the same wiring.

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// recordedReq is one request that reached the base-URL server. hasAuth catches a
// hardcoded credential appearing on the model-version read; see the note on
// TestGeneratePrintInput_WithCheckpointNeedsNoCredential for what it does and
// does not establish.
type recordedReq struct {
	method  string
	path    string
	hasAuth bool
}

func (r recordedReq) String() string {
	auth := "no-auth"
	if r.hasAuth {
		auth = "AUTHED"
	}
	return r.method + " " + r.path + " (" + auth + ")"
}

// cliRun is one end-to-end invocation of the REAL command through NewRootCmd.
type cliRun struct {
	stdout string
	stderr string
	err    error
	// hits is every request that reached the base-URL server. It is the whole
	// point of the harness: the offline claim is a measured zero rather than an
	// assumption about which seams got built.
	hits []recordedReq
}

// runGenerateCLI drives `civitai generate …` with an isolated config dir and a
// recording server standing in for civitai.com.
//
// token == "" means NO credential is configured: XDG_CONFIG_HOME is a fresh
// t.TempDir() (so no config file exists) and CIVITAI_TOKEN is explicitly set to
// the empty string, which clears any ambient value the developer's shell holds.
func runGenerateCLI(t *testing.T, token string, args ...string) cliRun {
	t.Helper()

	var mu sync.Mutex
	var hits []recordedReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits = append(hits, recordedReq{
			method:  r.Method,
			path:    r.URL.Path,
			hasAuth: r.Header.Get("Authorization") != "",
		})
		mu.Unlock()
		// Deliberately unhelpful: no row here wants a successful round-trip.
		// Either the request should never have happened (the offline rows) or
		// the row only needs proof that one arrived. 404 rather than 5xx on
		// purpose — pkg/civitai's read loop RETRIES a 5xx with backoff, which
		// would make every row that does reach the server slow for no gain.
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"the test server answers nothing"}`))
	}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	t.Setenv("CIVITAI_TOKEN", token)

	root := NewRootCmd()
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	// --no-update-check keeps the release poll (a real network call to GitHub,
	// not to srv) out of the run entirely.
	root.SetArgs(append([]string{"generate", "--no-update-check"}, args...))
	err := root.Execute()

	mu.Lock()
	defer mu.Unlock()
	return cliRun{stdout: out.String(), stderr: errBuf.String(), err: err, hits: append([]recordedReq(nil), hits...)}
}

// assertOffline fails loudly, naming every request, if anything reached the
// server.
func assertOffline(t *testing.T, r cliRun) {
	t.Helper()
	if len(r.hits) != 0 {
		t.Fatalf("expected NO request, got %d: %v", len(r.hits), r.hits)
	}
}

// assertRefusedByTheGate is the discriminating assertion. Its input is an
// invocation that is BOTH credential-less and usage-invalid, so:
//
//	gate present -> ErrUnauthorized (the gate ran first)
//	gate absent  -> ErrUsage        (validateGenerateOpts got there instead)
//
// Neither branch dials, and both would satisfy a plain "is it ErrUnauthorized?"
// check via the token source — which is precisely why that check cannot pin the
// gate and this one can.
//
// `via` names the usage-error site the row falls through to when the gate is
// absent; it is printed on failure so the message says WHICH check won the race.
func assertRefusedByTheGate(t *testing.T, r cliRun, via string) {
	t.Helper()
	if errors.Is(r.err, ErrUsage) {
		t.Fatalf("refused as a USAGE error by %s, so the credential gate no longer runs ahead of it: %v", via, r.err)
	}
	if !errors.Is(r.err, civitai.ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized from the credential gate (ahead of %s), got %v", via, r.err)
	}
	assertOffline(t, r)
}

// writeTinyPNG writes a real 1x1 PNG, so the --image row stays meaningful if the
// credential gate ever moves below the file read.
func writeTinyPNG(t *testing.T, dir string) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "ref.png")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestGeneratePrintInput_NoCredentialNeeded is the regression: RED at 8ed4d69
// with `errors.Is(err, civitai.ErrUnauthorized)` where nil was wanted.
func TestGeneratePrintInput_NoCredentialNeeded(t *testing.T) {
	r := runGenerateCLI(t, "", "a cat", "--print-input", "--quantity", "2")

	if r.err != nil {
		t.Fatalf("--print-input with no credential must succeed, got %v (unauthorized=%v)",
			r.err, errors.Is(r.err, civitai.ErrUnauthorized))
	}
	assertOffline(t, r)

	// 🔴 DECODED, NEVER strings.Contains — a substring search cannot tell "cfg"
	// from "cfgScale" (AGENTS.md item 14), so it cannot assert a key at all.
	var m map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &m); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, r.stdout)
	}
	if got := m["workflow"]; got != "txt2img" {
		t.Errorf("workflow = %v, want txt2img", got)
	}
	if got := m["prompt"]; got != "a cat" {
		t.Errorf("prompt = %v, want %q", got, "a cat")
	}
	// --quantity 2 was passed, so the key must be PRESENT and 2 — otherwise a
	// document that decodes fine could still be an empty graph.
	if got, ok := m["quantity"]; !ok || got != float64(2) {
		t.Errorf("quantity = %v (present=%v), want 2", got, ok)
	}
	// And an unset lever must be ABSENT rather than a zero the server would
	// price as a degenerate job (item 14).
	if _, ok := m["steps"]; ok {
		t.Errorf("steps must be absent from the printed graph, got %v", m["steps"])
	}
}

// TestGeneratePrintInput_WithImageStillNeedsACredential is the POSITIVE CONTROL
// on the gate itself: without it, deleting the credential check outright would
// pass every other row in this file.
//
// It is also the reason the narrowing is `--image` and not "--print-input is
// always free": the local file is UPLOADED before the graph is printed
// (item 19(f)) and the presign hop is authed (item 19(e)).
func TestGeneratePrintInput_WithImageStillNeedsACredential(t *testing.T) {
	dir := t.TempDir()
	img := writeTinyPNG(t, dir)

	t.Run("the refusal itself", func(t *testing.T) {
		// --ecosystem is supplied so the invocation is otherwise VALID: with the
		// gate deleted this run proceeds to reading and decoding the file and on
		// towards the presign, so the row is about a real path rather than an
		// invocation that would be refused anyway.
		r := runGenerateCLI(t, "", "a cat", "--print-input", "--image", img, "--ecosystem", "Qwen")
		if !errors.Is(r.err, civitai.ErrUnauthorized) {
			t.Fatalf("--print-input --image with no credential must be ErrUnauthorized, got %v", r.err)
		}
		assertOffline(t, r)
		if strings.TrimSpace(r.stdout) != "" {
			t.Errorf("nothing may be printed on a refusal, got %q", r.stdout)
		}
	})

	// 🔴 THREE discriminators, reached through THREE DIFFERENT usage-error
	// sites, because one row is not a battery.
	//
	// An audit measured the first version: all three widening mutants — dropping
	// `len(o.images) > 0`, swapping it for `len(o.loras) > 0`, replacing it with
	// `false` — reddened EXACTLY ONE leaf subtest in the whole package, and the
	// sibling "the refusal itself" survived every one of them (carried by the
	// genapi token-source bystander). Delete or reshape that single row and the
	// `--image` half of the condition is unguarded. That is AGENTS item 24's
	// recorded "battery rested on a single row" shape, regenerated here.
	//
	// So the rows are routed through validateImageOpts (before the graph is
	// built), parseImageFlag (inside resolveImages) and resolveLocalImage's
	// stat (deepest, one step before the upload). A change that silences any
	// ONE of those sites cannot silence the other two.
	// 🔴 AND THE THREE-SITE PROPERTY IS ASSERTED, NOT ASSERTED-IN-A-COMMENT.
	// An audit demonstrated the gap: `via` was interpolated only into FAILURE
	// messages, so a passing row never printed it — collapse rows 2 and 3 onto
	// row 1's site and the suite stayed fully green at 16 RUN / 0 FAIL, in a
	// file whose central thesis is that one row is not a battery. Each row now
	// carries a `fingerprint` of its site's message and a PREMISE subtest that
	// runs the same invocation WITH a credential — the gate does not fire, the
	// run falls through to the site the row names, and the fingerprint is
	// checked. `assertPairwiseDistinctSites` then requires each row's error to
	// carry its OWN fingerprint and NONE of the others', so two rows that
	// collapse onto one site fail by name.
	rows := []struct {
		name string
		args []string
		// via names the usage-error site the row reaches when the gate is
		// ABSENT.
		via string
		// fingerprint is a substring unique to that site's message. It is used
		// ONLY to identify WHICH check answered — never to assert the
		// classification, which stays errors.Is per item 7.
		fingerprint string
	}{
		{
			name:        "no --ecosystem",
			args:        []string{"a cat", "--print-input", "--image", img},
			via:         "validateImageOpts (item 19(b): --image requires --ecosystem)",
			fingerprint: "--image requires --ecosystem",
		},
		{
			name:        "empty --image value",
			args:        []string{"a cat", "--print-input", "--ecosystem", "Qwen", "--image", ""},
			via:         "parseImageFlag (an empty value is not a path or an https URL)",
			fingerprint: "the value is empty",
		},
		{
			name:        "--image is a directory",
			args:        []string{"a cat", "--print-input", "--ecosystem", "Qwen", "--image", dir},
			via:         "resolveLocalImage (os.Stat says directory, one step before the upload)",
			fingerprint: "is a directory, not an image file",
		},
	}

	for _, tc := range rows {
		t.Run("and it is THIS gate that refuses: "+tc.name, func(t *testing.T) {
			// Credential-less AND usage-invalid. ErrUnauthorized means the gate
			// won the race; ErrUsage means the gate is gone and tc.via got there
			// first.
			r := runGenerateCLI(t, "", tc.args...)
			assertRefusedByTheGate(t, r, tc.via)
		})
	}

	// The premise of every row above: with the gate out of the way, this
	// invocation really does reach the site it names. Without this, the rows
	// are three spellings of one claim and nobody would know.
	t.Run("each row reaches the DISTINCT site it names", func(t *testing.T) {
		seen := make(map[string]string, len(rows))
		for _, tc := range rows {
			r := runGenerateCLI(t, "not-a-real-token-000", tc.args...)
			if !errors.Is(r.err, ErrUsage) {
				t.Fatalf("%s: with a credential the run must fall through to %s and fail as ErrUsage, got %v",
					tc.name, tc.via, r.err)
			}
			if !strings.Contains(r.err.Error(), tc.fingerprint) {
				t.Fatalf("%s: expected the error from %s (fingerprint %q), got %v",
					tc.name, tc.via, tc.fingerprint, r.err)
			}
			// Pairwise distinctness, both directions: this row's error must
			// carry NO other row's fingerprint, and no two rows may share one.
			for _, other := range rows {
				if other.via == tc.via {
					continue
				}
				if strings.Contains(r.err.Error(), other.fingerprint) {
					t.Fatalf("%s reached %s as well as %s — the rows are not independent sites",
						tc.name, other.via, tc.via)
				}
			}
			if prev, dup := seen[tc.fingerprint]; dup {
				t.Fatalf("%s and %s share the fingerprint %q, so one of them is not pinning its own site",
					prev, tc.name, tc.fingerprint)
			}
			seen[tc.fingerprint] = tc.name
			assertOffline(t, r)
		}
		if len(seen) != len(rows) {
			t.Fatalf("expected %d distinct sites, saw %d", len(rows), len(seen))
		}
	})
}

// TestGenerateDryRun_StillNeedsACredential — --dry-run reaches the estimator, an
// authed tRPC query. CONTROL: green at base, so the plain half is an invariant
// guard rather than regression coverage. The second half is not: it is what
// makes deleting the gate visible.
func TestGenerateDryRun_StillNeedsACredential(t *testing.T) {
	t.Run("the refusal itself", func(t *testing.T) {
		r := runGenerateCLI(t, "", "a cat", "--dry-run")
		if !errors.Is(r.err, civitai.ErrUnauthorized) {
			t.Fatalf("--dry-run with no credential must be ErrUnauthorized, got %v", r.err)
		}
		assertOffline(t, r)
	})

	t.Run("and it is THIS gate that refuses", func(t *testing.T) {
		// --quantity 0 is a usage error (the server would silently clamp it to 1
		// and charge), so it is the second half of the discriminator.
		r := runGenerateCLI(t, "", "a cat", "--dry-run", "--quantity", "0")
		assertRefusedByTheGate(t, r, "validateGenerateOpts (--quantity must be at least 1)")
	})
}

// TestGenerateSubmit_StillNeedsACredential — the money path.
func TestGenerateSubmit_StillNeedsACredential(t *testing.T) {
	t.Run("the refusal itself", func(t *testing.T) {
		r := runGenerateCLI(t, "", "a cat", "--yes")
		if !errors.Is(r.err, civitai.ErrUnauthorized) {
			t.Fatalf("a plain submit with no credential must be ErrUnauthorized, got %v", r.err)
		}
		assertOffline(t, r)
	})

	t.Run("and it is THIS gate that refuses", func(t *testing.T) {
		r := runGenerateCLI(t, "", "a cat", "--yes", "--quantity", "0")
		assertRefusedByTheGate(t, r, "validateGenerateOpts (--quantity must be at least 1)")
	})
}

// TestGeneratePrintInput_SkipsTheGateNotTheValidation is the mirror image of
// assertRefusedByTheGate, and it is what stops the narrowing from being read as
// "--print-input bypasses the front of the command".
//
// The SAME usage-invalid invocation that comes back ErrUnauthorized on every
// other path must come back ErrUsage here, because the gate is skipped and
// validateGenerateOpts is reached. A widening of the skip (say, dropping the
// `len(o.images) > 0` term and letting --image through) is caught by the row
// above; a narrowing back to the unconditional gate is caught by this one.
func TestGeneratePrintInput_SkipsTheGateNotTheValidation(t *testing.T) {
	r := runGenerateCLI(t, "", "a cat", "--print-input", "--quantity", "0")

	if errors.Is(r.err, civitai.ErrUnauthorized) {
		t.Fatalf("the credential gate still fired on a --print-input run that needs no credential: %v", r.err)
	}
	if !errors.Is(r.err, ErrUsage) {
		t.Fatalf("want ErrUsage from validateGenerateOpts, got %v", r.err)
	}
	assertOffline(t, r)
}

// TestGeneratePrintInput_WithCheckpointNeedsNoCredential is the row that pins
// the NARROWING, and it was added because the obvious wider gate — the one
// issue #257 proposes, refusing whenever --image OR --checkpoint OR --lora is
// present — survived every other assertion in this file.
//
// `--checkpoint` resolves through `GET /api/v1/model-versions/{id}`, which is
// PUBLIC (AGENTS.md item 13 calls it "free, unauthenticated-capable"). So the
// run must be let through and must actually reach that route.
//
// 🔴 KEEP THE hasAuth ASSERTION'S CLAIM AT ITS MEASURED SIZE. Every row in this
// file runs with token == "", and `doOnceHdr` attaches no header on an empty
// token — so this can only catch a HARDCODED credential appearing on the
// version read (mutation-checked: forcing `Authorization: Bearer …` into
// `doOnceHdr` does redden it, so it is a real guard, not a decoration). It does
// NOT establish that the route is public: that is item 13's claim, established
// server-side, and this row consumes it rather than proving it.
//
// The server answers 404, so the run fails; that is fine and deliberate. The
// claim is about WHERE it got to, not that it succeeded.
// 🔴 BOTH FLAGS GET A ROW. `--checkpoint` alone was pinned, and an audit
// measured the consequence: an ADDITIVE `|| len(o.loras) > 0` widening survived
// all 1324 subtests in this package. The gate's own comment treats the two
// identically, so pinning one and not the other made half of that comment a
// claim nothing held.
func TestGeneratePrintInput_WithCheckpointNeedsNoCredential(t *testing.T) {
	for _, tc := range []struct {
		flag, value string
	}{
		{"--checkpoint", "128713"},
		{"--lora", "128713"},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			r := runGenerateCLI(t, "", "a cat", "--print-input", tc.flag, tc.value)

			if errors.Is(r.err, civitai.ErrUnauthorized) {
				t.Fatalf("--print-input %s needs no credential (the version read is public), but it was refused: %v",
					tc.flag, r.err)
			}
			var public int
			for _, h := range r.hits {
				if strings.Contains(h.path, "/model-versions/") {
					if h.hasAuth {
						t.Errorf("the model-version read carried an Authorization header: %s", h)
					}
					public++
				}
			}
			if public == 0 {
				t.Fatalf("the run never reached the public model-version read; hits=%v err=%v", r.hits, r.err)
			}
		})
	}
}

// TestGeneratePrintInput_WithCredentialUnchanged — the credentialed print path
// behaved this way before the change and must keep doing so: a token present
// changes nothing about what --print-input does, including that it sends
// nothing.
func TestGeneratePrintInput_WithCredentialUnchanged(t *testing.T) {
	r := runGenerateCLI(t, "not-a-real-token-000", "a cat", "--print-input")
	if r.err != nil {
		t.Fatalf("--print-input with a credential must succeed, got %v", r.err)
	}
	assertOffline(t, r)

	var m map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &m); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, r.stdout)
	}
	if got := m["prompt"]; got != "a cat" {
		t.Errorf("prompt = %v, want %q", got, "a cat")
	}
}

// TestGeneratePrintInput_NetworkRecorderPositiveControl is what stops every
// `assertOffline` above from being a reassuring zero.
//
// It drives the SAME recorder, through the SAME NewRootCmd wiring and the same
// CIVITAI_BASE_URL, with a credential and --dry-run — an invocation that MUST
// reach the estimator. If this ever reports zero hits, the harness is observing
// nothing and none of the offline rows mean anything.
//
// It doubles as the discriminator for the three refusal rows: it proves they are
// refused BY THE CREDENTIAL GATE (same command, credential added, request goes
// out) rather than by something else that would refuse regardless.
func TestGeneratePrintInput_NetworkRecorderPositiveControl(t *testing.T) {
	r := runGenerateCLI(t, "not-a-real-token-000", "a cat", "--dry-run")

	if len(r.hits) == 0 {
		t.Fatal("the recorder saw ZERO requests on an invocation that must reach the estimator — " +
			"the harness is wired to nothing and every offline assertion in this file is vacuous")
	}
	// The server answers 404, so this must fail — but never as ErrUnauthorized:
	// the refusal in the rows above came from the local gate, not from a shape
	// this run also produces.
	if r.err == nil {
		t.Fatal("a 404 from the estimator must surface as an error")
	}
	if errors.Is(r.err, civitai.ErrUnauthorized) {
		t.Fatalf("the credentialed run must not be refused for lack of a credential: %v", r.err)
	}
}
