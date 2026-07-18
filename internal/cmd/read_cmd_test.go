package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// setupReadServer points the CLI at a local server via CIVITAI_BASE_URL and
// clears any real token so command tests are hermetic + anonymous.
func setupReadServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
}

func TestReadCommandsRegistered(t *testing.T) {
	out, _, err := run(t, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	for _, want := range []string{"models", "model-versions", "images", "tags", "creators", "users"} {
		if !strings.Contains(out, want) {
			t.Errorf("root help missing read command %q\n%s", want, out)
		}
	}
}

func TestModelsSearchHumanOutput(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.URL.Query().Get("query") != "pony" {
			t.Errorf("query not passed: %v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`{"items":[{"id":1,"name":"Pony","type":"Checkpoint",
		  "creator":{"username":"bob"},"stats":{"downloadCount":10,"thumbsUpCount":3}}],
		  "metadata":{"nextCursor":"c1"}}`))
	})
	out, _, err := run(t, "models", "search", "--query", "pony", "--limit", "1")
	if err != nil {
		t.Fatalf("models search: %v", err)
	}
	if !strings.Contains(out, "Pony") || !strings.Contains(out, "bob") {
		t.Errorf("human output missing fields: %s", out)
	}
	if !strings.Contains(out, "next cursor: c1") {
		t.Errorf("footer should show next cursor: %s", out)
	}
}

func TestModelsSearchJSONPassthrough(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[],"metadata":{}}`))
	})
	out, _, err := run(t, "models", "search", "--json")
	if err != nil {
		t.Fatalf("models search --json: %v", err)
	}
	if !strings.Contains(out, `"items"`) || !strings.Contains(out, `"metadata"`) {
		t.Errorf("--json should print raw API JSON: %s", out)
	}
}

func TestModelsSearchLimitValidation(t *testing.T) {
	_, _, err := run(t, "models", "search", "--limit", "101")
	if err == nil {
		t.Fatal("expected error for --limit > 100")
	}
	if !strings.Contains(err.Error(), "1 and 100") {
		t.Errorf("error should name the max: %v", err)
	}
}

func TestImagesSearchLimitValidation(t *testing.T) {
	_, _, err := run(t, "images", "search", "--limit", "201")
	if err == nil {
		t.Fatal("expected error for --limit > 200")
	}
	if !strings.Contains(err.Error(), "1 and 200") {
		t.Errorf("error should name the max: %v", err)
	}
}

func TestModelsGetRejectsNonNumericId(t *testing.T) {
	_, _, err := run(t, "models", "get", "abc")
	if err == nil {
		t.Fatal("expected error for non-numeric model id")
	}
}

func TestModelsGetSurfacesServerError(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"No model with id 999"}`))
	})
	_, _, err := run(t, "models", "get", "999")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "No model with id 999") {
		t.Errorf("error should surface API body: %v", err)
	}
}

func TestUsersGetResolvesByQuery(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users" || r.URL.Query().Get("query") != "bob" {
			t.Errorf("unexpected users request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"items":[{"id":5,"username":"bob"},{"id":6,"username":"bobby"}]}`))
	})
	out, _, err := run(t, "users", "get", "bob")
	if err != nil {
		t.Fatalf("users get: %v", err)
	}
	if !strings.Contains(out, "bob (id 5)") {
		t.Errorf("should pick exact username match: %s", out)
	}
}

func TestUsersGetNumericUsesIds(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ids") != "5" {
			t.Errorf("numeric arg should use ?ids=: %v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`{"items":[{"id":5,"username":"bob"}]}`))
	})
	if _, _, err := run(t, "users", "get", "5"); err != nil {
		t.Fatalf("users get 5: %v", err)
	}
}

func TestUsersGetNoExactMatchErrors(t *testing.T) {
	// A name query returning only FUZZY neighbours (no exact username) must
	// ERROR, not confidently print the top fuzzy hit as "the" user.
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"id":5,"username":"bob"},{"id":6,"username":"bobby"}]}`))
	})
	out, _, err := run(t, "users", "get", "bo")
	if err == nil {
		t.Fatalf("expected an error for a name with no exact match; got output:\n%s", out)
	}
	if !strings.Contains(err.Error(), "no user found with exact username") {
		t.Errorf("error should explain no-exact-match, got: %v", err)
	}
	if !strings.Contains(err.Error(), "bob") {
		t.Errorf("error should list the closest candidates, got: %v", err)
	}
	if strings.Contains(out, "id 5") {
		t.Errorf("must NOT print a fuzzy user as the answer:\n%s", out)
	}
}

// TestImagesSearchBaseModelColumn asserts the human table surfaces a BASE MODEL
// column, populated from the API's per-image `baseModel` string, with an empty
// value rendered as `-`.
func TestImagesSearchBaseModelColumn(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[
		  {"id":1,"url":"https://img/1","width":10,"height":20,"nsfwLevel":"None","username":"alice","baseModel":"Krea 2","stats":{}},
		  {"id":2,"url":"https://img/2","width":30,"height":40,"nsfwLevel":"None","username":"bob","baseModel":"","stats":{}}
		],"metadata":{}}`))
	})
	out, _, err := run(t, "images", "search")
	if err != nil {
		t.Fatalf("images search: %v", err)
	}
	if !strings.Contains(out, "BASE MODEL") {
		t.Errorf("table should have a BASE MODEL header: %s", out)
	}
	if !strings.Contains(out, "Krea 2") {
		t.Errorf("populated baseModel should render: %s", out)
	}
	// The empty-baseModel row (id 2) must show a dash for its base model.
	var id2Line string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "2 ") {
			id2Line = line
		}
	}
	if id2Line == "" {
		t.Fatalf("could not find the id-2 row: %s", out)
	}
	if !strings.Contains(id2Line, "-") {
		t.Errorf("empty baseModel should render as '-': %q", id2Line)
	}
}

// TestImagesSearchBaseModelSanitized asserts a server-controlled baseModel with
// terminal control characters is routed through safeTerm before printing.
func TestImagesSearchBaseModelSanitized(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		// A real ANSI escape (ESC 0x1b, JSON-escaped) in a server string; safeTerm strips it.
		_, _ = w.Write([]byte(`{"items":[
		  {"id":1,"url":"https://img/1","width":10,"height":20,"nsfwLevel":"None","username":"alice","baseModel":"Krea\u001b[31m2","stats":{}}
		],"metadata":{}}`))
	})
	out, _, err := run(t, "images", "search")
	if err != nil {
		t.Fatalf("images search: %v", err)
	}
	if strings.ContainsRune(out, 0x1b) {
		t.Errorf("baseModel escape byte should be stripped by safeTerm: %q", out)
	}
	// The visible text survives (only the ESC control byte is removed).
	if !strings.Contains(out, "Krea") || !strings.Contains(out, "[31m2") {
		t.Errorf("printable baseModel text should survive sanitizing: %q", out)
	}
}

// TestImagesSearchJSONUnchangedByBaseModel asserts --json stays a raw
// passthrough — the baseModel column rendering must not touch machine output.
func TestImagesSearchJSONUnchangedByBaseModel(t *testing.T) {
	const raw = `{"items":[{"id":1,"baseModel":"Krea 2","url":"u"}],"metadata":{}}`
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(raw))
	})
	out, _, err := run(t, "images", "search", "--json")
	if err != nil {
		t.Fatalf("images search --json: %v", err)
	}
	if strings.Contains(out, "BASE MODEL") {
		t.Errorf("--json must not carry the human table header: %s", out)
	}
	if !strings.Contains(out, `"baseModel"`) {
		t.Errorf("--json should pass the raw body through: %s", out)
	}
}

// TestImagesSearchModelIDSortWarns asserts that combining --model-id with --sort
// prints the "ignored sort" note on STDERR (not stdout, so --json/stdout stay
// clean), while --model-id alone and --sort alone stay silent.
func TestImagesSearchModelIDSortWarns(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[],"metadata":{}}`))
	}
	const wantNote = "the API ignores --sort when --model-id is set"

	// Both set → note on stderr, nothing on stdout.
	setupReadServer(t, handler)
	stdout, stderr, err := run(t, "images", "search", "--model-id", "123", "--sort", "Most Reactions")
	if err != nil {
		t.Fatalf("images search: %v", err)
	}
	if !strings.Contains(stderr, wantNote) {
		t.Errorf("expected the sort-ignored note on stderr, got: %q", stderr)
	}
	if strings.Contains(stdout, wantNote) {
		t.Errorf("the note must not pollute stdout: %q", stdout)
	}

	// --model-id alone → no note.
	setupReadServer(t, handler)
	if _, stderr, err := run(t, "images", "search", "--model-id", "123"); err != nil {
		t.Fatalf("images search --model-id: %v", err)
	} else if strings.Contains(stderr, wantNote) {
		t.Errorf("--model-id alone must not warn: %q", stderr)
	}

	// --sort alone → no note.
	setupReadServer(t, handler)
	if _, stderr, err := run(t, "images", "search", "--sort", "Most Reactions"); err != nil {
		t.Fatalf("images search --sort: %v", err)
	} else if strings.Contains(stderr, wantNote) {
		t.Errorf("--sort alone must not warn: %q", stderr)
	}

	// --model-version-id + --sort → no note (that combo honours sort).
	setupReadServer(t, handler)
	if _, stderr, err := run(t, "images", "search", "--model-version-id", "128713", "--sort", "Most Reactions"); err != nil {
		t.Fatalf("images search --model-version-id --sort: %v", err)
	} else if strings.Contains(stderr, wantNote) {
		t.Errorf("--model-version-id honours --sort; must not warn: %q", stderr)
	}
}

// TestRootHelpMentionsImagesSearch guards Fix 4: the top-level help's examples
// surface image browsing as first-class.
func TestRootHelpMentionsImagesSearch(t *testing.T) {
	out, _, err := run(t, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	if !strings.Contains(out, "images search") {
		t.Errorf("root help should include an `images search` example: %s", out)
	}
}

func TestTagsSearchWiring(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tags" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"items":[{"name":"anime","link":"l"}],"metadata":{}}`))
	})
	out, _, err := run(t, "tags", "search", "--query", "anime")
	if err != nil {
		t.Fatalf("tags search: %v", err)
	}
	if !strings.Contains(out, "anime") {
		t.Errorf("tags output missing name: %s", out)
	}
}
