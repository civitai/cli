package pkgzip

import (
	"os"
	"strings"
	"testing"
)

// 🔴 A COMMENT IS A CLAIM, AND THIS ONE WAS FALSE FOR AS LONG AS IT EXISTED.
//
// Until issue #423 the const block above MaxFiles/MaxBundleSizeBytes/… said:
//
//	These caps mirror the server-side submitVersion service so a package that
//	the CLI accepts will not be rejected on size grounds.
//
// and four error messages said `server max %d` / `server per-file max %d`. #423
// disproved all five in one measurement: an 8.20 MB compressed bundle cleared
// every cap here and the server refused it, while a 2.32 MB one was accepted.
// The constant was never the defect — the CLAIM was, because it told an author
// that a clean local preflight predicted the submit would land, so when it did
// not, nothing anywhere pointed at size.
//
// This guard is deliberately about the SOURCE FILE rather than about behaviour:
// there is no runtime observable for "the comment lies", which is exactly why
// the lie survived. It is narrow on purpose — it bans the specific assertion
// that these numbers are the SERVER's, and says nothing about how the caps are
// otherwise described.
func TestCapsDoNotClaimToMirrorTheServer(t *testing.T) {
	raw, err := os.ReadFile("pkgzip.go")
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: %v", err)
	}
	src := string(raw)

	// POSITIVE CONTROL. A read that returned the wrong file, or a truncated
	// one, would satisfy every "absent" check below while measuring nothing —
	// the reassuring zero. Anchor on something that must be in THIS file.
	for _, mustExist := range []string{"MaxBundleSizeBytes", "func Build(dir string)"} {
		if !strings.Contains(src, mustExist) {
			t.Fatalf("CONTROL failure, not a finding: pkgzip.go does not contain %q, so the searcher is "+
				"not reading the file this test is about and every verdict below is vacuous", mustExist)
		}
	}
	// NEGATIVE CONTROL: the matcher must be able to report a hit at all.
	const absent = "this phrase is not in pkgzip.go anywhere at all"
	if strings.Contains(src, absent) {
		t.Fatalf("CONTROL failure, not a finding: the negative control %q matched — the searcher cannot "+
			"distinguish present from absent", absent)
	}

	for _, banned := range []struct{ phrase, why string }{
		{"server max", "an error message calling MaxBundleSizeBytes/MaxFiles/MaxDecompressedSize a SERVER maximum"},
		{"server per-file max", "an error message calling MaxFileSizeBytes a SERVER maximum"},
		{"caps mirror the server", "the const block claiming these numbers mirror the platform's"},
		{"will not be rejected on size grounds", "the promise #423 disproved"},
	} {
		if strings.Contains(src, banned.phrase) {
			t.Errorf("internal/pkgzip/pkgzip.go contains %q — %s.\n"+
				"These caps are the CLI's OWN. The server's real bundle ceiling is bracketed by #423 to "+
				"(2.32 MB, 8.20 MB] and is not known here, so nothing in this package may describe a cap as "+
				"the server's, and no number may be guessed from inside that bracket. Say whose cap it is.",
				banned.phrase, banned.why)
		}
	}
}
