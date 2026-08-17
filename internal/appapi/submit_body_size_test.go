package appapi

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

// TestSubmitBodySizeMatchesRealMarshal pins SubmitBodySize's arithmetic against
// the marshaller SubmitVersion actually uses.
//
// 🔴 THE EXPECTATION IS NOT DERIVED FROM THE IMPLEMENTATION. It comes from
// running the real json.Marshal over the real submitBody, which is what makes
// this a check on the constant rather than a restatement of it: change the
// envelope, the json tag, or add a field to submitBody, and this reddens.
//
// Lengths are pairwise distinct and cover every base64 padding residue (n%3 ==
// 0, 1, 2), because that residue is the only place EncodedLen can be wrong
// while looking right — a single length would be a claim about one residue
// dressed up as a claim about all three.
func TestSubmitBodySizeMatchesRealMarshal(t *testing.T) {
	for _, zipLen := range []int{0, 1, 2, 3, 4, 5, 97, 811, 1237, 2503, 4099, 6337, 65_537} {
		payload := make([]byte, zipLen)
		for i := range payload {
			payload[i] = byte(i*31 + 7)
		}
		want, err := json.Marshal(submitBody{
			BundleBase64: base64.StdEncoding.EncodeToString(payload),
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := SubmitBodySize(zipLen); got != len(want) {
			t.Errorf("SubmitBodySize(%d) = %d, but the real marshalled body is %d bytes",
				zipLen, got, len(want))
		}
	}
}

// TestSubmitBodySizeIsNotTheZipSize is the point of the function stated as a
// test: the number an author reads off `app submit` as "compressed" is NOT the
// number a request-body limit is applied to, and #423 is what that gap cost.
// Growth is ~4/3, so any non-trivial zip must produce a strictly larger body.
func TestSubmitBodySizeIsNotTheZipSize(t *testing.T) {
	for _, zipLen := range []int{1, 97, 811, 8_201_270} {
		if got := SubmitBodySize(zipLen); got <= zipLen {
			t.Errorf("SubmitBodySize(%d) = %d, which is not larger than the zip — base64 in JSON "+
				"cannot shrink anything, so this is not the quantity that goes on the wire", zipLen, got)
		}
	}
}
