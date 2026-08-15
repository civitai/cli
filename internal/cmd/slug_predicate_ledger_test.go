package cmd

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// THE SLUG-PREDICATE LEDGER — one definition, and a guard that notices a second.
//
// The defect this PR fixed was not that any single site was wrong. It was that
// ONE question had FOUR answers, three of which had never reasoned about the
// hazard the fourth was written for. Consolidating them fixes today; this ledger
// is what stops the count going back up, because the next author to write
// `row.BlockID == slug` will do it in a file none of the behavioural cases
// above touch, and every one of them will stay green.
//
// 🔴 IT IS A ZERO-OFFENDER ASSERTION, NOT A HAND-MAINTAINED LIST OF SITES. A
// list of names covers the sites somebody thought of — the failure mode the
// submissions-route ledger (newest_row_pick_test.go) was rewritten to escape.
// The rule here is structural: in the two packages that speak the submissions
// route, a blockId may be compared with == or != ONLY against the empty string.
// Everything else is an identity question and belongs to appapi.SameSlug.
//
// 🔴 WHY `== ""` AND `== nil` ARE ALLOWED AND NOT AN OVERSIGHT. "Is this field
// set?" is a different question from "do these two spellings name the same
// app", and it has no normalisation to get wrong — SameSlug's own empty guard
// is written in terms of it. FOURTEEN such presence checks exist in these two
// packages today — measured, by running this scan with the floor set above the
// real count and reading the number back — across app_dev_tunnel.go,
// app_status.go, app_listing.go, app_metrics.go, app_pull.go, appblocks.go and
// listing.go. Flagging them would make the guard noise, which is the one thing
// a permanently-red gate reliably achieves.
//
// The `nil` half is not a widening of convenience: `Submission.AppBlockID` is a
// `*string` and — unlike `BlockID` — is NOT a slug. It is the app-block's
// server-side id, nulled until a version is APPROVED, and all three of its
// comparisons in this tree (app_listing.go, app_metrics.go, app_pull.go) ask
// only whether it is populated. A slug field would never be compared to nil; if
// one ever is, the identifier is a pointer and the comparison is still a
// presence check.
//
// 🔴 WHAT IT CANNOT SEE, measured rather than waved at. It is a line scanner
// over source text, so: a comparison split across lines; a comparison routed
// through a local copy (`got := row.BlockID; got == slug`); `strings.EqualFold`
// open-coded directly, which is the RIGHT answer spelled the wrong way and is
// therefore the cheapest evasion; a slug field that is not named `*blockId*`;
// and any package other than the two ledgered ones. The last is bounded by the
// same thing that bounds the route ledger — a third package would have to
// obtain a Submission, and appapi is the only client of the route. The
// EqualFold evasion is deliberately left open: it produces correct behaviour
// and a duplicated definition, which is a review nit, not the silent defect
// this guard exists for.

// blockIDComparison matches a blockId-ish identifier compared with == or !=,
// capturing the right-hand side so a presence check can be told from an identity
// check.
//
// The identifier class allows `.`, `[`, `]` and `*` so a selector, an index and
// a pointer deref are all caught: `subs[i].BlockID`, `*m.AppBlockID`.
//
// 🔴 THERE IS NO `\b` BEFORE `block`, AND THAT IS THE WHOLE POINT. The first
// draft read `\bblock[A-Za-z0-9_]*id` and silently could not see `AppBlockID`:
// Go's `\b` is an ASCII word boundary, and inside `AppBlockID` the character
// before `B` is `p` — both word characters, so no boundary exists there. The
// scanner still reported "no offenders" over the real tree, because the sites it
// COULD see were already fixed. It was the calibration case below that caught
// it, which is the reason that case is written as a deref rather than as another
// `BlockID`: a control built from the shape you already handle proves nothing.
var blockIDComparison = regexp.MustCompile(`(?i)([A-Za-z0-9_.\[\]*]*block[A-Za-z0-9_]*id)\s*(==|!=)\s*(\S+)`)

// slugLedgerHit is one comparison the scanner found.
type slugLedgerHit struct {
	file, line, rhs string
}

// scanBlockIDComparisons returns every blockId comparison in src, split into the
// allowed presence checks and the identity checks that must go through SameSlug.
//
// Full-line comments are skipped: this file, slug.go and three call sites all
// discuss `m.BlockID == ""` in prose, and a guard that flags its own
// documentation is a guard people delete.
func scanBlockIDComparisons(file, src string) (identity, presence []slugLedgerHit) {
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		for _, m := range blockIDComparison.FindAllStringSubmatch(line, -1) {
			rhs := strings.TrimRight(m[3], `,;){`)
			hit := slugLedgerHit{file: file, line: trimmed, rhs: rhs}
			if rhs == `""` || rhs == "nil" {
				presence = append(presence, hit)
				continue
			}
			identity = append(identity, hit)
		}
	}
	return identity, presence
}

// TestBlockIDComparisonsUseSameSlug is the ledger. Zero identity comparisons,
// across both packages that speak the submissions route.
func TestBlockIDComparisonsUseSameSlug(t *testing.T) {
	var identity, presence []slugLedgerHit
	for prefix := range submissionsLedgerDirs {
		for name, src := range submissionsPkgSources(t, prefix) {
			i, p := scanBlockIDComparisons(prefix+"/"+name, src)
			identity = append(identity, i...)
			presence = append(presence, p...)
		}
	}

	// 🔴 THE POSITIVE CONTROL, AND IT COMES FIRST. "0 offenders" and "the regex
	// matched nothing" are the same output, and the second is what a renamed
	// field or a mis-set working directory produces. The presence checks are the
	// known-non-zero population this scan must be able to see: if THEY are gone,
	// the zero below is a fact about the pattern, not about the code.
	//
	// The floor sits below the measured 14 on purpose — it is a wired-up check,
	// not a census, and pinning it exactly would turn every legitimate deletion
	// of a nil check into a failure of the SLUG ledger, which is how a gate
	// stops being read.
	const presenceFloor = 10
	if len(presence) < presenceFloor {
		t.Fatalf("the scanner found only %d blockId presence check(s) (floor %d) — it is not seeing the "+
			"source it claims to check, so its 'no offenders' verdict means nothing", len(presence), presenceFloor)
	}

	if len(identity) == 0 {
		return
	}
	sort.Slice(identity, func(a, b int) bool { return identity[a].file < identity[b].file })
	var b strings.Builder
	for _, h := range identity {
		b.WriteString("\n  " + h.file + ": " + h.line)
	}
	t.Fatalf("%d blockId identity comparison(s) bypass appapi.SameSlug:%s\n\n"+
		"A blockId compared with == or != against anything but \"\" is asking 'are these the same app?', "+
		"and that question has ONE answer in this binary: appapi.SameSlug (internal/appapi/slug.go). "+
		"Open-coding it is how the four copies that this ledger exists to prevent came to disagree — three "+
		"of them exact, one normalising, and the whole suite green over a cased or padded blockId at all four. "+
		"If this comparison genuinely is NOT an identity question, say so at the call site and widen the "+
		"presence rule here on purpose.", len(identity), b.String())
}

// TestBlockIDComparisonScannerIsCalibrated is the negative control: the scanner
// must go RED on a source that violates the rule, and stay quiet on the shapes
// it deliberately allows. Without this, TestBlockIDComparisonsUseSameSlug is a
// claim about a regex nobody has watched match.
func TestBlockIDComparisonScannerIsCalibrated(t *testing.T) {
	for _, tc := range []struct {
		name         string
		src          string
		wantIdentity int
		wantPresence int
		why          string
	}{
		{
			name:         "the exact compare this PR removed, cmd shape",
			src:          "func f() {\n\tif subs[i].BlockID == slug {\n\t\treturn\n\t}\n}\n",
			wantIdentity: 1,
			why:          "internal/cmd/apps.go's original line must be caught, or the ledger could not have found it",
		},
		{
			name:         "the exact compare this PR removed, appapi shape",
			src:          "func f() {\n\tif s.BlockID != slug || s.Version != version {\n\t\tcontinue\n\t}\n}\n",
			wantIdentity: 1,
			why:          "internal/appapi/appblocks.go's original line, with a second unrelated compare on it",
		},
		{
			name:         "the two-row compare, both clauses",
			src:          "func f() {\n\tif m.BlockID == \"\" || m.BlockID != s.BlockID {\n\t\treturn\n\t}\n}\n",
			wantIdentity: 1,
			wantPresence: 1,
			why:          "app_status.go's original line: one presence check and one identity check, told apart",
		},
		{
			name:         "a pointer deref",
			src:          "func f() {\n\tif *subs[i].AppBlockID != want {\n\t\treturn\n\t}\n}\n",
			wantIdentity: 1,
			why:          "AppBlockID has no word boundary before Block; a \\bBlockID\\b pattern would miss it",
		},
		{
			name:         "presence checks are allowed",
			src:          "func f() {\n\tif blockID == \"\" {\n\t\treturn\n\t}\n\tif *subs[i].AppBlockID != \"\" {\n\t\treturn\n\t}\n}\n",
			wantPresence: 2,
			why:          "'is it set' is a different question with no normalisation to get wrong",
		},
		{
			name:         "nil checks on the nullable AppBlockID are allowed",
			src:          "func f() {\n\tif subs[i].AppBlockID != nil && *subs[i].AppBlockID != \"\" {\n\t\treturn\n\t}\n}\n",
			wantPresence: 2,
			why: "AppBlockID is a *string server id, not a slug, and both clauses ask only whether it is " +
				"populated — flagging them would make the ledger permanently red on correct code",
		},
		{
			name:         "a nil check does not license an identity check beside it",
			src:          "func f() {\n\tif sub.AppBlockID != nil && sub.BlockID == slug {\n\t\treturn\n\t}\n}\n",
			wantIdentity: 1,
			wantPresence: 1,
			why:          "the nil allowance is per-comparison, not per-line",
		},
		{
			name: "prose is not code",
			src: "// The hand-written `m.BlockID == \"\"` clause and m.BlockID != s.BlockID are discussed here.\n" +
				"func f() {}\n",
			why: "a guard that flags its own documentation gets deleted",
		},
		{
			name:         "the fixed call sites are clean",
			src:          "func f() {\n\tif !appapi.SameSlug(subs[i].BlockID, slug) {\n\t\treturn\n\t}\n\tif !SameSlug(s.BlockID, slug) || s.Version != version {\n\t\tcontinue\n\t}\n}\n",
			wantIdentity: 0,
			why:          "the POST-fix shape must produce zero, or the ledger fails on the very code it certifies",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			identity, presence := scanBlockIDComparisons("synthetic.go", tc.src)
			if len(identity) != tc.wantIdentity {
				t.Errorf("identity comparisons = %d, want %d — %s\nhits: %+v", len(identity), tc.wantIdentity, tc.why, identity)
			}
			if len(presence) != tc.wantPresence {
				t.Errorf("presence comparisons = %d, want %d — %s\nhits: %+v", len(presence), tc.wantPresence, tc.why, presence)
			}
		})
	}
}

// TestSameSlugHasExactlyOneDefinition closes the evasion the ledger above cannot
// see from the comparison side: re-deriving the predicate under another name.
// The consolidation's whole claim is ONE definition, so the count is asserted
// directly rather than inferred from the absence of `==`.
func TestSameSlugHasExactlyOneDefinition(t *testing.T) {
	var defs []string
	for prefix := range submissionsLedgerDirs {
		for name, src := range submissionsPkgSources(t, prefix) {
			if strings.Contains(src, "func SameSlug(") || strings.Contains(src, "func sameSlug(") {
				defs = append(defs, prefix+"/"+name)
			}
		}
	}
	sort.Strings(defs)
	if len(defs) != 1 || defs[0] != "appapi/slug.go" {
		t.Fatalf("SameSlug is defined in %v, want exactly [appapi/slug.go].\n"+
			"A second definition is the four-copies bug returning under a new name — the comparison-side "+
			"ledger cannot see it, because a duplicate definition produces no bare == at all.", defs)
	}
}
