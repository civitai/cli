package devtunnel

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// TestGenerateEphemeralKeyIsValidEd25519 asserts the minted key is a real,
// parseable ed25519 OpenSSH public key and that the signer's public half matches
// the authorized-key line (so the private key the tunnel presents corresponds to
// the pubkey the server fingerprints).
func TestGenerateEphemeralKeyIsValidEd25519(t *testing.T) {
	key, err := GenerateEphemeralKey()
	if err != nil {
		t.Fatalf("GenerateEphemeralKey: %v", err)
	}
	if key.Signer == nil {
		t.Fatal("nil signer")
	}

	// The authorized-key line must be a single, comment-free ssh-ed25519 line.
	if !strings.HasPrefix(key.AuthorizedKey, "ssh-ed25519 ") {
		t.Errorf("authorized key should be ssh-ed25519, got %q", key.AuthorizedKey)
	}
	if strings.ContainsAny(key.AuthorizedKey, "\n\r") {
		t.Errorf("authorized key must be a single line (no newline): %q", key.AuthorizedKey)
	}
	fields := strings.Fields(key.AuthorizedKey)
	if len(fields) != 2 {
		t.Errorf("authorized key should be exactly `<type> <base64>` (no comment), got %d fields: %q", len(fields), key.AuthorizedKey)
	}

	// It must parse as an authorized key.
	parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key.AuthorizedKey))
	if err != nil {
		t.Fatalf("authorized key does not parse: %v", err)
	}
	if parsed.Type() != ssh.KeyAlgoED25519 {
		t.Errorf("key type = %q, want %q", parsed.Type(), ssh.KeyAlgoED25519)
	}

	// The signer's public key must equal the marshaled authorized key (same pair).
	if got, want := string(ssh.MarshalAuthorizedKey(key.Signer.PublicKey())), key.AuthorizedKey+"\n"; got != want {
		t.Errorf("signer public key != authorized key\n got: %q\nwant: %q", got, want)
	}
}

// TestGenerateEphemeralKeyIsUnique asserts each call mints a fresh key (in
// memory, per session — not a reused on-disk identity).
func TestGenerateEphemeralKeyIsUnique(t *testing.T) {
	a, err := GenerateEphemeralKey()
	if err != nil {
		t.Fatal(err)
	}
	b, err := GenerateEphemeralKey()
	if err != nil {
		t.Fatal(err)
	}
	if a.AuthorizedKey == b.AuthorizedKey {
		t.Error("two GenerateEphemeralKey calls produced the same key — must be ephemeral/unique")
	}
}

// TestNewRealTimerFires is a light sanity check on the production Timer seam.
func TestNewRealTimerFires(t *testing.T) {
	tm := NewRealTimer(time.Millisecond)
	select {
	case <-tm.C():
	case <-time.After(time.Second):
		t.Fatal("real timer did not fire")
	}
	tm.Stop()
}
