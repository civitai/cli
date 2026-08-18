package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TESTS FOR THE READ HALF OF THE PROVENANCE STAMP (issue #411) — what
// `civitai app status` shows, and what `--json` carries.
//
// 🔴 THE TRI-STATE IS THE WHOLE THING, AND IT IS EASY TO LOSE IN A RENDERER.
// `null` is "nobody said", `false` is "a client looked and said clean", `true`
// is "a client said dirty". A renderer that prints nothing for the first two
// merges an UNKNOWN row into a CLEAN one, which is the `?? false` collapse
// arriving through the display layer instead of through the decoder. So the
// three rows below assert three DIFFERENT strings, and the `--json` case asserts
// the raw JSON text rather than a decoded Go value — a decode into a value type
// would flatten null to false before any assertion could see it.
//
// 🔴 AND NOTHING HERE MAY READ AS VERIFIED. The server stores an unverified
// client claim; the detail view says so in one sentence, and this file pins that
// the sentence is present whenever a commit is.

const (
	provSHAClean   = "1c4f8ab30d5e97624bd0af31e6c857920dbe4a68"
	provSHADirty   = "77b0e3d9142ca8e5630fb84c02d1975ae6cf3b20"
	provSHAUnknown = "acd51e6027f9b3184dc7a05e29b6f4d38017ce42"
)

// provRows is the fixture listing: four submissions covering every state the
// column can be in. The shas are pairwise distinct AND distinct from any
// constant this file asserts against, so a renderer that hardcoded one could not
// pass by luck.
func provRows() []map[string]any {
	return []map[string]any{
		{"id": "pubreq_4", "blockId": "with-clean", "version": "0.4.0", "status": "approved",
			"deployState": "live", "submittedAt": "2026-08-14T09:00:00.000Z", "liveUrl": "https://with-clean.civit.ai/",
			"sourceCommit": provSHAClean, "sourceDirty": false},
		{"id": "pubreq_3", "blockId": "with-dirty", "version": "0.3.0", "status": "approved",
			"deployState": "live", "submittedAt": "2026-08-13T09:00:00.000Z", "liveUrl": nil,
			"sourceCommit": provSHADirty, "sourceDirty": true},
		{"id": "pubreq_2", "blockId": "half-known", "version": "0.2.0", "status": "pending",
			"deployState": nil, "submittedAt": "2026-08-12T09:00:00.000Z", "liveUrl": nil,
			"sourceCommit": provSHAUnknown, "sourceDirty": nil},
		{"id": "pubreq_1", "blockId": "pre-feature", "version": "0.1.0", "status": "approved",
			"deployState": "live", "submittedAt": "2026-08-11T09:00:00.000Z", "liveUrl": nil},
	}
}

func provStatusEnv(t *testing.T, url string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-prov")
	t.Setenv("CIVITAI_BASE_URL", url)
}

// TestAppStatusTableShowsTheClaimedSource pins the SOURCE column, one distinct
// rendering per state.
func TestAppStatusTableShowsTheClaimedSource(t *testing.T) {
	srv := statusServer(t, map[string]any{"submissions": provRows()}, http.StatusOK, nil)
	defer srv.Close()
	provStatusEnv(t, srv.URL)

	out, _, err := run(t, "app", "status")
	if err != nil {
		t.Fatalf("app status: %v", err)
	}
	if !strings.Contains(out, "SOURCE") {
		t.Fatalf("the table has no SOURCE column, so the stamp is invisible where a reader looks for it:\n%s", out)
	}

	for _, want := range []struct{ row, cell, why string }{
		{"with-clean", provSHAClean[:7],
			"a clean build shows the abbreviated commit and no flag"},
		{"with-dirty", provSHADirty[:7] + " (dirty)",
			"a dirty build must be marked — that is the state #411 was filed about"},
		{"half-known", provSHAUnknown[:7] + " (dirty?)",
			"a commit whose dirtiness was never reported must not render like an asserted-clean row"},
		{"pre-feature", "-",
			"a row that claims nothing must read as UNKNOWN, never as a clean build"},
	} {
		line := lineContaining(out, want.row)
		if line == "" {
			t.Fatalf("no row for %q in:\n%s", want.row, out)
		}
		if !strings.Contains(line, want.cell) {
			t.Errorf("row %q renders %q, want it to carry %q — %s", want.row, line, want.cell, want.why)
		}
	}

	// The table abbreviates on purpose; a full sha would push the URL column off
	// an 80-column terminal. Assert the abbreviation is real, or "short sha" is
	// just a claim in a comment.
	if strings.Contains(out, provSHAClean) {
		t.Errorf("the table printed the FULL sha; it is supposed to abbreviate to %d characters "+
			"(the full value lives in the detail view and in --json):\n%s", shortSHALen, out)
	}
}

// TestAppStatusDetailShowsTheFullShaAndAttributesIt — the detail view is where
// someone goes for the value they will paste into `git show`, so it carries the
// whole forty characters, and the sentence saying who claimed them.
func TestAppStatusDetailShowsTheFullShaAndAttributesIt(t *testing.T) {
	for _, tc := range []struct {
		name, row string
		wantParts []string
	}{
		{
			name: "a dirty claim", row: "with-dirty",
			wantParts: []string{provSHADirty, "reported DIRTY", sourceClaimNote},
		},
		{
			name: "a clean claim", row: "with-clean",
			wantParts: []string{provSHAClean, "reported clean", sourceClaimNote},
		},
		{
			name: "a commit with no dirtiness reported", row: "half-known",
			wantParts: []string{provSHAUnknown, "not reported", sourceClaimNote},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := statusServer(t, map[string]any{"submissions": rowsFor(tc.row)}, http.StatusOK, nil)
			defer srv.Close()
			provStatusEnv(t, srv.URL)

			out, _, err := run(t, "app", "status", tc.row)
			if err != nil {
				t.Fatalf("app status %s: %v", tc.row, err)
			}
			for _, want := range tc.wantParts {
				if !strings.Contains(out, want) {
					t.Errorf("the detail view does not carry %q:\n%s", want, out)
				}
			}
		})
	}

	// A row that claims nothing must not print an empty "Source commit:" line,
	// and must not print the attribution sentence — there is nothing to
	// attribute, and a note under no fact implies the CLI checked something.
	t.Run("a pre-feature row says nothing at all", func(t *testing.T) {
		srv := statusServer(t, map[string]any{"submissions": rowsFor("pre-feature")}, http.StatusOK, nil)
		defer srv.Close()
		provStatusEnv(t, srv.URL)

		out, _, err := run(t, "app", "status", "pre-feature")
		if err != nil {
			t.Fatalf("app status: %v", err)
		}
		if strings.Contains(out, "Source commit") {
			t.Errorf("a submission that reported no commit printed a Source commit line:\n%s", out)
		}
		if strings.Contains(out, sourceClaimNote) {
			t.Errorf("the attribution note printed with nothing to attribute:\n%s", out)
		}
	})
}

func rowsFor(blockID string) []map[string]any {
	var out []map[string]any
	for _, r := range provRows() {
		if r["blockId"] == blockID {
			out = append(out, r)
		}
	}
	return out
}

// TestAppStatusJSONCarriesTheFullTriState is the scriptable contract.
//
// 🔴 IT ASSERTS THE RAW JSON TEXT. Decoding into a Go value type is exactly the
// collapse being guarded against: `null` and `false` both land on `false` and
// the assertion passes over a payload that has lost the distinction.
func TestAppStatusJSONCarriesTheFullTriState(t *testing.T) {
	srv := statusServer(t, map[string]any{"submissions": provRows()}, http.StatusOK, nil)
	defer srv.Close()
	provStatusEnv(t, srv.URL)

	out, _, err := run(t, "app", "status", "--json")
	if err != nil {
		t.Fatalf("app status --json: %v", err)
	}

	// The full sha, not the abbreviation a script would have to guess at.
	for _, sha := range []string{provSHAClean, provSHADirty, provSHAUnknown} {
		if !strings.Contains(out, sha) {
			t.Errorf("--json does not carry the FULL sha %q — a script comparing commits needs all forty:\n%s", sha, out)
		}
	}

	var doc struct {
		Submissions []map[string]json.RawMessage `json:"submissions"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("--json is not valid JSON: %v\n%s", err, out)
	}
	if len(doc.Submissions) != len(provRows()) {
		t.Fatalf("CONTROL failure: --json carried %d row(s), want %d", len(doc.Submissions), len(provRows()))
	}
	want := map[string]string{
		"with-clean":  "false",
		"with-dirty":  "true",
		"half-known":  "null",
		"pre-feature": "null",
	}
	seen := 0
	for _, row := range doc.Submissions {
		var id string
		if err := json.Unmarshal(row["blockId"], &id); err != nil {
			t.Fatal(err)
		}
		w, ok := want[id]
		if !ok {
			continue
		}
		seen++
		got := string(row["sourceDirty"])
		if got != w {
			t.Errorf("%s: sourceDirty is %q in --json, want %q.\n"+
				"null and false are DIFFERENT answers and both are scriptable: null means nobody reported, "+
				"false means a client asserted a clean tree.", id, got, w)
		}
	}
	if seen != len(want) {
		t.Fatalf("CONTROL failure: matched %d of %d rows by blockId, so the assertions above ran on the wrong payload",
			seen, len(want))
	}
}

// TestAppStatusJSONStaysColourFree keeps the new rendering inside
// internal/ui/CONVENTION.md rule 1: `--json` never passes through ui.
func TestAppStatusJSONStaysColourFree(t *testing.T) {
	const esc = "\x1b["
	// with-clean is the row that carries a liveUrl, and the live URL is the
	// coloured element — a row without one produces no ANSI at all and the
	// positive control below could not fire.
	srv := statusServer(t, map[string]any{"submissions": rowsFor("with-clean")}, http.StatusOK, nil)
	defer srv.Close()
	provStatusEnv(t, srv.URL)
	t.Setenv("CLICOLOR_FORCE", "1")

	human, _, err := run(t, "app", "status", "with-clean")
	if err != nil {
		t.Fatalf("app status: %v", err)
	}
	if !strings.Contains(human, esc) {
		t.Fatalf("POSITIVE CONTROL FAILED: CLICOLOR_FORCE produced no ANSI on the HUMAN path, so this "+
			"test cannot see a violation on the JSON path either:\n%q", human)
	}
	out, _, err := run(t, "app", "status", "with-clean", "--json")
	if err != nil {
		t.Fatalf("app status --json: %v", err)
	}
	if strings.Contains(out, esc) {
		t.Errorf("--json emitted an ANSI escape under CLICOLOR_FORCE:\n%q", out)
	}
}

// TestSourceLabelIsTheOneRenderer drives the column renderer directly, so the
// tri-state is pinned at the seam a future caller would inherit rather than only
// through the command that happens to call it today.
func TestSourceLabelIsTheOneRenderer(t *testing.T) {
	yes, no := true, false
	empty := ""
	sha := provSHAClean
	for _, tc := range []struct {
		name   string
		commit *string
		dirty  *bool
		want   string
	}{
		{"nothing reported", nil, nil, "-"},
		{"nothing reported, dirty somehow set", nil, &no, "-"},
		{"an empty commit is not a commit", &empty, &no, "-"},
		{"clean", &sha, &no, sha[:shortSHALen]},
		{"dirty", &sha, &yes, sha[:shortSHALen] + " (dirty)"},
		{"dirtiness unknown", &sha, nil, sha[:shortSHALen] + " (dirty?)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sourceLabel(tc.commit, tc.dirty); got != tc.want {
				t.Errorf("sourceLabel = %q, want %q", got, tc.want)
			}
		})
	}
}
