package cmd

import (
	"slices"
	"strings"
	"testing"
)

// 🔴 THE SUBJECT MOVED; THE GUARDS DID NOT. Until this change the symptom-index
// guards in readme_troubleshooting_test.go and readme_submit_entry_block_test.go
// read README.md's `## Troubleshooting` section. That section is now a POINTER:
// the index itself is published at
//
//	https://developer.civitai.com/site/guide/cli-troubleshooting
//
// verified by CONTENT before the deletion rather than by heading — all 69
// symptom strings, all six bucket tables and the three cause cells the guards
// assert on were read back off the live page.
//
// A hosted page cannot be an oracle for a hermetic test, so the guards were
// RE-POINTED rather than deleted, at the ledger below: a VENDORED mirror of the
// published index, in the same spirit as the slot registry (AGENTS.md item 2),
// the token-scope bitmask (item 4) and the ready-ack emitter (item 11). The
// mechanism is exitcodes_doc.go's: a Go declaration is the authority.
//
// 🔴 THE PROPERTY THIS PRESERVES, and it is the only coverage in the suite that
// has it: A REWORDED ERROR MESSAGE IN THE GO SOURCE MUST REDDEN SOMETHING.
// TestPublishedTroubleshootingSymptomsExistInTheSource still searches every
// non-test .go/.tmpl/.js file under internal/, cmd/ and pkg/ for each string
// below, so rewording a message the published page quotes is still red — the
// failure now names the page to fix instead of a README row.
//
// 🔴 WHY THIS FILE MUST NOT BECOME A NON-TEST FILE. symptomSourceCorpus excludes
// `_test.go` on purpose: a corpus that included this file would find every
// symptom in the ledger itself and pass unconditionally. Moving these strings
// into a non-test .go file under internal/ silently makes the guard vacuous.
//
// ⚠ THE DECLARED RESIDUAL. Nothing in this repository can prove the ledger still
// matches the live page — that is the same residual listing_media_docs_test.go
// accepted when `### Listing media requirements` moved, and it is the price of a
// hermetic test over relocated prose. What the ledger DOES guarantee is the half
// that rots silently: every string it holds is still a string this CLI prints.
// A row the page adds without a ledger entry is simply unguarded, not misread.

// publishedTroubleshootingURL is where the index a reader searches now lives.
// Named once so every failure message below can hand over an actionable target.
const publishedTroubleshootingURL = "https://developer.civitai.com/site/guide/cli-troubleshooting"

// publishedTroubleshootingSymptomCount pins the ledger's SIZE exactly, and it
// replaces a FLOOR of 15 over a 69-string extraction.
//
// 🔴 THE FLOOR WAS THE ALL-OR-NOTHING HAZARD, not an anti-vacuity control worth
// carrying over unchanged. While the strings were EXTRACTED from markdown, 15
// answered "is the row extractor reading the right text?" — a real question. A
// literal Go slice cannot be mis-extracted, so that question is gone, and all
// the floor still did was permit 54 of the 69 strings to be deleted green and
// silent. That is exactly how a partial cut of the README section would have
// passed. An exact count cannot be traded that way.
//
// Changing the page is allowed; changing it SILENTLY is not. Add or remove a row
// here, move this number in the same commit, and say why.
const publishedTroubleshootingSymptomCount = 69

// publishedTroubleshootingSymptoms is column one of every row of the published
// index, already split on ` / ` and ellipsis-trimmed — i.e. exactly the strings
// the deleted documentedSymptoms() extractor returned from the README table.
var publishedTroubleshootingSymptoms = []string{
	"no token configured",
	"forbidden (403)",
	"not permitted for your account (403)",
	"under a moderator takedown (403)",
	"belongs to another account (403)",
	"Submit Apps:",
	"(token scope not reported by the server — Buzz capabilities unknown)",
	"not permitted to read this app's analytics (403)",
	"block lacks ai:write:budgeted scope",
	"the server can receive",
	"insufficient Buzz",
	"generation disabled",
	"rate limited (429)",
	"Civitai returned HTTP",
	"cannot derive a slug from",
	"cannot appear in a blockId",
	"is not valid UTF-8",
	"and the limit is",
	"refusing to overwrite. Scaffold somewhere else",
	"not found at project root",
	"is this an App project?",
	"the server rejected this store-listing lookup (400)",
	"the server rejected the image-upload request (400)",
	"image upload PUT failed",
	"the server rejected this store-listing change (400)",
	"there is no open revision to submit",
	"this listing is not live",
	"pass a URL or --clear, not both",
	"nothing to do — pass a repository URL to set the link, or --clear",
	"the source-repository URL is blank",
	"source-repository link comes from the",
	"no such directory — pass the path to an App project root",
	"is not a directory — pass the App project ROOT",
	"it did NOT check that the file is loaded",
	"nothing index.html loads reaches it",
	"no lockfile is committed",
	"is not a lockfile",
	"refusing to submit without --yes",
	"What this CLI sent",
	"What this CLI would have sent",
	"largest entries in the bundle",
	"Your repo may be behind what was last released",
	"Resubmitting the version that is already live is almost always an accident",
	"That version is approved but not live",
	"from a dirty git work tree",
	"that go into the bundle are not committed",
	"look like they hold credentials",
	"HEAD is on no remote",
	"refusing to withdraw without --yes",
	"could not read your Buzz balance",
	"refusing to spend Buzz without --yes",
	"--image requires --ecosystem",
	"interrupted while waiting",
	"model substituted",
	"The server reported:",
	"An indented line under a row is what the server recorded",
	"prompt:",
	"negative:",
	"the orchestrator often supplies no failure reason, so it may not say why",
	"has no approved App Block yet",
	"no such app for your account",
	"has no approved version yet",
	"no such submission",
	"is an OFFSITE app",
	"is ambiguous — it matches",
	"SHA256 mismatch for",
	"checksum mismatch for",
	"git is required for `civitai app pull`",
	"unexpected response from",
}

// publishedTroubleshootingCause is a row whose CAUSE cell — column two — is
// itself under guard, vendored verbatim from the published index.
//
// Only three rows need one. Column one is covered for all 60 rows by the
// symptom ledger above; the cause cell is the half that carries a CLAIM, and
// these are the three claims something in this package measures against reality.
type publishedTroubleshootingRow struct {
	// fragment is the row's column-one string, as the symptom ledger holds it.
	fragment string
	// cause is column two, verbatim.
	cause string
	// readMore is column three's link TARGET as the published page writes it.
	//
	// 🔴 IT IS CHECKABLE FOR SOME ROWS AND NOT OTHERS, AND THE SPLIT IS RECORDED
	// RATHER THAN GLOSSED. A target naming an anchor in THIS repository's README
	// is still verified by readmeHasAnchor — the published page links row 1 back
	// to `…/civitai/cli#exit-code-1`, and `## Exit codes` is deliberately still
	// in README.md, so that assertion is live. A target on the docs site is not
	// resolvable hermetically; it is recorded here and left to
	// TestREADMEExternalURLsResolve's network-gated sibling on the docs repo.
	readMore string
}

var publishedTroubleshootingRows = []publishedTroubleshootingRow{
	{
		fragment: "not found at project root",
		cause: "**`civitai app validate`** found no `block.manifest.json` in the directory you named — the " +
			"finding reads `block.manifest.json not found at project root <dir>`, which the terminal " +
			"wraps onto a second line for a long path (`--json` carries it as one `message` string). `app " +
			"submit` prints it too, because it validates first. `app submit --skip-validate` never prints " +
			"it, because it waives the validation that produces it — that run fails on the row below " +
			"instead. The path itself was fine, which is why this exits `1` and not `2`.",
		readMore: "https://github.com/civitai/cli#exit-code-1",
	},
	{
		fragment: "is this an App project?",
		cause: "The same cause, reported by a command that did not validate first: `civitai app listing …`, " +
			"which has to work out *which* app you mean from the working directory, and `app submit " +
			"--skip-validate`, which waived the check that produces the row above. `app validate` and a " +
			"plain `app submit` never print it, because validation reports the row above first. Run `app " +
			"listing` from the app directory, or name the app with `--slug` / `--dir`.",
		readMore: "https://developer.civitai.com/apps/guide/store-listing#which-app-a-listing-command-acts-on",
	},
	{
		fragment: "largest entries in the bundle",
		cause: "Not an error of its own: the CLI's account of the bundle, and the largest entries it was " +
			"made of. `What this CLI **sent**` prints under any error the upload call reports once the " +
			"request has gone out — **it does not claim to know why** — and not on a `401`/`403`/`429`. A " +
			"failure that never reached the connection never prints the past tense: no usable credential, " +
			"an unwritable config and a connection that never opened print neither block. `What this CLI " +
			"**would have** sent` is the ceiling refusal alone — it sends nothing either, and says so — " +
			"and that one is exact too: nothing was uploaded. A refusal that stops the submit before the " +
			"upload step (no `--yes`, a dirty tree, the version guard, a validation failure) prints " +
			"neither.",
		readMore: "https://developer.civitai.com/apps/guide/packaging",
	},
}

// publishedCauseFor returns the vendored cause cell for a row fragment.
//
// It FATALS rather than returning "" on a miss: an extractor that silently found
// nothing satisfies every "the text does not contain X" check and reports a
// serene pass. That is the reassuring zero this file's predecessor guarded
// against with a section lookup, and the guarantee is carried over here.
func publishedCauseFor(t *testing.T, fragment string) string {
	t.Helper()
	for _, r := range publishedTroubleshootingRows {
		if r.fragment == fragment {
			// POSITIVE CONTROL carried over from troubleshootingEntryBlockCause's
			// 80-byte floor: a cell this short is not a cause cell.
			if len(r.cause) < 80 {
				t.Fatalf("CONTROL failure: the vendored cause cell for %q is %d bytes (want >= 80), "+
					"so it is a truncated or wrong entry and every assertion on it is vacuous: %q",
					fragment, len(r.cause), r.cause)
			}
			return r.cause
		}
	}
	t.Fatalf("CONTROL failure: no vendored row carries the fragment %q, so every assertion on its "+
		"cause cell is asserting against an empty string. The published index is at %s; add the row "+
		"back rather than deleting the guard.", fragment, publishedTroubleshootingURL)
	return ""
}

// publishedReadMoreFor returns the vendored "Where to read more" target for a
// row fragment, fataling on a miss for the same reason publishedCauseFor does.
func publishedReadMoreFor(t *testing.T, fragment string) string {
	t.Helper()
	for _, r := range publishedTroubleshootingRows {
		if r.fragment == fragment {
			if r.readMore == "" {
				t.Fatalf("CONTROL failure: the vendored row for %q carries no readMore target, so the "+
					"link assertion would pass by absence", fragment)
			}
			return r.readMore
		}
	}
	t.Fatalf("CONTROL failure: no vendored row carries the fragment %q", fragment)
	return ""
}

// TestVendoredTroubleshootingLedgerIsWellFormed carries the two properties the
// deleted markdown extractor gave away for free.
//
// 🔴 NEITHER IS AUTOMATIC IN A SLICE, WHICH IS WHY THIS EXISTS. (1) The old
// troubleshootingRowFor counted matches and FATALED on two rows quoting one
// fragment, because a guard that picks one of them says nothing about the other.
// publishedTroubleshootingRows is a slice and a duplicate would silently resolve
// to the first entry. (2) The old lookup matched a row's real first column, so a
// cause cell could not exist for a row the index does not have; here the two
// ledgers are separate declarations and can disagree.
func TestVendoredTroubleshootingLedgerIsWellFormed(t *testing.T) {
	seen := map[string]int{}
	for _, r := range publishedTroubleshootingRows {
		seen[r.fragment]++
	}
	// POSITIVE CONTROL: an empty ledger satisfies every loop below in silence.
	if len(publishedTroubleshootingRows) == 0 {
		t.Fatal("CONTROL failure: publishedTroubleshootingRows is empty, so this test asserts nothing")
	}
	for frag, n := range seen {
		if n > 1 {
			t.Errorf("%d vendored rows carry the fragment %q. Every lookup resolves to the FIRST of "+
				"them, so the others would drift unwatched — merge them, or make each fragment name "+
				"exactly one row.", n, frag)
		}
	}
	for _, r := range publishedTroubleshootingRows {
		if !slices.Contains(publishedTroubleshootingSymptoms, r.fragment) {
			t.Errorf("a cause cell is vendored for the fragment %q, but publishedTroubleshootingSymptoms "+
				"holds no such row. Either the symptom ledger lost a row (the guards that search the Go "+
				"source for it then stop watching that string) or this fragment was mistyped — the two "+
				"ledgers describe the same table and cannot disagree.", r.fragment)
		}
	}
}

// readmeGitHubBase is how the published pages spell a link back into this
// repository's README. A row pointing there is asserting that a heading in
// README.md exists, which is a claim this repository CAN check.
const readmeGitHubBase = "https://github.com/civitai/cli"

// readmeAnchorTarget reports whether a published link target names a heading in
// THIS repository's README, and if so returns the `#anchor` to resolve.
//
// 🔴 IT MUST NOT TREAT AN OFF-SITE URL THAT MERELY CONTAINS A `#` AS IN-REPO.
// `…/apps/guide/store-listing#which-app-…` has a fragment too, and resolving it
// against README.md would report a live docs link as a dead anchor — a guard
// that fails on correct input is one somebody deletes. So the host+path must
// match exactly, or the target must be a bare `#anchor`.
func readmeAnchorTarget(target string) (string, bool) {
	if strings.HasPrefix(target, "#") {
		return target, true
	}
	if rest, ok := strings.CutPrefix(target, readmeGitHubBase+"#"); ok {
		return "#" + rest, true
	}
	return "", false
}
