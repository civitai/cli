package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/manifest"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// This file is the behavioural half of issue #637: the past-tense block must be
// UNREACHABLE for a failure whose request never reached the connection.
//
// ⚠ "never reached the connection", NOT "nothing left this machine" — the second
// is the retracted wording, and this header carried it for a round after the
// code stopped meaning it. A transfer cut short DID put bytes on the wire, is
// not tagged, and prints the block; what the block says about the size is an
// upper bound, which is why printSubmitSizeDiagnosis says "up to".
//
// 🔴 IT DRIVES THE REAL appapi.Client, NOT THE FAKE Submitter, AND THAT IS THE
// WHOLE POINT. The defect lives between internal/auth, internal/appapi and
// internal/cmd — a fake Submitter returns whatever error the test hands it, so
// every one of these cases would pass vacuously against a fake while the real
// chain lied. RULES.md: "the defect lives in the SEAM nobody owns."

// failingTokenSource is the measured #637 path: internal/auth/source.go returns
// `persist refreshed tokens: %w` — UNTAGGED — when the OAuth refresh succeeds
// over the network but writing ~/.config/civitai/config.yaml fails (NFS-soft,
// sshfs, CIFS, read-only fs; appblocks.go calls that chain "Reachable, not
// theoretical"). authedDoWith returns on Tokens.Token() before doOnceWith
// builds anything, so ZERO bytes reach the network.
type failingTokenSource struct{ err error }

func (f failingTokenSource) Token(context.Context) (string, error)   { return "", f.err }
func (f failingTokenSource) Refresh(context.Context) (string, error) { return "", f.err }

// uploadRecorder is an httptest server that counts the submit requests it
// actually received, plus the status and body it answers with.
type uploadRecorder struct {
	*httptest.Server
	hits atomic.Int64
}

func newUploadRecorder(t *testing.T, status int, body string) *uploadRecorder {
	t.Helper()
	rec := &uploadRecorder{}
	rec.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.hits.Add(1)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(rec.Server.Close)
	return rec
}

// doUploadStderr runs doUpload against a real client and returns stderr plus the
// error, so each case below asserts on what an author would actually see.
func doUploadStderr(t *testing.T, client appapi.Submitter) (string, error) {
	t.Helper()
	var errBuf bytes.Buffer
	c := &cobra.Command{}
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&errBuf)
	m := &manifest.Manifest{BlockID: "demo", Version: "0.1.0", Name: "Demo"}
	err := doUpload(c, client, zipPayload, m, "https://civitai.com/", appapi.Provenance{})
	return errBuf.String(), err
}

// pastTenseBlock is the header printSubmitSizeDiagnosis opens with. Taken from
// app_submit.go rather than retyped: a reword there must move this assertion.
const pastTenseBlock = "What this CLI sent"

// zipPayload is the bundle every case here hands doUpload. Named so the size
// assertion below derives its expected line from the same bytes the run used.
var zipPayload = []byte("PRETEND-ZIP-BYTES")

// TestSubmitDiagnosisPrintsWhenBytesReallyLeft is the POSITIVE CONTROL for the
// three suppression cases below, and it is not optional.
//
// 🔴 A ZERO IS INDISTINGUISHABLE FROM A HARNESS WIRED TO NOTHING. Each case
// below asserts that a string does NOT appear; all three would pass against a
// doUpload that prints nothing ever, against a buffer nobody writes to, and
// against a manifest that fails earlier for an unrelated reason. This proves the
// same harness DOES produce the block when the request genuinely went out —
// server hit count 1, block present — so the zeros mean something.
func TestSubmitDiagnosisPrintsWhenBytesReallyLeft(t *testing.T) {
	rec := newUploadRecorder(t, http.StatusInternalServerError, `{"error":"boom"}`)
	client := appapi.New(rec.URL, "tok", "/api/blocks/submit-version")

	stderr, err := doUploadStderr(t, client)
	if err == nil {
		t.Fatal("a 500 from the submit endpoint must fail the upload")
	}
	if got := rec.hits.Load(); got != 1 {
		t.Fatalf("CONTROL failure: the recorder saw %d request(s), want exactly 1 — this test is "+
			"not exercising a real upload, so the suppression cases it controls prove nothing", got)
	}
	if !strings.Contains(stderr, pastTenseBlock) {
		t.Errorf("a 500 AFTER the request went out must still print %q — that is the #423 case the "+
			"block exists for, and suppressing it here would fix #637 by deleting the feature.\n"+
			"stderr:\n%s", pastTenseBlock, stderr)
	}

	// 🔴 AND THE COUNT IS PRESENTED AS AN UPPER BOUND, WHICH IS A SEPARATE CLAIM
	// AND WAS UNPINNED UNTIL A MUTANT SAID SO. app_submit_size_test.go asserts the
	// line CONTAINS "<n> bytes on the wire", which is satisfied with or without
	// the qualifier — so deleting it survived the whole battery. The qualifier is
	// load-bearing since #637 widened when this block prints: it now prints for a
	// transfer cut short, where the body size is what the CLI BUILT and more than
	// what arrived (measured: 8,388,627 printed against 233,266 received). The
	// whole line is constructed from the derived size rather than retyped, so a
	// reword fails and a change of arithmetic moves with it.
	wantLine := fmt.Sprintf("up to %d bytes on the wire — a %d-byte zip, base64-encoded into a JSON body.",
		appapi.SubmitBodySize(len(zipPayload), appapi.Provenance{}), len(zipPayload))
	if !strings.Contains(stderr, wantLine) {
		t.Errorf("the size line is not the upper-bound form.\nwant verbatim:\n  %s\nstderr:\n%s",
			wantLine, stderr)
	}
}

// TestSubmitDiagnosisAbsentWhenTokenLookupFails is issue #637's measured path.
func TestSubmitDiagnosisAbsentWhenTokenLookupFails(t *testing.T) {
	rec := newUploadRecorder(t, http.StatusOK, "{}")
	// UNTAGGED, exactly as internal/auth/source.go:115 returns it.
	client := appapi.NewWithSource(rec.URL, failingTokenSource{
		err: fmt.Errorf("persist refreshed tokens: open /config/civitai/config.yaml: read-only file system"),
	}, "/api/blocks/submit-version")

	stderr, err := doUploadStderr(t, client)
	if err == nil {
		t.Fatal("a token-source failure must fail the upload")
	}
	if got := rec.hits.Load(); got != 0 {
		t.Fatalf("CONTROL failure: the recorder saw %d request(s), want 0 — this case is supposed to "+
			"fail BEFORE a request is built, so it is not measuring what it claims", got)
	}
	if strings.Contains(stderr, pastTenseBlock) {
		t.Errorf("issue #637: %q printed for a failure raised before any request was built.\n"+
			"The server received 0 bytes and the CLI told the author it sent some. app_submit.go's "+
			"own comment says THE TENSE IS THE WHOLE POINT, and README.md now promises `sent` names "+
			"bytes that really went out.\nstderr:\n%s", pastTenseBlock, stderr)
	}
}

// TestSubmitDiagnosisAbsentWhenTheCredentialIsLocallyRefused is #637's MIRROR
// case, from the round 3 audit of #635: internal/auth/source.go:90 tags
// civitai.ErrUnauthorized onto a purely LOCAL error ("no refresh token stored"),
// so the old suppression arm hid the block for the right output and the wrong
// reason — a local fact classified in wire vocabulary.
//
// The observable does not change here; the ATTRIBUTION does. This test exists so
// that a future change to the 401 arm cannot silently start printing the block
// for a case where nothing was sent.
func TestSubmitDiagnosisAbsentWhenTheCredentialIsLocallyRefused(t *testing.T) {
	rec := newUploadRecorder(t, http.StatusOK, "{}")
	client := appapi.NewWithSource(rec.URL, failingTokenSource{
		err: civitai.Tag(civitai.ErrUnauthorized, errors.New("no refresh token stored — run `civitai login` again")),
	}, "/api/blocks/submit-version")

	stderr, err := doUploadStderr(t, client)
	if err == nil {
		t.Fatal("a locally-refused credential must fail the upload")
	}
	if got := rec.hits.Load(); got != 0 {
		t.Fatalf("CONTROL failure: the recorder saw %d request(s), want 0", got)
	}
	if strings.Contains(stderr, pastTenseBlock) {
		t.Errorf("%q printed for a credential refused locally, with no server contacted.\nstderr:\n%s",
			pastTenseBlock, stderr)
	}
	if !errors.Is(err, civitai.ErrUnauthorized) {
		t.Errorf("the fix must not strip the ErrUnauthorized classification — exit code 3 is "+
			"published contract (claudedocs/decisions/07). got: %v", err)
	}
}

// TestSubmitDiagnosisAbsentWhenTheDialFails is the case a pre-contact-only fix
// does NOT cover, and it is why the signal is "were the request bytes written"
// rather than "was a request built".
//
// The request IS built and IS handed to the transport; the connection is never
// established, so nothing goes on the wire. Under a build-time gate the block
// would print `N bytes on the wire` for zero bytes — the same lie in a different
// place.
func TestSubmitDiagnosisAbsentWhenTheDialFails(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := dead.URL
	dead.Close() // nothing listens on that port now

	client := appapi.New(url, "tok", "/api/blocks/submit-version")
	stderr, err := doUploadStderr(t, client)
	if err == nil {
		t.Fatal("a dial failure must fail the upload")
	}
	if strings.Contains(stderr, pastTenseBlock) {
		t.Errorf("%q printed for a connection that was never established, so the byte count it "+
			"names is zero.\nerr: %v\nstderr:\n%s", pastTenseBlock, err, stderr)
	}
}
