package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/civitai/cli/internal/config"
)

// pendingGeneration is the crash-recovery record for a submit.
//
// 🔴 It is written BEFORE the POST, not after. The money moves SERVER-SIDE the
// moment the orchestrator accepts the workflow, so every window in which the
// process can die — the POST's own round trip, a slow reply, Ctrl-C two seconds
// in, an OOM — is a window in which the user has been charged and, without this
// file, holds no handle to what they bought. Writing it afterwards would record
// exactly the cases that did not need recording.
//
// The externalId is the durable handle: the orchestrator dedupes on
// (userId, externalId) and answers a duplicate with HTTP 200 and the PRE-EXISTING
// workflow — there is no 409 — so re-submitting with the same key re-attaches
// instead of charging again. That is why it is minted before the request rather
// than read out of the reply.
type pendingGeneration struct {
	// ExternalID is the orchestrator idempotency key (see above).
	ExternalID string `json:"externalId"`
	// SubmittedAt is when the CLI was about to POST, in RFC3339.
	SubmittedAt string `json:"submittedAt"`
	// PayloadHash is the SHA-256 of the marshalled GRAPH.
	//
	// 🔴 It is RECORDED ONLY — nothing reads it back and compares it. The single
	// reader of a written record is recordPendingWorkflowID, which loads the file
	// solely to add the workflow id and rewrites this field unchanged. So the CLI
	// does NOT today detect "a different job that reused the key", and this
	// comment must not claim it does: the comparison needs a recover/re-attach
	// flow that does not exist yet.
	//
	// It is written now because the value is only obtainable at submit time —
	// once the process is gone the graph is gone with it — so recording it is
	// what keeps that future comparison POSSIBLE. If you add the recover flow,
	// the comparison is the point: the orchestrator silently returns the FIRST
	// workflow for a reused key, so a hash mismatch would be the only local
	// signal that the key no longer describes what the user is asking for.
	PayloadHash string `json:"payloadHash"`
	// BaseURL records which server was charged.
	BaseURL string `json:"baseUrl,omitempty"`
	// WorkflowID is filled in once the submit replies. Empty means the reply
	// never arrived — the run may still have been charged.
	WorkflowID string `json:"workflowId,omitempty"`
}

// pendingDirName is the subdirectory of the CLI config dir holding these
// records.
const pendingDirName = "pending"

// pendingDir returns the directory for pending-generation records, creating it.
func pendingDir() (string, error) {
	base, err := config.Dir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, pendingDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	return dir, nil
}

// graphPayloadHash returns the SHA-256 of v's JSON encoding, lowercase hex.
func graphPayloadHash(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		// A graph that will not marshal cannot be submitted either; record the
		// failure rather than a hash of nothing.
		return "unhashable:" + err.Error()
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// pendingPath is the record's on-disk path. The externalId is CLI-minted (a v4
// UUID), but it can also be user-supplied via --external-id, so it is reduced to
// a bare basename before joining — a value like "../../config.yaml" must not be
// able to steer the write.
func pendingPath(dir, externalID string) (string, error) {
	base := filepath.Base(strings.TrimSpace(externalID))
	if base == "" || base == "." || base == ".." || base == string(filepath.Separator) {
		return "", fmt.Errorf("unusable external id %q", externalID)
	}
	return filepath.Join(dir, base+".json"), nil
}

// writePendingGeneration writes the record atomically (temp file + rename, 0600)
// and returns its path. It takes the directory as an argument so tests write
// into a t.TempDir() rather than the user's real config dir.
func writePendingGeneration(dir string, p pendingGeneration) (string, error) {
	path, err := pendingPath(dir, p.ExternalID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	body, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(body, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("install %s: %w", path, err)
	}
	return path, nil
}

// readPendingGeneration loads a record written by writePendingGeneration.
func readPendingGeneration(path string) (*pendingGeneration, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // path is CLI-constructed
	if err != nil {
		return nil, err
	}
	var p pendingGeneration
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("unreadable pending-generation record %s: %w", path, err)
	}
	return &p, nil
}

// recordPendingWorkflowID re-writes the record with the workflow id the submit
// returned, so the recovery hint can name a workflow and not just a key.
// Failures are returned but are never fatal to the caller: the generation has
// already been paid for and printing its ids matters more than the bookkeeping.
func recordPendingWorkflowID(path, workflowID string) error {
	p, err := readPendingGeneration(path)
	if err != nil {
		return err
	}
	p.WorkflowID = workflowID
	dir, _ := filepath.Split(path)
	_, werr := writePendingGeneration(strings.TrimSuffix(dir, string(filepath.Separator)), *p)
	return werr
}

// nowRFC3339 is the timestamp seam (a package var so a test can pin it).
var nowRFC3339 = func() string { return time.Now().UTC().Format(time.RFC3339) }
