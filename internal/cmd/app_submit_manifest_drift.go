package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/validate"
)

// THE MANIFEST-DRIFT WARNING.
//
// 🔴 TWO WRITERS, ONE MANIFEST. `civitai app submit` uploads a BUNDLE built from
// local disk, and that bundle IS the submission — the manifest inside it becomes
// the app's manifest. But the PLATFORM writes that manifest too: the website's
// manifest form calls `blocks.updateManifest`, which commits a new copy and
// re-enters review. So an author who edits anything on the website and then
// submits from a checkout that predates the edit silently reverts it. Nothing
// conflicts and nothing warns: the bundle simply wins.
//
// Measured precedent, not a hypothetical — `playable-collections` was live at
// 0.1.7 while its source repo held 0.1.3, and re-submitting the repo would have
// regressed the live app.
//
// 🔴 THIS COMPARES CONTENT, NOT GIT HISTORY, AND THAT IS THE WHOLE DESIGN. An
// earlier attempt fetched the app's canonical Forgejo repo and fast-forwarded
// the packaged directory onto it. That cannot work, for three independent
// reasons, each measured:
//
//   - The canonical repo is created fresh from a template, so it shares NO
//     ancestor with an author's own repo — and the first-party fleet all develop
//     in independent GitHub repos. `rev-list --left-right --count` on unrelated
//     histories returns both sides non-zero, which reads as "diverged", so the
//     common case was a hard refusal.
//   - The manifest the platform commits carries `iframe.src` and `trustTier`,
//     both of which `civitai app validate` rejects outright. Merging the
//     canonical copy into an author's tree makes their tree invalid.
//   - The documented release path (`git archive | tar -x`) has no `.git` at all,
//     so anything git-based is a silent no-op there.
//
// Comparing the two manifests as DATA has none of those problems: no fetch, no
// merge, no credential in argv, no mutation of anyone's working tree, and it
// works identically for a clone, an independent repo, and an export.
//
// 🔴 IT WARNS, IT DOES NOT REFUSE. The author's manifest is a legitimate source
// of truth — bumping a version and shipping the repo's copy is the normal
// release, and the fleet's own recipe does exactly that. What was missing was
// not permission, it was VISIBILITY: the overwrite happened silently. So this
// names the fields that are about to change and leaves the decision alone. A
// refusal here would block the documented release path on every run.

// serverOwnedIgnored is the set of manifest paths the comparison SKIPS, derived
// from the validator's own list rather than restated.
//
// 🔴 IT MUST BE DERIVED. The platform stamps `iframe.src` and forces
// `trustTier` into the copy it stores, and the CLI rejects both in a
// developer's manifest — so these two fields differ between the two copies for
// EVERY app, always, by construction. A hand-kept copy of that list would
// eventually miss one, and the symptom is a warning that fires on every submit
// forever, which is the shape that teaches people to ignore warnings.
//
// `version` is skipped for a different and equally structural reason: a submit
// exists to raise it, so it differs on every correct run.
func serverOwnedIgnored() map[string]struct{} {
	ignored := map[string]struct{}{"version": {}}
	for _, f := range validate.ServerOwnedFields {
		ignored[f] = struct{}{}
	}
	return ignored
}

// manifestDifference is one field whose stored value differs from the local one.
type manifestDifference struct {
	// Field is the dotted path, e.g. "tagline" or "page.title".
	Field string
	// Stored is the platform's value, rendered for a human; "" when the field is
	// absent there.
	Stored string
	// Local is the value in the bundle about to be uploaded.
	Local string
}

// driftListCap bounds how many fields the warning prints, for the same reason
// the dirty guard caps its path list: a reader looking at twenty lines scrolls
// past the one that mattered.
const driftListCap = 8

// diffManifests reports fields where the STORED manifest and the LOCAL one
// disagree, ignoring the fields that always disagree.
//
// 🔴 IT WALKS THE UNION OF BOTH KEY SETS, NOT THE LOCAL ONE. A field the website
// ADDED and the local file has never had (a tagline set for the first time in
// the web form) is exactly the case worth reporting, and iterating the local
// manifest alone cannot see it — the loop would have nothing to iterate to.
//
// 🔴 IT RECURSES ONLY INTO OBJECTS, and compares everything else by its JSON
// rendering. Arrays are compared whole rather than element-wise on purpose: a
// reordered `scopes` array IS a difference worth naming, and a per-index diff
// would report five changes for one reorder.
func diffManifests(stored, local map[string]any, prefix string, ignored map[string]struct{}) []manifestDifference {
	var out []manifestDifference

	keys := map[string]struct{}{}
	for k := range stored {
		keys[k] = struct{}{}
	}
	for k := range local {
		keys[k] = struct{}{}
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	for _, k := range sorted {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		if _, skip := ignored[path]; skip {
			continue
		}
		sv, sok := stored[k]
		lv, lok := local[k]

		// Both objects → recurse, so the reported path is the leaf that actually
		// changed rather than the whole subtree.
		sm, smok := sv.(map[string]any)
		lm, lmok := lv.(map[string]any)
		if smok && lmok {
			out = append(out, diffManifests(sm, lm, path, ignored)...)
			continue
		}

		ss, ls := renderJSON(sv, sok), renderJSON(lv, lok)
		if ss != ls {
			out = append(out, manifestDifference{Field: path, Stored: ss, Local: ls})
		}
	}
	return out
}

// renderJSON renders a manifest value for comparison and display. An ABSENT key
// renders as "" — distinct from a present-but-empty string, which renders as
// `""` with quotes, so "the website added a tagline" and "the website blanked
// the tagline" do not read identically.
func renderJSON(v any, present bool) string {
	if !present {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// readLocalManifest loads the manifest being packaged as generic JSON.
//
// Generic, not the typed manifest.Manifest, for the reason StoredAppManifest
// gives: a typed decode drops unknown keys, and an unknown key is precisely the
// kind the website might have edited.
func readLocalManifest(dir string) (map[string]any, error) {
	b, err := os.ReadFile(filepath.Join(dir, "block.manifest.json"))
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// storedManifestFetcher is the seam: the API read that returns the platform's
// copy. Defaulted to the real client; tests swap it.
type storedManifestFetcher func(ctx context.Context, appBlockID string) (*appapi.StoredAppManifest, error)

// warnOnManifestDrift prints one warning naming the fields this submit would
// overwrite, and returns the differences it found.
//
// 🔴 IT DEGRADES, NEVER ENFORCES — the rule the dirty guard states at length,
// and for the same reason: this is an accident preventer, not an authorization
// check, and an author offline or submitting a brand-new app must still be able
// to ship. Every state it cannot establish an answer for PROCEEDS SILENTLY:
//
//	no appBlockId yet (first submit)  → nothing (there is no stored copy)
//	the API read fails / is forbidden → nothing
//	the local manifest cannot be read → nothing (validation already reported it)
//	no differences                    → nothing
//
// The silence on a failed read is deliberate and is a REVERSAL of what the
// earlier git-based attempt did: it warned "the sync was skipped, your bundle
// may overwrite the website's changes" whenever it could not check. That
// sentence is true on every first submit and every offline run, so it fired
// constantly and said nothing actionable. A warning that cannot be acted on is
// noise, and noise is what makes the real warning invisible.
func warnOnManifestDrift(
	ctx context.Context,
	fetchStored storedManifestFetcher,
	warn io.Writer,
	dir, appBlockID string,
) []manifestDifference {
	if fetchStored == nil || strings.TrimSpace(appBlockID) == "" {
		return nil
	}
	local, err := readLocalManifest(dir)
	if err != nil {
		return nil
	}
	stored, err := fetchStored(ctx, appBlockID)
	if err != nil || stored == nil || len(stored.Manifest) == 0 {
		return nil
	}

	diffs := diffManifests(stored.Manifest, local, "", serverOwnedIgnored())
	if len(diffs) == 0 {
		return nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "this bundle will OVERWRITE %s that differ from the copy on the website:\n", pluralFields(len(diffs)))
	shown := diffs
	if len(shown) > driftListCap {
		shown = shown[:driftListCap]
	}
	for _, d := range shown {
		fmt.Fprintf(&b, "  %s\n      website: %s\n      yours:   %s\n", d.Field, orAbsent(d.Stored), orAbsent(d.Local))
	}
	if len(diffs) > len(shown) {
		fmt.Fprintf(&b, "  … and %d more\n", len(diffs)-len(shown))
	}
	b.WriteString("The manifest in your bundle replaces the stored one, so anything edited in the web form " +
		"and not mirrored here will be lost. If those edits should survive, copy them into block.manifest.json first.")
	warnf(warn, "%s", b.String())
	return diffs
}

// orAbsent renders the empty marker for a field that is not present at all,
// which must not read the same as a field present and empty.
func orAbsent(s string) string {
	if s == "" {
		return "(not set)"
	}
	return s
}

// pluralFields renders a field count for a human.
func pluralFields(n int) string {
	if n == 1 {
		return "1 field"
	}
	return fmt.Sprintf("%d fields", n)
}
