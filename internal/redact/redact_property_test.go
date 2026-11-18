package redact

import (
	"math/rand"
	"strings"
	"testing"
)

// urlPunct is every printable ASCII character that is neither a letter, a digit
// nor white space — i.e. everything that can sit next to a secret inside one
// URL token, including the ones a URL grammar gives meaning to.
const urlPunct = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"

const urlAlnum = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randFrom(rng *rand.Rand, alphabet string, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[rng.Intn(len(alphabet))]
	}
	return string(b)
}

// TestPropertyASecretInsideAURLTokenNeverSurvives is the property the URL rule
// is written to: a secret placed in a URL's userinfo, query or fragment is
// absent from the output WHATEVER printable, non-space characters surround it —
// quotes, backticks, extra `@`, `?`, `#`, `/`, brackets — and the removal is
// counted.
//
// The rule's two predecessors each named a set of characters at which a URL (or
// a query, or a userinfo) "ends", and each was defeated by a character inside
// that set appearing inside the secret's surroundings. So this test does not
// enumerate shapes; it generates them. The generator is seeded, so a failure
// reproduces, and the inputs deliberately contain no white space — white space
// is the one thing the rule treats as the end of a URL.
//
// POSITIVE CONTROL: run against the rule as it stood at 3fc8e2a (a terminator
// class per part), this test reported 1,358 of its 3,000 inputs leaking and 83
// more uncounted.
func TestPropertyASecretInsideAURLTokenNeverSurvives(t *testing.T) {
	rng := rand.New(rand.NewSource(830_2))
	schemes := []string{"https", "http", "postgres", "redis", "s3", "git+ssh", "amqp"}
	const n = 3000

	var leaks, uncounted int
	perMode := map[string]int{}
	for i := 0; i < n; i++ {
		// The secret is alphanumeric with a marker prefix, so it cannot be
		// produced by the surrounding random text and a substring search for it
		// is exact.
		secret := "Zq7S3cr3t" + randFrom(rng, urlAlnum, 10)
		pre := randFrom(rng, urlPunct+urlAlnum, rng.Intn(7))
		post := randFrom(rng, urlPunct+urlAlnum, rng.Intn(7))
		host := randFrom(rng, "abcdefghijklmnop", 6) + ".fieldstone.dev"
		scheme := schemes[rng.Intn(len(schemes))]

		var url, mode, wantKind string
		switch i % 3 {
		case 0:
			mode, wantKind = "userinfo", KindCredential
			url = scheme + "://" + pre + secret + post + "@" + host + "/v1/x"
		case 1:
			mode, wantKind = "query", KindURLQuery
			url = scheme + "://" + host + "/v1/x?" + pre + secret + post
		default:
			mode, wantKind = "fragment", KindURLQuery
			url = scheme + "://" + host + "/v1/x#" + pre + secret + post
		}
		in := "request to " + url + " failed with 500"

		r := New(Options{})
		out := r.Apply(in)
		perMode[mode]++

		if strings.Contains(out, secret) {
			leaks++
			if leaks <= 5 {
				t.Errorf("[%s #%d] the secret survived:\n in: %s\nout: %s", mode, i, in, out)
			}
			continue
		}
		counted := false
		for _, c := range r.Counts() {
			if c.Kind == wantKind && c.Count > 0 {
				counted = true
			}
		}
		if !counted {
			uncounted++
			if uncounted <= 5 {
				t.Errorf("[%s #%d] the secret is gone but no %s redaction was counted (%+v):\n in: %s\nout: %s",
					mode, i, wantKind, r.Counts(), in, out)
			}
		}
		// The text around the URL is untouched: only the URL token changes.
		if !strings.HasPrefix(out, "request to "+scheme+"://") || !strings.HasSuffix(out, " failed with 500") {
			t.Errorf("[%s #%d] text outside the URL token was changed:\n in: %s\nout: %s", mode, i, in, out)
		}
	}
	if leaks > 0 || uncounted > 0 {
		t.Errorf("%d of %d generated URLs leaked their secret; %d more removed it without counting it", leaks, n, uncounted)
	}
	// The generator exercised every mode.
	for _, mode := range []string{"userinfo", "query", "fragment"} {
		if perMode[mode] < n/4 {
			t.Fatalf("CONTROL failure: only %d input(s) in mode %q", perMode[mode], mode)
		}
	}
}
