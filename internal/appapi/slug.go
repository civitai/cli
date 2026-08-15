package appapi

import "strings"

// THE SLUG-COMPARISON PREDICATE — ONE COPY, FOUR CALLERS.
//
// "Is this submissions-route row the app I am talking about?" was answered four
// different ways in this binary, and the four disagreed:
//
//	site                                          compare
//	internal/cmd/approved_version.go   (#412)     EqualFold + TrimSpace
//	internal/cmd/apps.go     ownedSubmission      ==
//	internal/appapi/appblocks.go latestMatching   !=
//	internal/cmd/app_status.go warnLocalVersionDrift  !=
//
// The normalising one was written LAST (#414) and deliberately, after an audit
// showed that an exact match makes the #412 version guard a SILENT no-op on a
// cased or padded blockId: a non-matching slug lands in the "no approved rows"
// branch, which proceeds without a word because that is also what a genuine
// first submit looks like. The other three never got the same treatment, so the
// binary held one site that had reasoned about the hazard and three that had
// not — the usual shape, wrong at N-1 sites in the same direction.
//
// This file is the consolidation. It lives in appapi and not in internal/cmd
// because appapi owns Submission.BlockID — the field on one side of all four
// comparisons — and because appapi cannot import internal/cmd, while every cmd
// caller already imports appapi. The predicate is a statement about THIS
// route's contract, so it belongs next to the client that speaks it.
//
// The ledger that keeps the count at four is TestBlockIDComparisonsUseSameSlug
// in internal/cmd/slug_predicate_ledger_test.go: it scans both packages for a
// blockId compared with == or != against anything but the empty string, and
// fails on a fifth site the day it is written.

// SameSlug reports whether two spellings name the same App Block slug.
//
// 🔴 IT NORMALISES, AND THAT IS DEFENCE IN DEPTH AGAINST AN UNDOCUMENTED SERVER
// CHANGE, NOT A LIVE HAZARD. This claim is carried over verbatim from #414's
// delta audit and must not be upgraded while being moved. The route as it
// stands today cannot hand back a mis-cased blockId:
// civitai:src/pages/api/v1/blocks/submissions.ts filters with
// `where.slug = blockId` (an exact Prisma match) and echoes `blockId: row.slug`
// from the same non-nullable column, so every row it returns matches the value
// asked for byte-for-byte. In the mis-casing scenario the server returns ZERO
// rows, not mis-cased ones.
//
// What the normalisation buys is the asymmetry: status and deployState were
// already compared case- and whitespace-insensitively while the slug was
// compared byte-for-byte, and the slug is the field whose mismatch is SILENT.
// If that server contract ever changed, the #412 feature would switch off on
// exactly the app it exists to protect, with no output to notice. The check is
// two string compares; keeping it is cheap insurance, and claiming it closes a
// hazard that exists today is not true.
//
// 🔴 ONE CALLER'S ARGUMENT IS STRONGER THAN THAT, AND IT IS NOT THE SERVER'S.
// warnLocalVersionDrift compares a LOCAL block.manifest.json's blockId against
// a row's, and manifest.Load is a bare json.Unmarshal — it does not validate
// against schema/app-block.manifest.schema.json. So on that one side an
// unnormalised value is reachable by hand-editing a file, without any server
// contract having to change. That is a shorter path to a mis-spelling than the
// three row-vs-request sites have; it is still not a demonstrated live break,
// because a manifest spelled that way fails `civitai app submit`'s own
// validation. See the comment at that call site.
//
// 🔴 WHY THIS CANNOT MERGE TWO DIFFERENT APPS, which is the question a widening
// predicate has to answer. A valid slug matches
// `^[a-z][a-z0-9-]*[a-z0-9]$` — the pattern in
// schema/app-block.manifest.schema.json, which the server's own validator
// (civitai:src/server/services/block-manifest-validator.service.ts) shares. It
// admits no uppercase and no whitespace, so EqualFold+TrimSpace is the IDENTITY
// MAP on the set of valid slugs: it can only ever join a mis-spelling to the
// one valid slug it is a mis-spelling OF. What it newly admits is therefore
// exactly the intended set — invalid spellings of the same app — and never a
// second app.
//
// 🔴 EMPTY IS NEVER A MATCH, INCLUDING EMPTY-AGAINST-EMPTY. The `==` this
// replaces answered TRUE for two empty strings, so a row with no blockId
// matched a request with no slug and read as "yes, that is your app". No slug
// names an app; nothing downstream of any of the four call sites wants that
// answer, and app_status.go already carried a hand-written `m.BlockID == ""`
// guard for precisely this. It is folded in here so the four sites cannot
// disagree about it either. TrimSpace runs first, so an all-whitespace blockId
// is empty for this purpose too — which the hand-written guard did not catch.
func SameSlug(a, b string) bool {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	return strings.EqualFold(a, b)
}
