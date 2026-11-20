package cmd

// `civitai generate --buzz-type` — civitai/cli#801.
//
// A user on civitai.red had a mature result paid in Blue Buzz come back locked,
// then paid again in Yellow on the site to unlock it. The submit mutation
// already takes the account to pay from (`buzzType`, a sibling of `input` in the
// envelope — civitai/civitai -> src/server/routers/orchestrator.router.ts,
// `generateFromGraph` -> `resolveGenerationCurrencies`); the CLI just never sent
// it, and refuses it inside an --input file because it is not a graph key.
//
// 🔴 EVERY CASE HERE DRIVES THE REAL COMMAND, THROUGH NewRootCmd, AGAINST A
// RECORDING SERVER. The flag is parsed by cobra, the key is serialised by the real
// genapi client, and the assertion reads the BYTES that reached the server. A
// seam-level fake would let a flag that is parsed but never forwarded (or
// forwarded on the estimate instead of the submit) pass.
//
// 🔴 THE ZEROS ARE PAIRED WITH THEIR POSITIVE CONTROLS IN THIS FILE. "No
// buzzType on the wire" is asserted only beside a case where the same recorder
// SEES buzzType on the wire, and "no submit" only beside a case where the same
// recorder sees the POST.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// buzzTypeServer stands in for the three routes a submit touches: the cost
// estimate, the balance read and the submit. It records what each one received.
type buzzTypeServer struct {
	mu sync.Mutex
	// whatIfInputs is the decoded `input` query parameter of every estimate.
	whatIfInputs []string
	// submitBodies is every submit body, raw.
	submitBodies [][]byte
	// account is the balance reply; cost is the estimate's total.
	account map[string]int64
	cost    float64
	// dropSubmit closes the connection on the submit instead of answering, so
	// the reply is lost the way a crash mid-POST loses it.
	dropSubmit bool
}

func (s *buzzTypeServer) handler(t *testing.T) http.Handler {
	t.Helper()
	trpc := func(w http.ResponseWriter, payload any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"data": map[string]any{"json": payload}}})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "orchestrator.whatIfFromGraph"):
			s.whatIfInputs = append(s.whatIfInputs, r.URL.Query().Get("input"))
			trpc(w, map[string]any{"ready": true, "cost": map[string]any{"base": 8, "total": s.cost}})
		case strings.HasSuffix(r.URL.Path, "orchestrator.generateFromGraph") && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			s.submitBodies = append(s.submitBodies, raw)
			if s.dropSubmit {
				if hj, ok := w.(http.Hijacker); ok {
					if conn, _, err := hj.Hijack(); err == nil {
						_ = conn.Close()
						return
					}
				}
				t.Error("could not drop the submit connection")
				return
			}
			trpc(w, map[string]any{"id": "wf_801", "status": "queued", "cost": map[string]any{"total": s.cost}})
		case strings.HasSuffix(r.URL.Path, "buzz.getBuzzAccount"):
			trpc(w, s.account)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not a route this test serves"}`))
		}
	})
}

type buzzTypeRun struct {
	stdout, stderr string
	err            error
	whatIfInputs   []string
	submitBodies   [][]byte
}

// runBuzzTypeCLI runs `civitai generate <args>` against a fresh recording server
// with an isolated config dir and a credential configured.
func runBuzzTypeCLI(t *testing.T, account map[string]int64, cost float64, args ...string) buzzTypeRun {
	t.Helper()
	return runBuzzTypeCLIWith(t, &buzzTypeServer{account: account, cost: cost}, args...)
}

func runBuzzTypeCLIWith(t *testing.T, s *buzzTypeServer, args ...string) buzzTypeRun {
	t.Helper()
	srv := httptest.NewServer(s.handler(t))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	t.Setenv("CIVITAI_TOKEN", "test-key")

	root := NewRootCmd()
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(append([]string{"generate", "--no-update-check"}, args...))
	err := root.Execute()

	s.mu.Lock()
	defer s.mu.Unlock()
	return buzzTypeRun{
		stdout: out.String(), stderr: errBuf.String(), err: err,
		whatIfInputs: append([]string(nil), s.whatIfInputs...),
		submitBodies: append([][]byte(nil), s.submitBodies...),
	}
}

// plentyOfEverything is a balance no case below runs out of.
var plentyOfEverything = map[string]int64{"blue": 1000, "yellow": 1000, "green": 1000}

// submitEnvelope decodes one submit body into its envelope (`json`) and graph
// (`json.input`).
func submitEnvelope(t *testing.T, raw []byte) (env, graph map[string]any) {
	t.Helper()
	var body struct {
		JSON map[string]any `json:"json"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("submit body is not JSON: %v: %s", err, raw)
	}
	graph, _ = body.JSON["input"].(map[string]any)
	if graph == nil {
		t.Fatalf("submit body has no json.input graph: %s", raw)
	}
	return body.JSON, graph
}

// assertNoBuzzTypeOnEstimate: the estimate must never carry the key. whatIf
// spreads every key it does not destructure into the graph it validates, so a
// buzzType there is a GRAPH key, not an account choice.
func assertNoBuzzTypeOnEstimate(t *testing.T, r buzzTypeRun) {
	t.Helper()
	if len(r.whatIfInputs) == 0 {
		t.Fatal("no estimate reached the server — the assertion below would be vacuous")
	}
	for _, in := range r.whatIfInputs {
		if strings.Contains(in, "buzzType") {
			t.Errorf("the estimate request carries buzzType: %s", in)
		}
	}
}

// 🔴 The chosen account reaches the SUBMIT, as an envelope sibling of `input`,
// verbatim in the site's own vocabulary — and the estimate does not carry it.
// This is also the POSITIVE CONTROL for every "absent" assertion in this file:
// the same recorder, the same decoding, SEEING the key.
func TestGenerateBuzzType_SubmitCarriesTheChosenAccount(t *testing.T) {
	withStdinTTY(t, false)
	cases := []struct{ flag, want string }{
		{"yellow", "yellow"},
		{"blue", "blue"},
		{"green", "green"},
		// Normalised to the server's lower-case value rather than refused.
		{" Yellow ", "yellow"},
	}
	for _, tc := range cases {
		t.Run(tc.flag, func(t *testing.T) {
			r := runBuzzTypeCLI(t, plentyOfEverything, 12, "a cat", "--buzz-type", tc.flag, "--yes", "--no-wait")
			if r.err != nil {
				t.Fatalf("--buzz-type %q: want a submit, got %v\nstderr: %s", tc.flag, r.err, r.stderr)
			}
			if len(r.submitBodies) != 1 {
				t.Fatalf("submits = %d, want 1", len(r.submitBodies))
			}
			env, graph := submitEnvelope(t, r.submitBodies[0])
			if got := env["buzzType"]; got != tc.want {
				t.Errorf("envelope buzzType = %#v, want %q (body %s)", got, tc.want, r.submitBodies[0])
			}
			if _, inGraph := graph["buzzType"]; inGraph {
				t.Errorf("buzzType leaked INTO the graph, where the server does not read it: %s", r.submitBodies[0])
			}
			assertNoBuzzTypeOnEstimate(t, r)
		})
	}
}

// Default unchanged: with no --buzz-type the key is ABSENT from the submit (not
// an empty string), so the server keeps its own default spend order.
//
// ⚠ INVARIANT GUARD, not regression coverage: main never sent the key, so this
// passes there too. It is here so the flag cannot start sending `""` or a
// default value. Its positive control is the case above.
func TestGenerateBuzzType_UnsetLeavesTheKeyAbsent(t *testing.T) {
	withStdinTTY(t, false)
	r := runBuzzTypeCLI(t, plentyOfEverything, 12, "a cat", "--yes", "--no-wait")
	if r.err != nil {
		t.Fatalf("plain submit: %v\nstderr: %s", r.err, r.stderr)
	}
	if len(r.submitBodies) != 1 {
		t.Fatalf("submits = %d, want 1", len(r.submitBodies))
	}
	env, _ := submitEnvelope(t, r.submitBodies[0])
	if v, ok := env["buzzType"]; ok {
		t.Errorf("no --buzz-type, yet the envelope carries buzzType = %#v", v)
	}
	if strings.Contains(r.stderr, "Pay with") || strings.Contains(r.stdout, "Pay with") {
		t.Errorf("no --buzz-type, yet a Pay with line was printed:\n%s%s", r.stdout, r.stderr)
	}
}

// A value the server's resolver could never select is refused as a USAGE error
// before ANY request — because the server would not refuse it: it falls back to
// its default spend order, so a typo would be billed to blue.
//
// Each refusal row is paired with the accepted row through the same harness, so
// the zero request count is not a recorder wired to nothing — and so a build
// where the flag does not exist (cobra's "unknown flag", also a usage error)
// fails the accepted row rather than passing the refusals.
func TestGenerateBuzzType_UnknownValueIsRefusedBeforeAnyRequest(t *testing.T) {
	withStdinTTY(t, false)

	ok := runBuzzTypeCLI(t, plentyOfEverything, 12, "a cat", "--buzz-type", "yellow", "--yes", "--no-wait")
	if ok.err != nil || len(ok.submitBodies) != 1 {
		t.Fatalf("control: --buzz-type yellow must submit once, got err=%v submits=%d\nstderr: %s", ok.err, len(ok.submitBodies), ok.stderr)
	}

	for _, bad := range []string{"yelow", "red", "Yellow Buzz", "blue,yellow", " ", ""} {
		t.Run(bad, func(t *testing.T) {
			r := runBuzzTypeCLI(t, plentyOfEverything, 12, "a cat", "--buzz-type", bad, "--yes", "--no-wait")
			if !errors.Is(r.err, ErrUsage) {
				t.Fatalf("--buzz-type %q: want a usage error, got %v", bad, r.err)
			}
			if !strings.Contains(r.err.Error(), "--buzz-type") {
				t.Errorf("the refusal does not name the flag: %v", r.err)
			}
			if len(r.whatIfInputs) != 0 || len(r.submitBodies) != 0 {
				t.Errorf("--buzz-type %q: %d estimate(s) and %d submit(s) reached the server, want 0 and 0",
					bad, len(r.whatIfInputs), len(r.submitBodies))
			}
		})
	}
}

// --dry-run shows the chosen account, says the estimate was computed without
// it, and neither submits nor sends the key on the estimate.
func TestGenerateBuzzType_DryRunShowsTheAccountButDoesNotPriceWithIt(t *testing.T) {
	r := runBuzzTypeCLI(t, plentyOfEverything, 12, "a cat", "--buzz-type", "yellow", "--dry-run")
	if r.err != nil {
		t.Fatalf("--dry-run --buzz-type yellow: %v\nstderr: %s", r.err, r.stderr)
	}
	if len(r.submitBodies) != 0 {
		t.Fatalf("--dry-run submitted %d time(s)", len(r.submitBodies))
	}
	var payWith string
	for _, line := range strings.Split(r.stdout, "\n") {
		if strings.HasPrefix(line, "Pay with:") {
			payWith = strings.Join(strings.Fields(line), " ")
		}
	}
	if payWith != "Pay with: yellow Buzz" {
		t.Errorf("quote Pay with row = %q, want %q\nstdout:\n%s", payWith, "Pay with: yellow Buzz", r.stdout)
	}
	if !strings.Contains(r.stdout, "total  12") {
		t.Errorf("the estimate's own total is missing from the quote:\n%s", r.stdout)
	}
	if !strings.Contains(r.stderr, "--buzz-type yellow is sent with the submit, not with this estimate") {
		t.Errorf("the quote does not say the estimate was computed without the account choice:\n%s", r.stderr)
	}
	assertNoBuzzTypeOnEstimate(t, r)
}

// 🔴 With --buzz-type the balance check reads THAT account, not the total. The
// fixture is chosen so the two answers disagree: 1000 blue + 5 yellow covers a
// 12-Buzz job in total but not in yellow.
func TestGenerateBuzzType_BalanceCheckUsesTheChosenAccount(t *testing.T) {
	withStdinTTY(t, false)
	acct := map[string]int64{"blue": 1000, "yellow": 5, "green": 0}
	cases := []struct {
		name        string
		args        []string
		wantSubmits int
		wantShort   bool
	}{
		{"yellow cannot cover it", []string{"--buzz-type", "yellow"}, 0, true},
		{"green cannot cover it", []string{"--buzz-type", "green"}, 0, true},
		{"blue can", []string{"--buzz-type", "blue"}, 1, false},
		// Unchanged without the flag: the total covers it.
		{"no flag, total covers it", nil, 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"a cat", "--yes", "--no-wait"}, tc.args...)
			r := runBuzzTypeCLI(t, acct, 12, args...)
			if tc.wantShort {
				if !errors.Is(r.err, ErrInsufficientBuzz) {
					t.Fatalf("want ErrInsufficientBuzz, got %v", r.err)
				}
			} else if r.err != nil {
				t.Fatalf("this job is affordable from the chosen account and must not be refused: %v", r.err)
			}
			if len(r.submitBodies) != tc.wantSubmits {
				t.Errorf("submits = %d, want %d", len(r.submitBodies), tc.wantSubmits)
			}
		})
	}

	// The boundary, on the chosen account: exactly enough yellow is affordable.
	r := runBuzzTypeCLI(t, map[string]int64{"blue": 0, "yellow": 12, "green": 0}, 12, "a cat", "--buzz-type", "yellow", "--yes", "--no-wait")
	if r.err != nil || len(r.submitBodies) != 1 {
		t.Errorf("12 yellow vs a 12-Buzz job: want one submit, got err=%v submits=%d", r.err, len(r.submitBodies))
	}
}

// An --input file carrying buzzType is still refused (it is an envelope key),
// and the refusal now names the flag that does what the file was trying to do.
func TestGenerateBuzzType_InputFileRefusalPointsAtTheFlag(t *testing.T) {
	_, err := parseGraphInput([]byte(`{"workflow":"txt2img","prompt":"a cat","buzzType":"yellow"}`))
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("want the envelope-key usage refusal, got %v", err)
	}
	if !strings.Contains(err.Error(), "pass --buzz-type instead") {
		t.Errorf("the buzzType refusal does not point at --buzz-type: %v", err)
	}
	// Control: a refusal for a DIFFERENT envelope key does not grow the hint.
	_, err = parseGraphInput([]byte(`{"workflow":"txt2img","prompt":"a cat","civitaiTip":5}`))
	if err == nil || strings.Contains(err.Error(), "--buzz-type") {
		t.Errorf("a civitaiTip refusal must not mention --buzz-type: %v", err)
	}
}

// A lost submit reply prints a re-attach command. It must repeat --buzz-type:
// should the dedupe ever miss, the job it re-submits has to be paid from the
// account the user chose, not the server's default.
func TestGenerateBuzzType_ReattachCommandKeepsTheAccount(t *testing.T) {
	withStdinTTY(t, false)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"with the flag", []string{"--buzz-type", "yellow"}, `civitai generate "a cat" --buzz-type yellow --external-id `},
		// Control: without the flag the command is unchanged.
		{"without it", nil, `civitai generate "a cat" --external-id `},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &buzzTypeServer{account: plentyOfEverything, cost: 12, dropSubmit: true}
			r := runBuzzTypeCLIWith(t, s, append([]string{"a cat", "--yes", "--no-wait"}, tc.args...)...)
			if r.err == nil {
				t.Fatal("a dropped submit must fail")
			}
			if len(r.submitBodies) == 0 {
				t.Fatal("the submit never reached the server — the re-attach path did not run")
			}
			if !strings.Contains(r.stderr, tc.want) {
				t.Errorf("re-attach command missing %q:\n%s", tc.want, r.stderr)
			}
		})
	}
}
