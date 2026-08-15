package cli_test

import (
	"os"
	"regexp"
	"testing"

	"gopkg.in/yaml.v3"
)

// `.goreleaser.yaml`'s changelog exclude patterns decide what the PUBLISHED
// release notes say. They were wrong for at least three releases and nobody
// noticed, because the failure is silent in the only direction anyone looks: a
// commit that should have been filtered simply appears, and a human reading the
// notes assumes it was meant to.
//
// The patterns used to be `^docs:`, `^test:`, `^chore:` — anchors that match
// only the UNSCOPED conventional-commit form. This repo writes SCOPED subjects
// (`chore(scaffold):`, `docs(release):`), so every one of them landed in the
// changelog anyway. v0.1.93, v0.1.94 and v0.1.95's notes each recorded the
// resulting "leaking commits" count BY HAND, three releases running, without
// anyone tracing it to the regex. See `claudedocs/release-v0.1.95-draft.md`.
//
// # Why this is a test and not a comment
//
// The fix is one character class in a YAML string. It reverts by accident — a
// merge, a reformat, someone "simplifying" the escaping — and the revert
// produces no error, no failing build, and no visible change until a release
// ships with the wrong notes weeks later. The only cheap way to hold it is to
// assert the patterns' BEHAVIOUR on real subjects.
//
// # Why the negative controls are the important half
//
// A filter that matches nothing looks exactly like a filter with nothing to
// match. So this table asserts both directions, and deliberately includes
// near-misses (`chorus:`, `documentation:`) that a lazily-widened pattern such
// as `^chore` or `^docs` would swallow. If someone "fixes" a future miss by
// dropping the colon, those two rows go red.
//
// The two historical leakers are quoted VERBATIM from the v0.1.95 release body
// rather than paraphrased, so this guard is pinned to subjects that actually
// shipped wrong, not to invented ones.
//
// What this does NOT check: that goreleaser applies these patterns to the
// subject line the way this test assumes, or anything about a real release
// body. That is a claim about goreleaser, and the honest place to confirm it is
// the NEXT release's published notes.
func TestGoreleaserChangelogFiltersExcludeScopedCommits(t *testing.T) {
	raw, err := os.ReadFile(".goreleaser.yaml")
	if err != nil {
		t.Fatalf("read .goreleaser.yaml: %v", err)
	}

	var cfg struct {
		Changelog struct {
			Filters struct {
				Exclude []string `yaml:"exclude"`
			} `yaml:"filters"`
		} `yaml:"changelog"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse .goreleaser.yaml: %v", err)
	}

	pats := cfg.Changelog.Filters.Exclude

	// POSITIVE CONTROL ON THE INSTRUMENT ITSELF. If the key moves or is renamed,
	// `pats` goes empty, every "must be excluded" row below would report
	// "not excluded", and every "must be kept" row would pass — i.e. a
	// half-vacuous suite. Fail loudly on the empty read instead.
	if len(pats) < 3 {
		t.Fatalf("changelog.filters.exclude has %d pattern(s), want at least 3 "+
			"(docs/test/chore) — did the key move in .goreleaser.yaml?", len(pats))
	}

	res := make([]*regexp.Regexp, 0, len(pats))
	for _, p := range pats {
		re, err := regexp.Compile(p)
		if err != nil {
			t.Fatalf("changelog exclude pattern %q does not compile as a Go regexp: %v", p, err)
		}
		res = append(res, re)
	}

	excluded := func(subject string) bool {
		for _, re := range res {
			if re.MatchString(subject) {
				return true
			}
		}
		return false
	}

	for _, tc := range []struct {
		subject string
		want    bool
		why     string
	}{
		// The two that actually shipped into v0.1.95's published changelog.
		{"chore(scaffold): bump @civitai/* pins to published latest (#408)", true,
			"scoped chore leaked into v0.1.95's notes"},
		{"docs(release): v0.1.94 shipped — and its own commit count went stale between writing and tagging, for the third time (#407)", true,
			"scoped docs leaked into v0.1.95's notes"},

		// The unscoped forms must keep working — the fix widens, never replaces.
		{"chore: bump a dependency", true, "unscoped chore"},
		{"docs: fix a typo", true, "unscoped docs"},
		{"test: add a case", true, "unscoped test"},
		{"test(pkgzip): add a case", true, "scoped test"},

		// User-facing work must still reach the notes. These are the real
		// subjects of the three guards released in v0.1.95.
		{"feat(app status): warn when the local manifest is BEHIND the highest approved version (#412) (#413)", false,
			"a feat must reach the changelog"},
		{"fix(app submit): refuse a version at or below the highest APPROVED one (#412 piece 1) (#414)", false,
			"a fix must reach the changelog"},
		{"feat(app submit): refuse a submit from a dirty git work tree (#411 piece 1) (#415)", false,
			"a feat must reach the changelog"},

		// NEGATIVE CONTROLS against a pattern widened by dropping the colon
		// (`^chore`, `^docs`, `^test`), which would silently start hiding real
		// commits.
		//
		// 🔴 THESE THREE ARE THE ONLY ROWS THAT CATCH THAT MUTATION, and the
		// first draft of this test did not have them. It used `chorus:` and
		// `documentation:`, which FEEL like near-misses and are not: `^chore`
		// does not match `chorus` (`chore` vs `choru`), and `^docs` does not
		// match `documentation` (`docs` vs `docu`). The dropped-colon mutant was
		// applied — verified applied, not assumed — and the suite stayed GREEN.
		// A discriminating control has to share the type word EXACTLY and differ
		// only after it, which is what `chores` / `testing` / `docsite` do.
		//
		// `testing: …` is not hypothetical; it is an ordinary subject someone
		// will write, and under `^test` it would vanish from the notes.
		{"chores: quarterly cleanup", false,
			"^chore without the colon would swallow this"},
		{"testing: add benchmarks for the packager", false,
			"^test without the colon would swallow this"},
		{"docsite: rebuild the reference", false,
			"^docs without the colon would swallow this"},

		// Kept for prefix hygiene, though neither discriminates the mutation
		// above — see the note on the three rows before this one.
		{"chorus: unrelated word starting with chore-ish letters", false,
			"must not over-match on a prefix"},
		{"documentation: not a docs-typed commit", false,
			"must not over-match on a prefix"},
	} {
		if got := excluded(tc.subject); got != tc.want {
			t.Errorf("excluded(%q) = %v, want %v — %s\npatterns: %q",
				tc.subject, got, tc.want, tc.why, pats)
		}
	}
}
