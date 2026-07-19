package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The collections endpoint only supports cursor pagination for the default
// (Newest) sort. For any other accepted sort (currently just "Most Followers")
// the API returns a nextCursor that it then rejects — a dead cursor. These
// tests pin the CLI behavior: a working next-page hint for the pageable sorts,
// and a stderr note (no misleading hint, exit 0) for the non-pageable ones.

// collectionsCursorServer stands up an httptest collections endpoint that
// always returns one item plus the given nextCursor (raw JSON literal, so
// callers can pass a numeric `3`, a quoted `"3"`, or `null` for "no cursor").
func collectionsCursorServer(t *testing.T, nextCursor string) {
	t.Helper()
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/collections" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		body := `{"items":[{"id":3,"name":"Anime Picks","type":"Image",` +
			`"read":"Public","isPublic":true,"itemCount":25,` +
			`"user":{"id":9,"username":"carol"}}],` +
			`"metadata":{"nextCursor":` + nextCursor + `}}`
		_, _ = w.Write([]byte(body))
	})
}

const noteFragment = "cursor pagination isn't available"

// Newest sort with a server nextCursor → the working next-page hint IS printed
// on stdout, and no suppression note is emitted.
func TestCollectionsSearchNewestPrintsCursorHint(t *testing.T) {
	collectionsCursorServer(t, `3`)
	out, errOut, err := run(t, "collections", "search", "--sort", "Newest", "--limit", "1")
	if err != nil {
		t.Fatalf("collections search --sort Newest: %v", err)
	}
	if !strings.Contains(out, "next cursor: 3") {
		t.Errorf("Newest should print the next-cursor footer: %s", out)
	}
	if !strings.Contains(out, "next page: civitai collections search --cursor '3'") {
		t.Errorf("Newest should print the working next-page hint: %s", out)
	}
	if strings.Contains(errOut, noteFragment) {
		t.Errorf("Newest must NOT emit the suppression note: %s", errOut)
	}
}

// "Most Followers" with a server nextCursor → the hint is NOT printed, the
// stderr note IS printed, and the command exits 0.
func TestCollectionsSearchMostFollowersSuppressesHint(t *testing.T) {
	collectionsCursorServer(t, `6686272`)
	out, errOut, err := run(t, "collections", "search", "--sort", "Most Followers", "--limit", "1")
	if err != nil {
		t.Fatalf("collections search --sort Most Followers should exit 0, got: %v", err)
	}
	// Items still render.
	if !strings.Contains(out, "Anime Picks") {
		t.Errorf("first page items should still be shown: %s", out)
	}
	// No cursor footer or next-page hint on stdout — the cursor is dead.
	if strings.Contains(out, "next cursor") || strings.Contains(out, "next page") {
		t.Errorf("Most Followers must NOT print a next-cursor/next-page hint: %s", out)
	}
	// The note is on stderr and names the sort.
	if !strings.Contains(errOut, noteFragment) {
		t.Errorf("Most Followers should emit the suppression note on stderr: %s", errOut)
	}
	if !strings.Contains(errOut, `--sort "Most Followers"`) {
		t.Errorf("note should name the offending sort: %s", errOut)
	}
	if !strings.Contains(errOut, "--sort Newest for deep paging") {
		t.Errorf("note should point at --sort Newest: %s", errOut)
	}
}

// Default (no --sort) behaves as the cursor-pageable (Newest) case: working
// hint, no spurious note.
func TestCollectionsSearchDefaultSortPageable(t *testing.T) {
	collectionsCursorServer(t, `17329876`)
	out, errOut, err := run(t, "collections", "search", "--limit", "1")
	if err != nil {
		t.Fatalf("collections search (default sort): %v", err)
	}
	if !strings.Contains(out, "next page: civitai collections search --cursor '17329876'") {
		t.Errorf("default sort should print the working next-page hint: %s", out)
	}
	if strings.Contains(errOut, noteFragment) {
		t.Errorf("default sort must NOT emit the suppression note: %s", errOut)
	}
}

// --json for a non-pageable sort → stdout is pure valid JSON carrying the raw
// nextCursor, and the note appears only on stderr (never on stdout).
func TestCollectionsSearchMostFollowersJSONStdoutClean(t *testing.T) {
	collectionsCursorServer(t, `6686272`)
	out, errOut, err := run(t, "collections", "search", "--sort", "Most Followers", "--limit", "1", "--json")
	if err != nil {
		t.Fatalf("collections search --json: %v", err)
	}
	// stdout must be valid JSON (the raw body, untouched).
	if !json.Valid([]byte(out)) {
		t.Errorf("--json stdout must be valid JSON, got: %s", out)
	}
	// The raw nextCursor is still present in the body — --json is untouched.
	if !strings.Contains(out, `"nextCursor"`) || !strings.Contains(out, "6686272") {
		t.Errorf("--json body should still carry the raw nextCursor: %s", out)
	}
	// The human note must never leak onto stdout.
	if strings.Contains(out, noteFragment) {
		t.Errorf("--json stdout must not contain the note: %s", out)
	}
	// But the note is still surfaced on stderr.
	if !strings.Contains(errOut, noteFragment) {
		t.Errorf("--json should still emit the note on stderr: %s", errOut)
	}
}

// A non-pageable sort with NO nextCursor from the server (a genuine last page)
// → no hint AND no note: there is nothing to page, so nothing to warn about.
func TestCollectionsSearchMostFollowersNoCursorNoNote(t *testing.T) {
	collectionsCursorServer(t, `null`)
	out, errOut, err := run(t, "collections", "search", "--sort", "Most Followers", "--limit", "1")
	if err != nil {
		t.Fatalf("collections search: %v", err)
	}
	if strings.Contains(out, "next cursor") || strings.Contains(out, "next page") {
		t.Errorf("no cursor → no hint: %s", out)
	}
	if strings.Contains(errOut, noteFragment) {
		t.Errorf("no cursor → no note (nothing to page): %s", errOut)
	}
}

// Newest with NO nextCursor (last page) → no hint, no note either.
func TestCollectionsSearchNewestNoCursorNoFooterHint(t *testing.T) {
	collectionsCursorServer(t, `null`)
	out, errOut, err := run(t, "collections", "search", "--sort", "Newest", "--limit", "1")
	if err != nil {
		t.Fatalf("collections search: %v", err)
	}
	if strings.Contains(out, "next page") {
		t.Errorf("no cursor → no next-page hint: %s", out)
	}
	if strings.Contains(errOut, noteFragment) {
		t.Errorf("Newest last page must not emit a note: %s", errOut)
	}
}

// Unit-level guard on the pageability rule so the allowlist is pinned: Newest
// and the empty (default) sort are pageable; everything else is not.
func TestCollectionsSortIsCursorPageable(t *testing.T) {
	cases := map[string]bool{
		"":               true, // default → Newest
		"Newest":         true,
		"Most Followers": false,
		"newest":         false, // API is case-sensitive; only exact "Newest" pages
		"Oldest":         false, // hypothetical future sort → non-pageable by default
	}
	for sort, want := range cases {
		if got := collectionsSortIsCursorPageable(sort); got != want {
			t.Errorf("collectionsSortIsCursorPageable(%q) = %v, want %v", sort, got, want)
		}
	}
}
