package genapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// TestNotFoundNamesTheWorkflowNotTheProcedure is civitai/cli#363 §3:
// `civitai workflows get <bad-id>` answered "no such generation procedure or
// resource (404)", which reads as "this CLI is calling an endpoint that does not
// exist" — the tool looks broken — rather than "the id you typed is not one of
// your workflows".
func TestNotFoundNamesTheWorkflowNotTheProcedure(t *testing.T) {
	srv := trpcErrJSON(t, 404, "Not Found", "NOT_FOUND")
	c := New(srv, "tok")

	calls := map[string]func() error{
		"workflows get":    func() error { _, _, err := c.GetWorkflow(context.Background(), "not-a-real-id-xyz"); return err },
		"workflows cancel": func() error { _, err := c.CancelWorkflow(context.Background(), "not-a-real-id-xyz"); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			if err == nil {
				t.Fatal("expected a 404 error")
			}
			msg := err.Error()
			if strings.Contains(msg, "procedure") {
				t.Errorf("the internal procedure noun leaked into a user-facing error: %s", msg)
			}
			for _, want := range []string{"no such workflow", "civitai workflows list"} {
				if !strings.Contains(msg, want) {
					t.Errorf("missing %q; got: %s", want, msg)
				}
			}
			// AGENTS.md item 7 — wording changes must not move the exit code.
			if !errors.Is(err, civitai.ErrNotFound) {
				t.Errorf("want not-found classification (exit 4), got %T: %v", err, err)
			}
		})
	}
}

// TestNotFoundOnAFixedRouteIsNotBlamedOnTheUser is the negative control: a proc
// that carries no user-supplied id 404s only if the ROUTE is gone, and telling
// that user to check an id they never passed would be a new wrong answer.
func TestNotFoundOnAFixedRouteIsNotBlamedOnTheUser(t *testing.T) {
	err := generateError("generateFromGraph", 404, []byte(`{"error":{"json":{"message":"Not Found"}}}`))
	msg := err.Error()
	if strings.Contains(msg, "no such workflow") || strings.Contains(msg, "civitai workflows list") {
		t.Errorf("a fixed-route 404 is not a bad workflow id: %s", msg)
	}
	if !strings.Contains(msg, "no such generation route") {
		t.Errorf("a fixed-route 404 must say the route is missing; got: %s", msg)
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("want not-found classification (exit 4), got %T: %v", err, err)
	}
}

// TestWorkflowNotFoundDoesNotBlameTheTypist: `getWorkflow` has TWO callers with
// opposite blame — `civitai workflows get <id>` (the user typed the id) and
// `generate`'s status poll (the CLI minted it; 404 is non-retryable, so the poll
// aborts to this message AFTER the charge moved). "Check the id you typed" is
// addressed to a user who typed none.
func TestWorkflowNotFoundDoesNotBlameTheTypist(t *testing.T) {
	err := generateError("getWorkflow", 404, []byte(`{"error":{"json":{"message":"Not Found"}}}`))
	msg := err.Error()
	for _, blaming := range []string{"check the id", "the id you", "you typed"} {
		if strings.Contains(strings.ToLower(msg), blaming) {
			t.Errorf("this message also reaches a poll whose id the CLI minted — %q blames the wrong party: %s", blaming, msg)
		}
	}
	// It still has to be actionable for the caller who DID type an id.
	if !strings.Contains(msg, "civitai workflows list") {
		t.Errorf("the lookup must still name where the readable ids are; got: %s", msg)
	}
}

// TestFixedRouteNotFoundMakesNoClaimAboutTheInput: "nothing you passed can cause
// this" is a claim about SERVER semantics the CLI cannot make. `generate --input
// <graph>` ships a user-authored graph with nothing checked locally (AGENTS.md
// item 13), so a bad reference inside it plausibly surfaces as a 404 — and this
// message would then send that user to `civitai upgrade` and a bug report
// instead of to their own file.
func TestFixedRouteNotFoundMakesNoClaimAboutTheInput(t *testing.T) {
	err := generateError("generateFromGraph", 404, []byte(`{"error":{"json":{"message":"Not Found"}}}`))
	msg := err.Error()
	if strings.Contains(msg, "nothing you passed") {
		t.Errorf("the CLI cannot rule the caller's own graph out: %s", msg)
	}
	if !strings.Contains(msg, "unlikely") {
		t.Errorf("the likely cause is still worth saying — say it as a likelihood; got: %s", msg)
	}
	// The actionable half must survive the softening.
	if !strings.Contains(msg, "civitai upgrade") {
		t.Errorf("the next command must survive; got: %s", msg)
	}
}

// TestWorkflowIDProcsCoverEveryIDCarryingCallSite is the ledger guard on
// workflowIDProcs.
//
// The failure it exists for is SILENT: add a new procedure that takes a workflow
// id, forget the map, and its 404 quietly falls back to the "the route is gone"
// message — nothing goes red. So this asserts the full set of generateError call
// sites in the package, failing when it GROWS or SHRINKS, and forces a decision
// about each one.
func TestWorkflowIDProcsCoverEveryIDCarryingCallSite(t *testing.T) {
	// Every proc label passed to generateError anywhere in this package, and
	// whether its request carries a user-supplied workflow id.
	want := map[string]bool{
		"getWorkflow":          true,
		"cancelWorkflow":       true,
		"queryGeneratedImages": false,
		"whatIfFromGraph":      false,
		"generateFromGraph":    false,
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`generateError\("([A-Za-z]+)"`)
	// Every invocation, literal label or not. The ledger regex above can only see
	// a STRING LITERAL: a call site passing a variable (`generateError(proc, …)`)
	// is invisible to it, so the ledger would stay green while an unclassified
	// proc reached the 404 arm. Counting both and comparing is what makes the
	// literal scan's blindness visible.
	anyCall := regexp.MustCompile(`generateError\(`)
	found := map[string]bool{}
	literalCalls, allCalls := 0, 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		text := string(src)
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			found[m[1]] = true
			literalCalls++
		}
		allCalls += len(anyCall.FindAllString(text, -1))
		// The definition itself matches `generateError(`; it is not a call site.
		allCalls -= strings.Count(text, "func generateError(")
	}

	// Positive control: the scan must actually find something, or an empty set
	// would satisfy every check below for the wrong reason.
	if len(found) == 0 {
		t.Fatal("the source scan found no generateError call sites — the pattern is wrong, not the code")
	}
	if literalCalls != allCalls {
		t.Errorf("%d generateError call sites but only %d pass a string literal — a call site passing a VARIABLE is invisible to this ledger, so it cannot vouch for the set; give it a literal label or teach the scan to resolve it", allCalls, literalCalls)
	}

	for proc := range found {
		if _, known := want[proc]; !known {
			t.Errorf("new generateError call site %q — decide whether its request carries a user-supplied workflow id and add it to workflowIDProcs (and to this ledger)", proc)
		}
	}
	for proc := range want {
		if !found[proc] {
			t.Errorf("generateError call site %q is gone — drop it from this ledger and from workflowIDProcs", proc)
		}
	}
	for proc, carriesID := range want {
		if workflowIDProcs[proc] != carriesID {
			t.Errorf("workflowIDProcs[%q] = %v, want %v", proc, workflowIDProcs[proc], carriesID)
		}
	}
	if len(workflowIDProcs) != countTrue(want) {
		keys := make([]string, 0, len(workflowIDProcs))
		for k := range workflowIDProcs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Errorf("workflowIDProcs has %d entries (%v); the ledger names %d id-carrying procs", len(workflowIDProcs), keys, countTrue(want))
	}
}

func countTrue(m map[string]bool) int {
	n := 0
	for _, v := range m {
		if v {
			n++
		}
	}
	return n
}
