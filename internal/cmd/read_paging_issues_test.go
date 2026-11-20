package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// Regression tests for five community-reported read/pagination defects:
// civitai/cli#932, #940, #942, #944 and #946. Each expectation below is a
// literal taken from the issue's reported contract, not derived from the code.

// recordReads stands up the read fixture, answers every request with body, and
// returns a pointer to the decoded query of every request it received.
func recordReads(t *testing.T, body string) *[]url.Values {
	t.Helper()
	var seen []url.Values
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Query())
		_, _ = w.Write([]byte(body))
	})
	return &seen
}

// hasLine (app_listing_rm_screenshot_test.go) compares WHOLE trimmed lines.

// ---------------------------------------------------------------------------
// #932 — workflows list: an EMPTY page that carries a continuation cursor must
// still print it in text mode.
// ---------------------------------------------------------------------------

func TestWorkflowsListEmptyPageKeepsNextCursor_932(t *testing.T) {
	// A page with a cursor in, no items, and a continuation out.
	c, out, _ := genCmd("")
	if err := runWorkflowsList(c, wfListDeps(`{"items":[],"nextCursor":"tail:cursor"}`, nil, nil, nil),
		workflowsListOpts{cursor: "start:opaque+/="}); err != nil {
		t.Fatalf("workflows list: %v", err)
	}
	if !hasLine(out.String(), "Next cursor: tail:cursor") {
		t.Errorf("an empty page with a continuation dropped its cursor line:\n%q", out.String())
	}
	if strings.Contains(out.String(), "No more workflows") {
		t.Errorf("an empty page that HAS a continuation must not claim there are no more:\n%q", out.String())
	}

	// The FIRST page can be the empty one too: it must not read as "you have
	// never generated anything" when the feed says there is more.
	c, out, _ = genCmd("")
	if err := runWorkflowsList(c, wfListDeps(`{"items":[],"nextCursor":"first:cursor"}`, nil, nil, nil),
		workflowsListOpts{}); err != nil {
		t.Fatalf("workflows list: %v", err)
	}
	if !hasLine(out.String(), "Next cursor: first:cursor") {
		t.Errorf("an empty first page with a continuation dropped its cursor line:\n%q", out.String())
	}
	if strings.Contains(out.String(), "No workflows yet") {
		t.Errorf("an empty first page with a continuation must not say 'No workflows yet':\n%q", out.String())
	}

	// Control: a genuinely terminal empty page keeps its message and prints no cursor.
	c, out, _ = genCmd("")
	if err := runWorkflowsList(c, wfListDeps(`{"items":[],"nextCursor":null}`, nil, nil, nil),
		workflowsListOpts{cursor: "last:cursor"}); err != nil {
		t.Fatalf("workflows list: %v", err)
	}
	if !hasLine(out.String(), "No more workflows on this page.") || strings.Contains(out.String(), "Next cursor:") {
		t.Errorf("terminal empty page changed:\n%q", out.String())
	}
}

// ---------------------------------------------------------------------------
// #940 — users get --json must fail not-found exactly like text mode: error,
// exit 4, EMPTY stdout. Exact matches still pass the API document through.
// ---------------------------------------------------------------------------

func TestUsersGetJSONNotFoundMatchesTextMode_940(t *testing.T) {
	misses := []struct {
		name, arg, body string
	}{
		{"fuzzy-only name", "sample_typo", `{"items":[{"id":123,"username":"sample_user"}]}`},
		{"empty id result", "2147483647", `{"items":[]}`},
	}
	for _, tc := range misses {
		t.Run(tc.name, func(t *testing.T) {
			recordReads(t, tc.body)
			out, _, err := run(t, "users", "get", tc.arg, "--anon", "--json")
			if !errors.Is(err, civitai.ErrNotFound) {
				t.Fatalf("users get %s --json: want a not-found error (exit 4), got %v", tc.arg, err)
			}
			if out != "" {
				t.Errorf("users get %s --json: stdout must be empty on a miss, got %q", tc.arg, out)
			}
		})
	}

	hits := []struct {
		name, arg, body, want string
	}{
		{"case-insensitive exact name", "SAMPLE_USER", `{"items":[{"id":123,"username":"sample_user"}]}`, "sample_user"},
		{"exact id", "123", `{"items":[{"id":123,"username":"id_owner"}]}`, "id_owner"},
	}
	for _, tc := range hits {
		t.Run(tc.name, func(t *testing.T) {
			recordReads(t, tc.body)
			out, _, err := run(t, "users", "get", tc.arg, "--anon", "--json")
			if err != nil {
				t.Fatalf("users get %s --json: %v", tc.arg, err)
			}
			var doc struct {
				Items []struct {
					ID       int    `json:"id"`
					Username string `json:"username"`
				} `json:"items"`
			}
			if jerr := json.Unmarshal([]byte(out), &doc); jerr != nil {
				t.Fatalf("stdout is not the API document: %v\n%s", jerr, out)
			}
			if len(doc.Items) != 1 || doc.Items[0].ID != 123 || doc.Items[0].Username != tc.want {
				t.Errorf("stdout document = %+v, want the single item 123/%s", doc.Items, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// #942 — the "next page:" hint must replay the flags the user passed, so
// pasting it continues the SAME search.
// ---------------------------------------------------------------------------

func TestModelsSearchNextPageHintReplaysFlags_942(t *testing.T) {
	const body = `{"items":[{"id":4384,"name":"DreamShaper","type":"Checkpoint","stats":{}}],` +
		`"metadata":{"nextCursor":"2"}}`

	recordReads(t, body)
	out, _, err := run(t, "models", "search", "--query", "dreamshaper", "--limit", "2", "--nsfw=false", "--anon")
	if err != nil {
		t.Fatalf("models search: %v", err)
	}
	const want = "next page: civitai models search --anon --limit=2 --nsfw=false --query=dreamshaper --cursor '2'"
	if !hasLine(out, want) {
		t.Errorf("next-page hint does not replay the search flags\nwant line: %s\ngot:\n%s", want, out)
	}

	// Values that need quoting, a repeatable flag, and a previous --cursor that
	// the new one must REPLACE rather than accompany.
	recordReads(t, `{"items":[],"metadata":{"nextCursor":"a b|c'd"}}`)
	out, _, err = run(t, "models", "search", "--query", "it's", "--sort", "Most Downloaded",
		"--base-model", "Pony", "--base-model", "SD 1.5", "--cursor", "old-cursor")
	if err != nil {
		t.Fatalf("models search: %v", err)
	}
	const want2 = `next page: civitai models search --base-model=Pony --base-model='SD 1.5' ` +
		`--query='it'\''s' --sort='Most Downloaded' --cursor 'a b|c'\''d'`
	if !hasLine(out, want2) {
		t.Errorf("next-page hint mis-quotes or keeps the old cursor\nwant line: %s\ngot:\n%s", want2, out)
	}
}

// The same footer serves every page-numbered search: the --page hint must keep
// the filters too, and replace (not repeat) a --page the user passed.
func TestTagsSearchNextPageHintReplaysFlags_942(t *testing.T) {
	recordReads(t, `{"items":[{"name":"anime","link":"l"}],"metadata":{"currentPage":1,"totalPages":3}}`)
	out, _, err := run(t, "tags", "search", "--query", "anime", "--limit", "2", "--page", "1")
	if err != nil {
		t.Fatalf("tags search: %v", err)
	}
	const want = "next page: civitai tags search --limit=2 --query=anime --page 2"
	if !hasLine(out, want) {
		t.Errorf("--page hint does not replay the search flags\nwant line: %s\ngot:\n%s", want, out)
	}
}

// ---------------------------------------------------------------------------
// #944 — --model-version-id honours --sort even alongside --model-id, so the
// "ignored" note must not fire for that combination.
// ---------------------------------------------------------------------------

func TestImagesSearchModelAndVersionSortNoNote_944(t *testing.T) {
	const note = "the API ignores --sort"
	seen := recordReads(t, `{"items":[],"metadata":{}}`)
	_, stderr, err := run(t, "images", "search", "--model-id", "827184", "--model-version-id", "1612720", "--sort", "Newest")
	if err != nil {
		t.Fatalf("images search: %v", err)
	}
	if strings.Contains(stderr, note) {
		t.Errorf("model+version+sort must not claim the sort was ignored: %q", stderr)
	}
	if len(*seen) != 1 || (*seen)[0].Get("sort") != "Newest" || (*seen)[0].Get("modelVersionId") != "1612720" {
		t.Errorf("request did not carry sort + modelVersionId: %v", *seen)
	}

	// Control: model-only + sort still warns.
	recordReads(t, `{"items":[],"metadata":{}}`)
	if _, stderr, err := run(t, "images", "search", "--model-id", "827184", "--sort", "Newest"); err != nil {
		t.Fatalf("images search: %v", err)
	} else if !strings.Contains(stderr, note) {
		t.Errorf("model-only + sort must still warn: %q", stderr)
	}

	// The help describes the combined case.
	out, _, err := run(t, "images", "search", "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	if !strings.Contains(out, "With --model-version-id also set, --sort is honoured.") {
		t.Errorf("images search --help does not describe --sort with both filters set:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// #946 — an explicit --page below the endpoint's minimum is a usage error (exit
// 2, no request); one at or above it is SENT, including tags/creators page 0;
// an omitted --page stays absent.
// ---------------------------------------------------------------------------

func TestSearchPageBounds_946(t *testing.T) {
	cases := []struct {
		cmd     string
		minPage int
	}{
		{"models", 1},
		{"images", 1},
		{"tags", 0},
		{"creators", 0},
	}
	for _, tc := range cases {
		for _, page := range []int{-1, 0, 1} {
			name := tc.cmd + "/page=" + strconv.Itoa(page)
			t.Run(name, func(t *testing.T) {
				seen := recordReads(t, `{"items":[],"metadata":{}}`)
				out, _, err := run(t, tc.cmd, "search", "--page="+strconv.Itoa(page), "--json")
				if page < tc.minPage {
					if !errors.Is(err, ErrUsage) {
						t.Fatalf("%s search --page=%d: want a usage error (exit 2), got %v", tc.cmd, page, err)
					}
					if !strings.Contains(err.Error(), "--page") {
						t.Errorf("usage error should name --page: %v", err)
					}
					if len(*seen) != 0 {
						t.Errorf("%s search --page=%d sent %d request(s); a refused flag must send none", tc.cmd, page, len(*seen))
					}
					if out != "" {
						t.Errorf("stdout must be empty on a usage error, got %q", out)
					}
					return
				}
				if err != nil {
					t.Fatalf("%s search --page=%d: %v", tc.cmd, page, err)
				}
				if len(*seen) != 1 || !(*seen)[0].Has("page") || (*seen)[0].Get("page") != strconv.Itoa(page) {
					t.Errorf("%s search --page=%d: want page=%d on the wire, got %v", tc.cmd, page, page, *seen)
				}
			})
		}
		t.Run(tc.cmd+"/omitted", func(t *testing.T) {
			seen := recordReads(t, `{"items":[],"metadata":{}}`)
			if _, _, err := run(t, tc.cmd, "search", "--json"); err != nil {
				t.Fatalf("%s search: %v", tc.cmd, err)
			}
			if len(*seen) != 1 || (*seen)[0].Has("page") {
				t.Errorf("%s search with no --page must not send page, got %v", tc.cmd, *seen)
			}
		})
	}
}
