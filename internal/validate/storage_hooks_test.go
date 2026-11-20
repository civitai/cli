package validate

import (
	"encoding/json"
	"strings"
	"testing"
)

// storage_hooks_test.go pins `civitai app validate`'s reading of a collection's
// permission `hook` against civitai#8126, which switched the `hooks` storage
// feature ON and left who may add or change a hook to the default-off Flipt
// cohort `app-blocks-storage-hooks`:
//
//   - a hook the server's parseHook accepts draws a ROLLOUT WARNING, not the
//     old "not available yet" gate (the CLI cannot see the cohort);
//   - a hook whose file is inside the manifest's `outputDir` is the server's
//     HARD ERROR (isStorageHookFileUnderOutputDir), which the schema cannot
//     state and this package used to let through as a gate warning.

const testHookSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// builtManifest is docManifest with a build: `buildCommand` plus the given
// `outputDir`, so buildCoherence has nothing to say and the only findings are
// the storage ones under test.
func builtManifest(outputDir, storage string, scopes ...string) string {
	od, err := json.Marshal(outputDir)
	if err != nil {
		panic(err)
	}
	return `{"buildCommand":"npm run build","outputDir":` + string(od) + `,` +
		strings.TrimPrefix(docManifest(storage, scopes...), "{")
}

func hookDecl(file string) string {
	return `{"visibility":"private","hook":{"file":"` + file + `","sha256":"` + testHookSHA + `"}}`
}

// rolloutWarnings returns the hook-rollout advisories among fs.
func rolloutWarnings(fs []Finding) []Finding {
	var out []Finding
	for _, f := range fs {
		if strings.Contains(f.Message, "is in a limited rollout") {
			out = append(out, f)
		}
	}
	return out
}

// TestStorageHookRolloutIsAWarning — a well-formed hook is ACCEPTED by the
// server since civitai#8126, but only from an author in the rollout cohort (a
// new or changed hook from anyone else is refused `not_in_cohort` at submit).
// The CLI cannot see the cohort or the approved manifest, so it warns, in the
// server's words, and never errors.
//
// The message is a LITERAL, not built from storageHookRolloutWarning.
func TestStorageHookRolloutIsAWarning(t *testing.T) {
	for _, tc := range []struct {
		name, collection, decl, twin string
		scopes                       []string
	}{
		{
			name: "hook", collection: "guarded",
			decl:   `{"visibility":"shared","write":"own","hook":{"file":"hooks/guard.js","sha256":"` + testHookSHA + `","timeoutMs":100}}`,
			twin:   `{"visibility":"shared","write":"own"}`,
			scopes: []string{"apps:storage", "apps:storage:publish"},
		},
		// Exactly APP_STORAGE_HOOK_FILE_MAX_LENGTH (200) characters: the bound is inclusive.
		{
			name: "hook with a 200-character file", collection: "long_path",
			decl:   hookDecl("h/" + strings.Repeat("a", 195) + ".js"),
			twin:   `{"visibility":"private"}`,
			scopes: []string{"apps:storage"},
		},
		// The file pattern ends `\.m?js$`: an ES module may be .mjs.
		{
			name: "hook with a .mjs file", collection: "esm_hook",
			decl:   hookDecl("hooks/guard.mjs"),
			twin:   `{"visibility":"private"}`,
			scopes: []string{"apps:storage"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			twin := manifestOnlyResult(t, docManifest(oneCollection(tc.collection, tc.twin), tc.scopes...))
			if len(twin.Errors) != 0 || len(twin.Warnings) != 0 {
				t.Errorf("accept twin: want nothing, got errors:%s\n warnings:%s",
					describe(twin.Errors), describe(twin.Warnings))
			}
			res := manifestOnlyResult(t, docManifest(oneCollection(tc.collection, tc.decl), tc.scopes...))
			if len(res.Errors) != 0 {
				t.Errorf("a hook must not be an ERROR (submit blocks on errors), got:%s", describe(res.Errors))
			}
			if len(res.Warnings) != 1 {
				t.Fatalf("want exactly 1 warning, got %d:%s", len(res.Warnings), describe(res.Warnings))
			}
			at := "storage.collections." + tc.collection + ".hook"
			want := at + " is in a limited rollout: civitai.com accepts a new or changed hook only from " +
				"authors the rollout includes, and refuses it otherwise. If submit refuses it, keep the " +
				"approved hook or remove hook (the collection schema still applies). It is a warning here, " +
				"not an error, because whether this account is in the rollout, and whether the hook is new " +
				"or changed since the approved version, is known only to civitai.com."
			if got := res.Warnings[0]; got.Message != want || got.Field != at {
				t.Errorf("got  [%s] %s\nwant [%s] %s", got.Field, got.Message, at, want)
			}
			if strings.Contains(res.Warnings[0].Message, "not available yet") {
				t.Error("the hook warning still says hooks are not available — civitai#8126 switched them on")
			}
		})
	}
}

// TestStorageHookFileInsideOutputDirIsRefused — parseHook's
// isStorageHookFileUnderOutputDir, ported. The first five rows are the server's
// own vectors (block-storage.constants.test.ts at civitai#8126's head:
// "refuses a file inside outputDir %s (%s)" and "accepts a file beside outputDir
// that only shares its prefix"); the rest widen them to the boundaries the port
// has to get right.
//
// A refused hook draws the server's error at `<hook>.file` and NOT the rollout
// warning (parseHook returned null, so the cohort is never consulted); an
// accepted one draws no error and the rollout warning.
func TestStorageHookFileInsideOutputDirIsRefused(t *testing.T) {
	for _, tc := range []struct {
		outputDir, file string
		refused         bool
	}{
		// --- the server's golden vectors ---
		{"dist", "dist/hooks/cards.js", true},
		{"./dist/", "dist/cards.js", true},
		{"dist/", "dist/x.mjs", true},
		{".", "hooks/cards.js", true},
		{"dist", "dist-hooks/cards.js", false},
		// --- the boundaries ---
		{"dist", "dist2/hook.js", false},     // sibling sharing the prefix: a "/" boundary, not a string prefix
		{"dist", "hooks/dist/a.js", false},   // dist as a LATER segment is not inside ./dist
		{"dist", "dist/a/b/c/hook.js", true}, // nested deep
		{"build/web", "build/web/hook.js", true},
		{"build/web", "build/hook.js", false}, // parent of a nested outputDir
		{"build/web", "build/web2/hook.js", false},
		{"hook.js", "hook.js", true},               // equal to the directory itself
		{"hooks/guard.js", "hooks/guard.js", true}, // equal, nested
		{"./", "hooks/cards.js", true},             // "./" strips to "" — the whole bundle
		{"./.", "hooks/cards.js", true},            // strips to "." — the whole bundle
		{"././dist//", "dist/cards.js", true},      // both strips loop
		{" dist ", "dist/cards.js", true},          // trimmed
		{"\tdist\n", "dist/cards.js", true},        // trimmed (JS line terminators too)
		{"\uFEFFdist", "dist/cards.js", true},      // JS trim() strips U+FEFF; strings.TrimSpace does not
		{"\u0085dist", "dist/cards.js", false},     // JS trim() keeps U+0085; strings.TrimSpace strips it
		{"Dist", "dist/cards.js", false},           // case-sensitive
		{"dist/.", "dist/cards.js", false},         // only a WHOLE "." is the root; "dist/." is a different prefix
	} {
		t.Run(tc.outputDir+" | "+tc.file, func(t *testing.T) {
			res := manifestOnlyResult(t, builtManifest(tc.outputDir, oneCollection("cards", hookDecl(tc.file)), "apps:storage"))
			rollout := rolloutWarnings(res.Warnings)
			if !tc.refused {
				if len(res.Errors) != 0 {
					t.Errorf("want no error, got:%s", describe(res.Errors))
				}
				if len(rollout) != 1 {
					t.Errorf("an accepted hook draws the rollout warning, got:%s", describe(res.Warnings))
				}
				return
			}
			fileJSON, _ := json.Marshal(tc.file)
			want := "storage.collections.cards.hook.file " + string(fileJSON) +
				" is inside the build output (outputDir) — a hook must be a committed source file of the " +
				"bundle, shipped verbatim, so its sha256 can be checked at submit"
			if len(res.Errors) != 1 || res.Errors[0].Message != want ||
				res.Errors[0].Field != "storage.collections.cards.hook.file" {
				t.Errorf("want exactly [storage.collections.cards.hook.file] %s\ngot %d:%s", want, len(res.Errors), describe(res.Errors))
			}
			if len(rollout) != 0 {
				t.Errorf("a refused hook must not also draw the rollout warning, got:%s", describe(rollout))
			}
		})
	}
}

// TestStorageHookOutputDirCheckIsIndependentOfTheOtherHookKeys — the server
// pushes the outputDir refusal from the `file` branch of parseHook, beside (not
// instead of) its sha256 / timeoutMs / key errors. So a hook with a bad sha256
// in outputDir gets BOTH: the schema's sha256 error and the outputDir one.
// And a file that fails the shape or node_modules check never reaches it.
func TestStorageHookOutputDirCheckIsIndependentOfTheOtherHookKeys(t *testing.T) {
	outDirMsg := "is inside the build output (outputDir)"
	count := func(fs []Finding) int {
		n := 0
		for _, f := range fs {
			if strings.Contains(f.Message, outDirMsg) {
				n++
			}
		}
		return n
	}
	for _, tc := range []struct {
		name, decl string
		want       int
	}{
		{"bad sha256", `{"visibility":"private","hook":{"file":"dist/h.js","sha256":"ABC"}}`, 1},
		{"timeoutMs out of range", `{"visibility":"private","hook":{"file":"dist/h.js","sha256":"` + testHookSHA + `","timeoutMs":501}}`, 1},
		{"unknown key", `{"visibility":"private","hook":{"file":"dist/h.js","sha256":"` + testHookSHA + `","x":1}}`, 1},
		{"file fails the pattern", `{"visibility":"private","hook":{"file":"dist/h.ts","sha256":"` + testHookSHA + `"}}`, 0},
		{"file inside node_modules", `{"visibility":"private","hook":{"file":"dist/node_modules/h.js","sha256":"` + testHookSHA + `"}}`, 0},
		{"invalid visibility", `{"visibility":"public","hook":{"file":"dist/h.js","sha256":"` + testHookSHA + `"}}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := manifestOnlyResult(t, builtManifest("dist", oneCollection("cards", tc.decl), "apps:storage"))
			if got := count(res.Errors); got != tc.want {
				t.Errorf("outputDir refusals = %d, want %d; errors:%s", got, tc.want, describe(res.Errors))
			}
			if tc.want == 1 && len(res.Errors) < 2 {
				t.Errorf("want the other key's error BESIDE the outputDir one, got:%s", describe(res.Errors))
			}
			if n := len(rolloutWarnings(res.Warnings)); n != 0 {
				t.Errorf("a hook parseHook refuses must not draw the rollout warning, got %d", n)
			}
		})
	}
}

// TestIsStorageHookFileUnderOutputDirWithoutAString — outputDirPrefix returns
// null for a missing or non-string outputDir, and nothing is then inside it.
// (The schema reports a non-string outputDir on its own.)
func TestIsStorageHookFileUnderOutputDirWithoutAString(t *testing.T) {
	for _, od := range []any{nil, 1.0, true, []any{"dist"}, map[string]any{}} {
		if isStorageHookFileUnderOutputDir("dist/h.js", od) {
			t.Errorf("outputDir %#v: want not inside", od)
		}
	}
	// Positive control: the same file IS inside a string "dist".
	if !isStorageHookFileUnderOutputDir("dist/h.js", "dist") {
		t.Error("positive control: dist/h.js must be inside outputDir \"dist\"")
	}
	// No outputDir in the manifest at all: the hook is accepted (rollout warning, no error).
	res := manifestOnlyResult(t, docManifest(oneCollection("cards", hookDecl("dist/h.js")), "apps:storage"))
	if len(res.Errors) != 0 || len(rolloutWarnings(res.Warnings)) != 1 {
		t.Errorf("no outputDir: want no error and the rollout warning, got errors:%s warnings:%s",
			describe(res.Errors), describe(res.Warnings))
	}
}
