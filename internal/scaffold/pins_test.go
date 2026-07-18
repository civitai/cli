package scaffold

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestParseSemver covers the concrete X.Y.Z parser used by every pin helper:
// happy path, prerelease/build-metadata stripping, and the loud-failure inputs
// (wrong arity, non-numeric components) that a `~`/`>=`/`x` operator produces.
func TestParseSemver(t *testing.T) {
	cases := []struct {
		in               string
		wantMaj, wantMin int
		wantPat          int
		wantErr          bool
	}{
		{in: "0.25.3", wantMaj: 0, wantMin: 25, wantPat: 3},
		{in: "1.4.0", wantMaj: 1, wantMin: 4, wantPat: 0},
		{in: "0.0.7", wantMaj: 0, wantMin: 0, wantPat: 7},
		{in: "10.20.30", wantMaj: 10, wantMin: 20, wantPat: 30},
		// Prerelease/build metadata is dropped before parsing.
		{in: "1.2.3-beta.1", wantMaj: 1, wantMin: 2, wantPat: 3},
		{in: "1.2.3+build.5", wantMaj: 1, wantMin: 2, wantPat: 3},
		{in: "2.0.0-rc1+exp", wantMaj: 2, wantMin: 0, wantPat: 0},
		// Failure inputs.
		{in: "1.2", wantErr: true},     // too few components
		{in: "1.2.3.4", wantErr: true}, // too many components
		{in: "1.x.0", wantErr: true},   // non-numeric (an `x` range)
		{in: "~1.2.0", wantErr: true},  // operator not stripped by parseSemver
		{in: "", wantErr: true},        // empty
		{in: "abc", wantErr: true},     // garbage
		{in: "1..3", wantErr: true},    // empty middle component
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			maj, min, pat, err := parseSemver(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("parseSemver(%q) = (%d,%d,%d), want error", c.in, maj, min, pat)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSemver(%q) unexpected error: %v", c.in, err)
			}
			if maj != c.wantMaj || min != c.wantMin || pat != c.wantPat {
				t.Errorf("parseSemver(%q) = (%d,%d,%d), want (%d,%d,%d)",
					c.in, maj, min, pat, c.wantMaj, c.wantMin, c.wantPat)
			}
		})
	}
}

// TestCmpSemver checks the three-way version comparator's sign on each
// component's precedence.
func TestCmpSemver(t *testing.T) {
	cases := []struct {
		name                   string
		a0, a1, a2, b0, b1, b2 int
		wantSign               int // -1, 0, +1
	}{
		{"equal", 1, 2, 3, 1, 2, 3, 0},
		{"major lower", 0, 9, 9, 1, 0, 0, -1},
		{"major higher", 2, 0, 0, 1, 9, 9, +1},
		{"minor lower", 1, 2, 9, 1, 3, 0, -1},
		{"minor higher", 1, 3, 0, 1, 2, 9, +1},
		{"patch lower", 1, 2, 3, 1, 2, 4, -1},
		{"patch higher", 1, 2, 4, 1, 2, 3, +1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := cmpSemver(c.a0, c.a1, c.a2, c.b0, c.b1, c.b2)
			if sign(got) != c.wantSign {
				t.Errorf("cmpSemver(%d.%d.%d, %d.%d.%d) = %d (sign %d), want sign %d",
					c.a0, c.a1, c.a2, c.b0, c.b1, c.b2, got, sign(got), c.wantSign)
			}
		})
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return +1
	default:
		return 0
	}
}

// TestDesiredPin locks the pre-1.0 caret invariant: for a 0.0.Z package the
// caret pins the PATCH (`^0.0.Z`), otherwise it pins the leftmost non-zero
// component's line (`^X.Y.0`).
func TestDesiredPin(t *testing.T) {
	cases := []struct {
		published string
		want      string
		wantErr   bool
	}{
		// 0.Y.Z (Y>0): caret locks the minor -> pin the minor line at .0.
		{published: "0.25.3", want: "^0.25.0"},
		{published: "0.25.0", want: "^0.25.0"},
		{published: "0.1.99", want: "^0.1.0"},
		// X>0: caret locks the major -> pin the minor line at .0.
		{published: "1.4.0", want: "^1.4.0"},
		{published: "1.4.9", want: "^1.4.0"},
		{published: "2.0.0", want: "^2.0.0"},
		// 0.0.Z: caret locks the PATCH -> must pin the exact patch, never ^0.0.0.
		{published: "0.0.7", want: "^0.0.7"},
		{published: "0.0.1", want: "^0.0.1"},
		{published: "0.0.0", want: "^0.0.0"},
		// Prerelease metadata is stripped before deriving the pin.
		{published: "0.25.3-beta.1", want: "^0.25.0"},
		// Unparseable published version -> error.
		{published: "1.2", wantErr: true},
		{published: "not-a-version", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.published, func(t *testing.T) {
			got, err := DesiredPin(c.published)
			if c.wantErr {
				if err == nil {
					t.Fatalf("DesiredPin(%q) = %q, want error", c.published, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("DesiredPin(%q) unexpected error: %v", c.published, err)
			}
			if got != c.want {
				t.Errorf("DesiredPin(%q) = %q, want %q", c.published, got, c.want)
			}
		})
	}
}

// TestDesiredPinAdmitsItsOwnPublished is a property check: the pin DesiredPin
// derives for a version must, by construction, admit that same version — the
// guard/bumper agree-by-construction invariant.
func TestDesiredPinAdmitsItsOwnPublished(t *testing.T) {
	for _, v := range []string{"0.25.3", "1.4.9", "0.0.7", "2.0.0", "0.1.0", "0.0.1"} {
		pin, err := DesiredPin(v)
		if err != nil {
			t.Fatalf("DesiredPin(%q): %v", v, err)
		}
		ok, err := CaretAdmits(pin, v)
		if err != nil {
			t.Fatalf("CaretAdmits(%q, %q): %v", pin, v, err)
		}
		if !ok {
			t.Errorf("DesiredPin(%q)=%q does not admit %q", v, pin, v)
		}
	}
}

// TestCaretAdmits exercises the full pre-1.0 caret window semantics across the
// three caret regimes (X>0, 0.Y>0, 0.0.Z) plus the bare (caret-less) exact-pin
// path and the error branches.
func TestCaretAdmits(t *testing.T) {
	cases := []struct {
		name      string
		pin       string
		published string
		want      bool
		wantErr   bool
	}{
		// ^X.Y.Z, X>0 -> [X.Y.Z, (X+1).0.0)
		{name: "major caret admits same", pin: "^1.4.0", published: "1.4.0", want: true},
		{name: "major caret admits higher minor", pin: "^1.4.0", published: "1.9.5", want: true},
		{name: "major caret admits up to next major bound", pin: "^1.4.0", published: "1.999.999", want: true},
		{name: "major caret rejects next major", pin: "^1.4.0", published: "2.0.0", want: false},
		{name: "major caret rejects below lower bound", pin: "^1.4.5", published: "1.4.4", want: false},

		// ^0.Y.Z, Y>0 -> [0.Y.Z, 0.(Y+1).0) — the load-bearing pre-1.0 case:
		// caret LOCKS the minor.
		{name: "minor caret admits same", pin: "^0.25.0", published: "0.25.0", want: true},
		{name: "minor caret admits higher patch", pin: "^0.25.0", published: "0.25.9", want: true},
		{name: "minor caret REJECTS next minor (the stale-pin trap)", pin: "^0.25.0", published: "0.26.0", want: false},
		{name: "minor caret rejects below lower bound", pin: "^0.25.3", published: "0.25.2", want: false},
		{name: "minor caret rejects a major bump", pin: "^0.25.0", published: "1.0.0", want: false},

		// ^0.0.Z -> [0.0.Z, 0.0.(Z+1)) — caret locks the PATCH.
		{name: "patch caret admits same", pin: "^0.0.7", published: "0.0.7", want: true},
		{name: "patch caret REJECTS next patch", pin: "^0.0.7", published: "0.0.8", want: false},
		{name: "patch caret rejects below lower bound", pin: "^0.0.7", published: "0.0.6", want: false},

		// Bare (caret-less) pin: must equal published exactly.
		{name: "bare pin equal admits", pin: "1.2.3", published: "1.2.3", want: true},
		{name: "bare pin unequal rejects", pin: "1.2.3", published: "1.2.4", want: false},

		// Error branches.
		{name: "bad published", pin: "^0.25.0", published: "1.2", wantErr: true},
		{name: "bad pin", pin: "^1.x", published: "1.2.3", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := CaretAdmits(c.pin, c.published)
			if c.wantErr {
				if err == nil {
					t.Fatalf("CaretAdmits(%q, %q) = %v, want error", c.pin, c.published, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("CaretAdmits(%q, %q) unexpected error: %v", c.pin, c.published, err)
			}
			if got != c.want {
				t.Errorf("CaretAdmits(%q, %q) = %v, want %v", c.pin, c.published, got, c.want)
			}
		})
	}
}

// TestFetchNpmLatest drives the registry client against an httptest server via
// the npmRegistryBase seam: success, 404/410 -> ErrPkgNotFound, other non-200 ->
// transient error, and malformed/empty payloads.
func TestFetchNpmLatest(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		wantVersion string
		wantErrIs   error // errors.Is target (nil = "any non-nil error" when wantErr)
		wantErr     bool
	}{
		{name: "ok", status: 200, body: `{"version":"0.25.3"}`, wantVersion: "0.25.3"},
		{name: "404 not found", status: 404, body: `{}`, wantErr: true, wantErrIs: ErrPkgNotFound},
		{name: "410 gone", status: 410, body: `{}`, wantErr: true, wantErrIs: ErrPkgNotFound},
		{name: "500 transient", status: 500, body: `oops`, wantErr: true},
		{name: "429 rate limit transient", status: 429, body: ``, wantErr: true},
		{name: "empty version", status: 200, body: `{"version":""}`, wantErr: true},
		{name: "malformed json", status: 200, body: `{not json`, wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Sanity-check the request path the client builds.
				if got, want := r.URL.Path, "/@civitai/app-sdk/latest"; got != want {
					t.Errorf("request path = %q, want %q", got, want)
				}
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			}))
			defer srv.Close()

			restore := npmRegistryBase
			npmRegistryBase = srv.URL
			defer func() { npmRegistryBase = restore }()

			got, err := FetchNpmLatest("@civitai/app-sdk")
			if c.wantErr {
				if err == nil {
					t.Fatalf("FetchNpmLatest = %q, want error", got)
				}
				if c.wantErrIs != nil && !errors.Is(err, c.wantErrIs) {
					t.Errorf("FetchNpmLatest error = %v, want errors.Is %v", err, c.wantErrIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("FetchNpmLatest unexpected error: %v", err)
			}
			if got != c.wantVersion {
				t.Errorf("FetchNpmLatest = %q, want %q", got, c.wantVersion)
			}
		})
	}
}

// TestFetchNpmLatestConnErr covers the transport-error branch (the server is
// closed before the request), which returns the raw client.Get error.
func TestFetchNpmLatestConnErr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening now

	restore := npmRegistryBase
	npmRegistryBase = url
	defer func() { npmRegistryBase = restore }()

	if _, err := FetchNpmLatest("@civitai/app-sdk"); err == nil {
		t.Fatal("FetchNpmLatest against a closed server should error")
	}
}

// TestCivitaiPinRe checks the pin extractor matches @civitai/* entries (and only
// those) in package.json bytes, capturing the package and its pin.
func TestCivitaiPinRe(t *testing.T) {
	src := `{
  "dependencies": {
    "@civitai/app-sdk": "^0.25.0",
    "@civitai/blocks-react": "^0.32.0",
    "react": "^18.2.0",
    "@types/node": "^20.0.0"
  }
}`
	got := map[string]string{}
	for _, m := range CivitaiPinRe.FindAllStringSubmatch(src, -1) {
		got[m[1]] = m[2]
	}
	want := map[string]string{
		"@civitai/app-sdk":      "^0.25.0",
		"@civitai/blocks-react": "^0.32.0",
	}
	if len(got) != len(want) {
		t.Fatalf("CivitaiPinRe matched %d pins, want %d: %v", len(got), len(want), got)
	}
	for pkg, pin := range want {
		if got[pkg] != pin {
			t.Errorf("pin for %q = %q, want %q", pkg, got[pkg], pin)
		}
	}
	if _, ok := got["react"]; ok {
		t.Error("CivitaiPinRe should not match non-@civitai packages")
	}
}
