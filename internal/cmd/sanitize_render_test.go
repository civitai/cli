package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Control bytes built from numeric rune values (never typed as raw bytes): ESC
// (ANSI/OSC introducer), BEL (OSC terminator), and a raw C1 CSI (U+009B).
var (
	escRune = string(rune(0x1b))
	belRune = string(rune(0x07))
	c1CSI   = string(rune(0x9b))
)

// forge decorates a field with a representative attack payload: cursor-up +
// line-clear (output forgery over the CLI's own lines), an OSC-52 clipboard set
// terminated by BEL, an ANSI color, and a raw C1 CSI. json.Marshal will encode
// these as spec-compliant \uXXXX escapes, so the wire body is valid JSON.
func forge(prefix string) string {
	return prefix + escRune + "[1A" + escRune + "[2K" + // cursor up + clear line
		escRune + "]52;c;AAAA" + belRune + // OSC-52 clipboard set
		escRune + "[31m" + // ANSI red
		c1CSI + "[2K" // C1 CSI clear
}

// marshalBody JSON-encodes v to a compact wire body (valid JSON with \uXXXX
// escapes for the injected control chars).
func marshalBody(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal test body: %v", err)
	}
	return string(b)
}

// jsonString returns s as a JSON string literal (quoted, with control chars
// \uXXXX-escaped) for interpolation into a hand-built JSON body.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// assertJSONByteIdentical proves the --json output preserved the raw API bytes
// (control-char escapes intact): compacting the rendered --json output must equal
// the compact server body. emitJSON pretty-prints (re-indents) but must never
// alter string CONTENT, so a compact round-trip is byte-identical to the source.
func assertJSONByteIdentical(t *testing.T, jsonOut, compactBody string) {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(strings.TrimSpace(jsonOut))); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", err, jsonOut)
	}
	if buf.String() != compactBody {
		t.Errorf("--json mutated the raw bytes.\n compacted: %s\n want body: %s", buf.String(), compactBody)
	}
	// The JSON escape for ESC (the 6-char sequence json.Marshal emits for U+001B)
	// must survive verbatim — proof --json is NOT sanitized (safeTerm would have
	// removed the decoded control byte).
	escMarshaled, _ := json.Marshal(escRune)
	escSeq := strings.Trim(string(escMarshaled), `"`)
	if strings.Contains(compactBody, escSeq) && !strings.Contains(jsonOut, escSeq) {
		t.Errorf("--json stripped the %q escape; control chars must be preserved raw:\n%s", escSeq, jsonOut)
	}
}

func TestModelsSearchSanitizesControlChars(t *testing.T) {
	body := marshalBody(t, map[string]any{
		"items": []any{map[string]any{
			"id": 1, "name": forge("Pon"), "type": "Checkpoint",
			"creator": map[string]any{"username": forge("bob")},
			"stats":   map[string]any{"downloadCount": 10, "thumbsUpCount": 3},
		}},
		"metadata": map[string]any{},
	})
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) })

	out, _, err := run(t, "models", "search", "--query", "x")
	if err != nil {
		t.Fatalf("models search: %v", err)
	}
	assertNoControlBytes(t, "models search human output", out)
	if !strings.Contains(out, "Pon") || !strings.Contains(out, "bob") {
		t.Errorf("visible text should survive the strip: %s", out)
	}

	jsonOut, _, err := run(t, "models", "search", "--query", "x", "--json")
	if err != nil {
		t.Fatalf("models search --json: %v", err)
	}
	assertJSONByteIdentical(t, jsonOut, body)
}

func TestModelsGetSanitizesControlChars(t *testing.T) {
	body := marshalBody(t, map[string]any{
		"id": 7, "name": forge("Mod"), "type": "Checkpoint",
		"creator": map[string]any{"username": forge("bob")},
		"stats":   map[string]any{"downloadCount": 1, "thumbsUpCount": 2, "commentCount": 0},
		"tags":    []any{forge("tag"), "clean"},
		"modelVersions": []any{map[string]any{
			"id": 9, "name": forge("v1"), "baseModel": forge("SD 1.5"),
		}},
	})
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) })

	out, _, err := run(t, "models", "get", "7")
	if err != nil {
		t.Fatalf("models get: %v", err)
	}
	assertNoControlBytes(t, "models get human output", out)
	if !strings.Contains(out, "Mod") {
		t.Errorf("visible name should survive: %s", out)
	}

	jsonOut, _, err := run(t, "models", "get", "7", "--json")
	if err != nil {
		t.Fatalf("models get --json: %v", err)
	}
	assertJSONByteIdentical(t, jsonOut, body)
}

func TestUsersGetSanitizesControlChars(t *testing.T) {
	body := marshalBody(t, map[string]any{
		"items": []any{map[string]any{
			"id": 5, "username": forge("bob"), "image": forge("https://x/img.png"),
		}},
	})
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) })

	// Numeric arg → exact ?ids= lookup (no fuzzy-match requirement).
	out, _, err := run(t, "users", "get", "5")
	if err != nil {
		t.Fatalf("users get: %v", err)
	}
	assertNoControlBytes(t, "users get human output", out)

	jsonOut, _, err := run(t, "users", "get", "5", "--json")
	if err != nil {
		t.Fatalf("users get --json: %v", err)
	}
	assertJSONByteIdentical(t, jsonOut, body)
}

func TestModelVersionsGetSanitizesControlChars(t *testing.T) {
	body := marshalBody(t, map[string]any{
		"id": 9, "modelId": 4, "name": forge("ver"), "baseModel": forge("SD 1.5"),
		"air": forge("air"), "trainedWords": []any{forge("trigger")},
		"downloadUrl": forge("https://x/dl"),
		"stats":       map[string]any{"downloadCount": 1, "thumbsUpCount": 1},
		"model":       map[string]any{"name": forge("Mod"), "type": "Checkpoint"},
		"files": []any{map[string]any{
			"id": 1, "name": forge("weights.safetensors"), "type": "Model", "sizeKB": 2048, "primary": true,
		}},
	})
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) })

	out, _, err := run(t, "model-versions", "get", "9")
	if err != nil {
		t.Fatalf("model-versions get: %v", err)
	}
	assertNoControlBytes(t, "model-versions get human output", out)

	jsonOut, _, err := run(t, "model-versions", "get", "9", "--json")
	if err != nil {
		t.Fatalf("model-versions get --json: %v", err)
	}
	assertJSONByteIdentical(t, jsonOut, body)
}

func TestArticlesGetSanitizesControlChars(t *testing.T) {
	body := marshalBody(t, map[string]any{
		"id": 3, "title": forge("Guide"),
		"user":        map[string]any{"username": forge("bob")},
		"nsfwLevel":   1,
		"publishedAt": "2026-01-02T03:04:05Z",
		"tags":        []any{map[string]any{"name": forge("tag")}},
	})
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) })

	out, _, err := run(t, "articles", "get", "3")
	if err != nil {
		t.Fatalf("articles get: %v", err)
	}
	assertNoControlBytes(t, "articles get human output", out)
	if !strings.Contains(out, "Guide") {
		t.Errorf("visible title text should survive: %s", out)
	}

	jsonOut, _, err := run(t, "articles", "get", "3", "--json")
	if err != nil {
		t.Fatalf("articles get --json: %v", err)
	}
	assertJSONByteIdentical(t, jsonOut, body)
}
