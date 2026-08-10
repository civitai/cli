package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
)

// THE TWO DOGFOOD HELP GAPS ON THE SPEND SURFACE.
//
// Both guards are about what `civitai generate --help` and the README TEACH, not
// about what the command does — nothing here asserts a behaviour, because both
// issues were fixed docs-only on purpose (AGENTS.md item 13: the generation path
// mirrors nothing, so the fix for "the example is wrong" is a better example, not
// a new local check).
//
// 🔴 THESE DO NOT REPLACE THE GOLDEN. `generate_help_long.txt` pins the whole
// Long exactly and is what closes ADDITION (item 28). These pin two properties
// the golden structurally cannot hold, because a golden is re-approved with
// `-update` and says nothing about what the new text must still contain.

// bashBlockRe captures the body of every ```bash fenced block in the README —
// the blocks that TEACH a command to run. It deliberately does not match a
// ```console block: those are transcripts of output that was really produced
// (the substitution demo, for one), and editing a recorded transcript to satisfy
// a guard would make it a fabrication.
var bashBlockRe = regexp.MustCompile("(?s)```bash\\n(.*?)```")

// TestCheckpointExamplesNameAnEcosystem is the regression guard for issue #345.
//
// The help's headline example was `civitai generate "a cat" --checkpoint 128713
// --lora 250712:0.8` with no ecosystem. Copied verbatim against the default
// ecosystem it charged 8 Buzz and returned `Status: failed` with 0 deliverable
// outputs: the checkpoint was SD 1.5 and the request ran under `ecosystem:
// zImage` / `steps: 9` / `cfgScale: 1`, because naming a checkpoint does not move
// the settings with it.
//
// 🔴 THE INVARIANT IS THE PAIRING, NOT A WORD. This does not check that the help
// contains a sentence about ecosystems — a spelled guard that any paraphrase or
// unrelated mention satisfies. It checks that no surface hands a reader a
// runnable `--checkpoint` invocation with no `--ecosystem` beside it, which is
// the exact shape that was copied and charged.
func TestCheckpointExamplesNameAnEcosystem(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join(repoRootDir(t), "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}

	root := NewRootCmd()
	sources := map[string]string{
		"generate Example": newGenerateCmd().Example,
		"generate Long":    newGenerateCmd().Long,
		"root Long":        root.Long,
		"root Example":     root.Example,
	}
	blocks := bashBlockRe.FindAllStringSubmatch(string(readme), -1)
	// Positive control on the PARSER: a regex that stopped matching fences would
	// scan nothing and report a serene pass over the whole README.
	if len(blocks) < 10 {
		t.Fatalf("CONTROL failure: found %d ```bash blocks in README.md, expected many more — "+
			"the fence regex is not matching, so this guard is reading nothing", len(blocks))
	}
	for i, b := range blocks {
		sources["README.md ```bash block "+strconv.Itoa(i)] = b[1]
	}

	checkpointExamples := 0
	for name, text := range sources {
		for _, line := range strings.Split(text, "\n") {
			if !strings.Contains(line, "civitai generate ") || !strings.Contains(line, "--checkpoint") {
				continue
			}
			checkpointExamples++
			if !strings.Contains(line, "--ecosystem") {
				t.Errorf("%s teaches --checkpoint with no --ecosystem beside it:\n\t%s\n\n"+
					"🔴 This is issue #345. A checkpoint does not carry its ecosystem: the engine, steps, cfg scale and "+
					"sampler follow the ecosystem, so a checkpoint from a different one is accepted, CHARGED, and can "+
					"return no usable output — and nothing local detects it, `Resources ready` included. Name the "+
					"ecosystem in the example, and use a placeholder rather than a literal id: which checkpoint belongs "+
					"to which ecosystem is server knowledge this repo must not vendor (item 13).", name, strings.TrimSpace(line))
			}
		}
	}

	// Positive control on the FINDER: zero `--checkpoint` examples anywhere is
	// indistinguishable from a guard wired to the wrong text, and it would also
	// mean the flag became undiscoverable — which is not the fix #345 asked for.
	if checkpointExamples == 0 {
		t.Fatal("CONTROL failure: no `civitai generate … --checkpoint …` example found in the help or the README. " +
			"#345 asked for the example to be CORRECTED and given its own entry, not deleted; with none, this guard is inert.")
	}
}

// degradingGraphKeys returns the wire names of the four graph parameters that
// have no flag, read off genapi.Graph by REFLECTION rather than hand-listed, so
// a renamed json tag moves the guard with it instead of silently un-pinning it.
func degradingGraphKeys(t *testing.T) []string {
	t.Helper()
	typ := reflect.TypeOf(genapi.Graph{})
	var keys []string
	for _, field := range []string{"Seed", "Steps", "CfgScale", "Sampler"} {
		f, ok := typ.FieldByName(field)
		if !ok {
			t.Fatalf("CONTROL failure: genapi.Graph has no field %q. It is one of the parameters #329 requires the help "+
				"to name; if it was renamed or removed, fix this list rather than dropping the name from the help.", field)
		}
		tag := strings.Split(f.Tag.Get("json"), ",")[0]
		if tag == "" {
			t.Fatalf("CONTROL failure: genapi.Graph.%s has no json tag, so there is no wire name to look for", field)
		}
		keys = append(keys, tag)
	}
	return keys
}

// TestGenerateHelpNamesTheFlaglessGraphParameters is the regression guard for
// issue #329's help half.
//
// `--help` said only that `--input` "is how you reach graph parameters this CLI
// has no flag for" and never named one, so a user wanting a seed — the only route
// to reproducing a run — had no discoverable path at all. The names are safe to
// print because they are fields genapi.Graph ALREADY models; naming them vendors
// no enum, no default and no range, which is what item 13 forbids.
//
// 🔴 THE FLAGS THEMSELVES STAY REFUSED, and this guard must never be read as a
// step toward them: item 14 measured `steps: 0` accepted by the server at a 0.333
// cost factor — HTTP 200, billed, degenerate output. That is why there is no
// --steps.
func TestGenerateHelpNamesTheFlaglessGraphParameters(t *testing.T) {
	long := newGenerateCmd().Long
	if len(long) < 500 {
		t.Fatalf("CONTROL failure: the generate Long is %d bytes, too short to be the real help — "+
			"the assertions below would pass on an empty string", len(long))
	}

	for _, key := range degradingGraphKeys(t) {
		// Word-boundary, NOT a substring: `seed` substring-matches `Seedream`,
		// which is an ECOSYSTEM name the help already prints in an --image
		// example. A Contains() check here would have passed before this change
		// ever landed — the spelled-guard failure in a live form.
		re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(key) + `\b`)
		if !re.MatchString(long) {
			t.Errorf("`civitai generate --help` never names the graph parameter %q.\n\n"+
				"🔴 This is issue #329. The raw-graph paragraph promises access to \"graph parameters this CLI has no flag "+
				"for\" without naming any, so there is no discoverable route to a seed and no reproducibility. Name the four "+
				"in the RAW GRAPHS paragraph. Do NOT close this by adding a --%s flag: item 14 measured the server ACCEPTING "+
				"a zero for steps/cfgScale and billing the degenerate job.", key, key)
		}
	}

	// The names alone are not the fix if the route to setting them is missing.
	if !strings.Contains(long, "--print-input") || !strings.Contains(long, "--input") {
		t.Errorf("the RAW GRAPHS paragraph names the parameters but not the --print-input/--input round-trip that reaches them:\n%s", long)
	}
}
