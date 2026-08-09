package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// ----------------------------------------------------------------------------
// Node discovery
// ----------------------------------------------------------------------------

// readAPINode is one node of the public-read-API command group: either a LEAF
// (a command that actually calls the read API) or the GROUP it hangs under.
type readAPINode struct {
	path string // e.g. "models search"
	cmd  *cobra.Command
	leaf bool
}

// readAPINodes returns every node of the public read API group, discovered by
// WALKING the real command tree.
//
// 🔴 THE MEMBERSHIP TEST IS STRUCTURAL, NOT A LIST OF NAMES. A leaf is a command
// carrying BOTH the --json and --anon flags, which is exactly what bindReadFlags
// attaches and nothing else in this CLI has: `civitai download` has --anon but
// no --json, and every other --json (app status, buzz, generate, workflows …)
// comes without --anon. So a new read command is picked up here the moment it is
// wired, and fails the completeness guard below until it is documented — which a
// hardcoded list cannot do, because a command missing from the list is
// indistinguishable from a command that passed.
//
// The groups are then the leaves' PARENTS, again derived rather than named. The
// root is asserted not to be one: a read leaf hanging directly off the root
// would mean this group's shape changed, and the guards below assume a group
// page exists to carry the shared prose.
func readAPINodes(t *testing.T) []readAPINode {
	t.Helper()
	var (
		out    []readAPINode
		groups = map[*cobra.Command]bool{}
		root   = NewRootCmd()
	)
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Flags().Lookup("json") != nil && c.Flags().Lookup("anon") != nil {
			out = append(out, readAPINode{path: cmdPath(c), cmd: c, leaf: true})
			if p := c.Parent(); p != nil {
				if p == root {
					t.Fatalf("read leaf %q hangs off the root — this group is expected to be "+
						"grouped, and the shared help lives on the group page", c.Name())
				}
				groups[p] = true
			} else {
				t.Fatalf("read leaf %q has no parent", c.Name())
			}
		}
		for _, s := range c.Commands() {
			walk(s)
		}
	}
	walk(root)
	for g := range groups {
		out = append(out, readAPINode{path: cmdPath(g), cmd: g})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

// cmdPath renders a command's path without the "civitai " root prefix.
func cmdPath(c *cobra.Command) string {
	return strings.TrimPrefix(c.CommandPath(), c.Root().Name()+" ")
}

// The floors. They are asserted, not assumed: a walk that matched nothing would
// otherwise report a serene pass over an empty set, and every guard below
// iterates the same slice.
const (
	wantReadAPILeaves = 13
	wantReadAPIGroups = 8
	wantReadAPINodes  = wantReadAPILeaves + wantReadAPIGroups
)

func TestReadAPINodeWalkFindsTheWholeGroup(t *testing.T) {
	nodes := readAPINodes(t)
	var leaves, groups int
	for _, n := range nodes {
		if n.leaf {
			leaves++
		} else {
			groups++
		}
	}
	if leaves != wantReadAPILeaves || groups != wantReadAPIGroups {
		var got []string
		for _, n := range nodes {
			kind := "group"
			if n.leaf {
				kind = "leaf"
			}
			got = append(got, kind+" "+n.path)
		}
		t.Fatalf("walked %d leaves / %d groups, want %d / %d — adding a read command means "+
			"documenting it, so move these floors deliberately.\nfound:\n  %s",
			leaves, groups, wantReadAPILeaves, wantReadAPIGroups, strings.Join(got, "\n  "))
	}
}

// ----------------------------------------------------------------------------
// Completeness + budget (the pilot's contract, applied to this group)
// ----------------------------------------------------------------------------

// TestReadAPIHelpBodiesAreComplete: every node carries an authored Long, Short
// and Example.
//
// Five of these commands (collections get, creators search, model-versions get,
// models get, tags search) shipped with NO Long at all — cobra falls back to
// Short, so `--help` printed one line, and the docs generator, which parses that
// same help text, published one line too.
func TestReadAPIHelpBodiesAreComplete(t *testing.T) {
	nodes := readAPINodes(t)
	if len(nodes) != wantReadAPINodes {
		t.Fatalf("walked %d nodes, want %d", len(nodes), wantReadAPINodes)
	}
	for _, n := range nodes {
		t.Run(n.path, func(t *testing.T) {
			if strings.TrimSpace(n.cmd.Long) == "" {
				t.Errorf("`%s` has no Long — `--help` will print only its Short (%q), and the "+
					"docs generator publishes exactly what `--help` prints", n.path, n.cmd.Short)
			}
			if strings.TrimSpace(n.cmd.Short) == "" {
				t.Errorf("`%s` has no Short", n.path)
			}
			if strings.TrimSpace(n.cmd.Example) == "" {
				t.Errorf("`%s` has no Example", n.path)
			}
		})
	}
}

// TestReadAPIHelpStaysWithinTheBudget applies the pilot's ceiling: 1400 runes
// per body, 80 columns per line.
//
// 🔴 RUNES, NOT BYTES, in both measurements. These bodies are full of em-dashes
// and en-dashes (3 bytes, one column each), so a byte count reports a 79-column
// line as 81 and reddens prose that renders fine.
func TestReadAPIHelpStaysWithinTheBudget(t *testing.T) {
	nodes := readAPINodes(t)
	var checked int
	for _, n := range nodes {
		body := strings.TrimSpace(n.cmd.Long)
		if body == "" {
			continue // TestReadAPIHelpBodiesAreComplete owns that failure
		}
		checked++
		if got := utf8.RuneCountInString(body); got > helpBodyBudget {
			t.Errorf("`%s` Long is %d chars, over the %d budget — prose that does not fit a "+
				"screen belongs in the guide, not in `--help`", n.path, got, helpBodyBudget)
		}
		for _, line := range strings.Split(body, "\n") {
			if got := utf8.RuneCountInString(line); got > 80 {
				t.Errorf("`%s` Long has an %d-column line (>80) — cobra does not wrap Long, so "+
					"it will hard-wrap in a standard terminal:\n%s", n.path, got, line)
			}
		}
	}
	if checked != wantReadAPINodes {
		t.Fatalf("only %d bodies were measured, want %d — a body that is empty is not a body "+
			"that is within budget", checked, wantReadAPINodes)
	}
}

// ----------------------------------------------------------------------------
// The derived facts
// ----------------------------------------------------------------------------

// readAPILimitLedger maps each list command to the ceiling its RunE enforces.
//
// The ledger is not trusted. TestReadAPIHelpQuotesTheEnforcedLimit checks it in
// BOTH directions: its key set must equal the set of read leaves that actually
// bind a --limit flag (so a new list command cannot slip past, and a removed one
// cannot leave a stale row looking like coverage), and every row's ceiling is
// exercised against the real command — accepted at the ceiling, refused one
// above it. Only then is the help required to quote it.
var readAPILimitLedger = map[string]int{
	"models search":      modelsLimitMax,
	"images search":      imagesLimitMax,
	"articles search":    articlesLimitMax,
	"collections search": collectionsLimitMax,
	"tags search":        tagsLimitMax,
	"creators search":    creatorsLimitMax,
}

// TestReadAPIHelpQuotesTheEnforcedLimit pins the coupling between what `--help`
// PROMISES about --limit and what the command actually ENFORCES.
//
// 🔴 THE POINT IS THE DIRECTION OF DERIVATION. The expected string is computed
// from the endpoint's own ceiling constant through the SAME limitRule the body
// interpolates, and that constant is independently proven to be the enforced
// boundary by driving the real command at max and max+1. So moving a ceiling
// without moving the help reddens this test whether the body computes its number
// or has a stale literal typed into it, and mis-wiring one command's help to
// another's constant reddens it too — the cross-endpoint assertion below is what
// catches that, and it is not redundant with the per-command one.
func TestReadAPIHelpQuotesTheEnforcedLimit(t *testing.T) {
	nodes := readAPINodes(t)

	// (1) The ledger's key set must be exactly the leaves that bind --limit.
	haveLimit := map[string]bool{}
	for _, n := range nodes {
		if n.leaf && n.cmd.Flags().Lookup("limit") != nil {
			haveLimit[n.path] = true
		}
	}
	for path := range haveLimit {
		if _, ok := readAPILimitLedger[path]; !ok {
			t.Errorf("`%s` binds --limit but has no ledger row — a list command whose ceiling "+
				"is unledgered is a ceiling nothing checks", path)
		}
	}
	for path := range readAPILimitLedger {
		if !haveLimit[path] {
			t.Errorf("ledger row %q no longer binds --limit — a stale row is a false map, not "+
				"harmless", path)
		}
	}
	if len(haveLimit) != len(readAPILimitLedger) || len(haveLimit) == 0 {
		t.Fatalf("ledger/tree disagree: %d commands bind --limit, %d rows ledgered",
			len(haveLimit), len(readAPILimitLedger))
	}

	byPath := map[string]*cobra.Command{}
	for _, n := range nodes {
		byPath[n.path] = n.cmd
	}

	// (2) Per command: the ledgered ceiling is the ENFORCED one, and the help
	// quotes it.
	for path, max := range readAPILimitLedger {
		t.Run(path, func(t *testing.T) {
			args := strings.Fields(path)

			// Refused one above the ceiling, and the request never goes out.
			hits := setupReadAPIServer(t)
			_, _, err := run(t, append(append([]string{}, args...), "--limit", strconv.Itoa(max+1))...)
			// 🔴 Classification by errors.Is, never by message text (AGENTS item
			// 7): a test asserting on the wording says nothing about the exit code
			// the README promises for a bad flag value.
			if !errors.Is(err, ErrUsage) && !errors.Is(err, civitai.ErrBadRequest) {
				t.Fatalf("`%s --limit %d` should be refused as a usage mistake, got %v",
					path, max+1, err)
			}
			if n := hits.count(); n != 0 {
				t.Errorf("`%s --limit %d` was refused but still made %d request(s) — the ceiling "+
					"must be enforced before the network", path, max+1, n)
			}

			// Accepted AT the ceiling, and the value reaches the wire.
			hits = setupReadAPIServer(t)
			if _, _, err := run(t, append(append([]string{}, args...), "--limit", strconv.Itoa(max))...); err != nil {
				t.Fatalf("`%s --limit %d` should be accepted, got %v", path, max, err)
			}
			if got := hits.lastQuery().Get("limit"); got != strconv.Itoa(max) {
				t.Errorf("`%s --limit %d` sent limit=%q — the ledgered ceiling is not the value "+
					"the command actually passes", path, max, got)
			}

			// Only now is the help held to it.
			want := limitRule(max)
			if !strings.Contains(byPath[path].Long, want) {
				t.Errorf("`%s` Long does not state the limit it enforces.\nwant: %q\nLong:\n%s",
					path, want, byPath[path].Long)
			}
		})
	}

	// (3) 🔴 CROSS-ENDPOINT: a body must not quote a DIFFERENT ceiling.
	//
	// Three of these endpoints cap at 100 and three at 200, so `limitRule(100)`
	// and `limitRule(200)` are each shared by three commands — which means a
	// same-ceiling swap (models <-> articles) is INVISIBLE here and is declared
	// an equivalent mutant in the file header. What this catches is the swap that
	// MATTERS: a 100-capped body quoting 200 (or the reverse), which is what a
	// copy-paste between two families produces.
	for path, max := range readAPILimitLedger {
		for otherPath, otherMax := range readAPILimitLedger {
			if otherMax == max {
				continue
			}
			if strings.Contains(byPath[path].Long, limitRule(otherMax)) {
				t.Errorf("`%s` (ceiling %d) quotes `%s`'s ceiling %d — this is what a copy-paste "+
					"between two families looks like", path, max, otherPath, otherMax)
			}
		}
	}
}

// TestReadAPIHelpDescribesItsOwnPagingShape pins the other fact a reader acts
// on, and it is the one a copy-paste gets wrong silently: these six list
// commands do NOT page the same way. models/images take both --page and
// --cursor; articles/collections are cursor-only (a keyset feed, no --page);
// tags/creators are page-only (a classic page envelope, no cursor).
//
// The expectation is read off the command's OWN FLAGS, never a table, so the
// guard cannot drift from the tree. Absence is required to be DOCUMENTED rather
// than merely unmentioned ("there is no --page here"), because a body that is
// silent about --page is indistinguishable from a body that forgot it — and the
// two phrasings are kept distinguishable so the present case cannot satisfy the
// absent one.
func TestReadAPIHelpDescribesItsOwnPagingShape(t *testing.T) {
	var checked int
	for _, n := range readAPINodes(t) {
		if !n.leaf || n.cmd.Flags().Lookup("limit") == nil {
			continue
		}
		checked++
		t.Run(n.path, func(t *testing.T) {
			for _, flag := range []string{"page", "cursor"} {
				var (
					has     = n.cmd.Flags().Lookup(flag) != nil
					long    = n.cmd.Long
					mention = "--" + flag
					absent  = "no --" + flag
				)
				switch {
				case has && !strings.Contains(long, mention):
					t.Errorf("`%s` accepts %s but its Long never names it", n.path, mention)
				case has && strings.Contains(long, absent):
					t.Errorf("`%s` accepts %s but its Long says %q", n.path, mention, absent)
				case !has && !strings.Contains(long, absent):
					t.Errorf("`%s` has no %s flag and its Long does not say so — %q must be "+
						"stated, or a reader cannot tell a missing paging mode from a missing "+
						"sentence", n.path, mention, absent)
				case !has && strings.Count(long, mention) != strings.Count(long, absent):
					// 🔴 STATING THE ABSENCE IS NOT ENOUGH — the body must not ALSO
					// advertise the flag somewhere else. Measured: appending "Use
					// --cursor for deep paging." to `tags search` (which has no
					// cursor) SURVIVED the three cases above, because the
					// documented-absence sentence was still sitting there. Every
					// occurrence of the flag must therefore be an occurrence of the
					// "no --x" phrase.
					t.Errorf("`%s` has no %s flag but its Long mentions %s outside the %q "+
						"sentence (%d mentions, %d of them the documented absence) — a body "+
						"that documents the absence AND advertises the flag is worse than "+
						"either alone", n.path, mention, mention, absent,
						strings.Count(long, mention), strings.Count(long, absent))
				}
			}
		})
	}
	if checked != len(readAPILimitLedger) {
		t.Fatalf("checked %d list commands, want %d", checked, len(readAPILimitLedger))
	}
}

// TestReadAPIHelpNamesItsAliases: a node with aliases must name every one of
// them, and the text is rendered FROM cmd.Aliases (see newModelVersionsCmd) so a
// dropped alias cannot leave the help advertising a spelling the tree no longer
// answers to.
//
// The count floor matters: exactly one node in this group has aliases today, and
// a guard that iterated an empty set would pass silently if that changed.
func TestReadAPIHelpNamesItsAliases(t *testing.T) {
	var withAliases int
	for _, n := range readAPINodes(t) {
		if len(n.cmd.Aliases) == 0 {
			continue
		}
		withAliases++
		for _, a := range n.cmd.Aliases {
			if !strings.Contains(n.cmd.Long, a) {
				t.Errorf("`%s` answers to the alias %q but its Long never mentions it", n.path, a)
			}
		}
	}
	if withAliases != 1 {
		t.Fatalf("%d read-API nodes carry aliases, want 1 — if that changed, this guard's "+
			"positive control did too", withAliases)
	}
}

// TestReadAPIHelpStatesItsAuthRequirement is the guard for the claim this group
// is most likely to get WRONG, because the CLI's other list commands got it
// wrong: PR #268 corrected a blanket "reads are anonymous — no login needed"
// that was FALSE for `app list` / `app view`.
//
// Two halves, and neither substitutes for the other:
//
//   - every node states the requirement, from the ONE shared constant (so eight
//     families cannot end up with eight differently-worded auth claims); and
//   - TestReadAPICommandsRunAnonymously proves the claim behaviourally.
//
// A body that merely contains the word "anonymous" would satisfy neither.
func TestReadAPIHelpStatesItsAuthRequirement(t *testing.T) {
	var groups, leaves int
	for _, n := range readAPINodes(t) {
		want, label := readAnonNote, "readAnonNote"
		if n.leaf {
			want, label = readAnonShort, "readAnonShort"
		}
		if !strings.Contains(n.cmd.Long, want) {
			t.Errorf("`%s` Long does not carry %s — the auth requirement is stated once, in "+
				"read_help.go, precisely so it cannot be re-worded per family", n.path, label)
			continue
		}
		if n.leaf {
			leaves++
		} else {
			groups++
		}
	}
	if groups != wantReadAPIGroups || leaves != wantReadAPILeaves {
		t.Fatalf("auth note found on %d groups / %d leaves, want %d / %d",
			groups, leaves, wantReadAPIGroups, wantReadAPILeaves)
	}
	// The two forms must stay distinguishable, or the group/leaf split above is
	// satisfiable by one string appearing everywhere.
	if strings.Contains(readAnonNote, readAnonShort) || strings.Contains(readAnonShort, readAnonNote) {
		t.Fatal("readAnonNote and readAnonShort must not contain one another — the per-form " +
			"assertions above would stop distinguishing them")
	}
}

// ----------------------------------------------------------------------------
// The behavioural half of the auth claim
// ----------------------------------------------------------------------------

// readAPIInvocations is the argv for every read leaf, used to exercise the whole
// group against a fake server. Its key set is asserted equal to the walked leaf
// set, so a new leaf cannot be silently left unexercised.
var readAPIInvocations = map[string][]string{
	"models search":          {"models", "search"},
	"models get":             {"models", "get", "4384"},
	"model-versions get":     {"model-versions", "get", "128713"},
	"model-versions by-hash": {"model-versions", "by-hash", "5D8D26E2A6"},
	"images search":          {"images", "search"},
	"images get":             {"images", "get", "136456589"},
	"articles search":        {"articles", "search"},
	"articles get":           {"articles", "get", "1234"},
	"collections search":     {"collections", "search"},
	"collections get":        {"collections", "get", "1234"},
	"tags search":            {"tags", "search"},
	"creators search":        {"creators", "search"},
	"users get":              {"users", "get", "alice"},
}

// TestReadAPICommandsRunAnonymously is what makes "no login is needed" evidence
// rather than a sentence.
//
// With NO token configured, every leaf in the group must complete successfully
// and send NO Authorization header. The request COUNT is asserted too: a leaf
// that made zero requests would satisfy "sent no Authorization header" while
// proving nothing at all — the reassuring-zero shape.
//
// It is deliberately not a claim about the SERVER (this repo cannot make one).
// It is the claim the help actually makes: the CLI does not require a credential
// on these routes and does not invent one.
func TestReadAPICommandsRunAnonymously(t *testing.T) {
	leaves := map[string]bool{}
	for _, n := range readAPINodes(t) {
		if n.leaf {
			leaves[n.path] = true
		}
	}
	for path := range leaves {
		if _, ok := readAPIInvocations[path]; !ok {
			t.Fatalf("read leaf %q has no invocation row — it would go unexercised", path)
		}
	}
	for path := range readAPIInvocations {
		if !leaves[path] {
			t.Fatalf("invocation row %q is not a read leaf any more", path)
		}
	}
	if len(leaves) != wantReadAPILeaves {
		t.Fatalf("walked %d leaves, want %d", len(leaves), wantReadAPILeaves)
	}

	for path, args := range readAPIInvocations {
		t.Run(path, func(t *testing.T) {
			hits := setupReadAPIServer(t)
			if _, _, err := run(t, args...); err != nil {
				t.Fatalf("`%s` failed with no token configured: %v", path, err)
			}
			if n := hits.count(); n < 1 {
				t.Fatalf("`%s` made no request — the header assertion below would be vacuous", path)
			}
			for _, auth := range hits.authHeaders() {
				if auth != "" {
					t.Errorf("`%s` sent Authorization %q with no token configured", path, auth)
				}
			}
		})
	}

	// Positive control for the header assertion itself: with a token configured
	// the SAME harness must observe a non-empty Authorization, or the loop above
	// is a probe wired to nothing.
	t.Run("control/token is observable", func(t *testing.T) {
		hits := setupReadAPIServer(t)
		t.Setenv("CIVITAI_TOKEN", "test-token")
		if _, _, err := run(t, "models", "search"); err != nil {
			t.Fatalf("models search with a token: %v", err)
		}
		got := hits.authHeaders()
		if len(got) == 0 || got[0] == "" {
			t.Fatalf("harness never observed an Authorization header even WITH a token — the "+
				"anonymous assertions above prove nothing (headers seen: %v)", got)
		}
	})

	// Negative control for --anon: a configured token must be dropped.
	t.Run("control/--anon drops a configured token", func(t *testing.T) {
		hits := setupReadAPIServer(t)
		t.Setenv("CIVITAI_TOKEN", "test-token")
		if _, _, err := run(t, "models", "search", "--anon"); err != nil {
			t.Fatalf("models search --anon: %v", err)
		}
		for _, auth := range hits.authHeaders() {
			if auth != "" {
				t.Errorf("--anon still sent Authorization %q", auth)
			}
		}
	})
}

// ----------------------------------------------------------------------------
// Harness
// ----------------------------------------------------------------------------

// readAPIRecorder records what the CLI actually sent.
type readAPIRecorder struct {
	mu      sync.Mutex
	queries []url.Values
	auths   []string
}

func (r *readAPIRecorder) record(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queries = append(r.queries, req.URL.Query())
	r.auths = append(r.auths, req.Header.Get("Authorization"))
}

func (r *readAPIRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.queries)
}

func (r *readAPIRecorder) authHeaders() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.auths...)
}

func (r *readAPIRecorder) lastQuery() url.Values {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.queries) == 0 {
		return url.Values{}
	}
	return r.queries[len(r.queries)-1]
}

// setupReadAPIServer points the CLI at a fake /api/v1 that answers every read
// route this group uses, with no token configured and the update check off.
//
// The bodies are the minimum each renderer needs to reach a clean exit — a
// single item where an empty page would be reported as not-found (`images get`,
// `users get`), an empty page otherwise.
func setupReadAPIServer(t *testing.T) *readAPIRecorder {
	t.Helper()
	rec := &readAPIRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(readAPIBodyFor(r.URL.Path, r.URL.Query())))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "1")
	return rec
}

func readAPIBodyFor(path string, q url.Values) string {
	switch {
	case path == "/api/v1/users":
		// `users get alice` requires an EXACT username match, so the fixture has
		// to answer with the name that was asked for.
		name := "alice"
		if v := q.Get("query"); v != "" {
			name = v
		}
		b, _ := json.Marshal(map[string]any{
			"items": []map[string]any{{"id": 5, "username": name, "image": "https://img/u"}},
		})
		return string(b)
	case path == "/api/v1/images":
		// `images get` reports an empty page as not-found, so this must be a hit.
		return `{"items":[{"id":136456589,"url":"https://img/1","width":8,"height":8,
			"nsfwLevel":"None","username":"alice","meta":null,"stats":{}}],"metadata":{}}`
	case strings.HasPrefix(path, "/api/v1/model-versions/"):
		return `{"id":128713,"modelId":4384,"name":"v1","baseModel":"SD 1.5","files":[]}`
	case strings.HasPrefix(path, "/api/v1/models/"):
		return `{"id":4384,"name":"m","type":"Checkpoint","modelVersions":[]}`
	case strings.HasPrefix(path, "/api/v1/articles/"):
		return `{"id":1234,"title":"a","content":"<p>hi</p>"}`
	case strings.HasPrefix(path, "/api/v1/collections/"):
		return `{"id":1234,"name":"c","type":"Model","read":"Public","isPublic":true}`
	default:
		return `{"items":[],"metadata":{}}`
	}
}
