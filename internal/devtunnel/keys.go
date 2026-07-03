// Package devtunnel holds the transport-level pieces of `civitai app dev-tunnel`:
// the EPHEMERAL SSH keypair the CLI mints per session, the reverse-tunnel dialer
// (`ssh -R` to the sish endpoint) behind an interface, and a small clock/timer
// seam — so the command's lifecycle (mint → tunnel → teardown on signal / idle)
// is unit-testable without a live server or a real network.
//
// DARK/INERT: this drives the P1 server contract + the P3 public sish endpoint,
// neither of which is live yet. The dialer is real code, but there is no way to
// exercise it end-to-end until P3 exposes the SSH listener; the interface is what
// the tests cover.
package devtunnel

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

// EphemeralKey is a per-session SSH keypair generated IN MEMORY and never
// written to disk (not to the user's ~/.ssh, nowhere). The private half lives
// only for the tunnel's lifetime; the public half is sent to
// blocks.startDevTunnel, which keys the tunnel credential by its fingerprint.
type EphemeralKey struct {
	// Signer authenticates the `ssh -R` bind (the private key stays here, in
	// memory only).
	Signer ssh.Signer
	// AuthorizedKey is the normalized OpenSSH public-key line
	// (`ssh-ed25519 AAAA...`, no comment) the CLI POSTs as `sshPublicKey`. It
	// matches the server's normalizeSshPublicKey form exactly.
	AuthorizedKey string
}

// GenerateEphemeralKey mints a fresh ed25519 SSH keypair in memory. ed25519 is
// small, fast, and universally supported by modern OpenSSH/sish. The returned
// AuthorizedKey is the trimmed single-line authorized-keys form (no trailing
// comment/newline), which is exactly what the server fingerprints.
func GenerateEphemeralKey() (*EphemeralKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, fmt.Errorf("build ssh signer: %w", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("build ssh public key: %w", err)
	}
	// MarshalAuthorizedKey emits `ssh-ed25519 AAAA...\n` (no comment); trim the
	// newline so the wire value is a clean single line.
	authorized := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
	return &EphemeralKey{Signer: signer, AuthorizedKey: authorized}, nil
}
