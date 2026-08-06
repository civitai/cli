package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 🔴 pendingPath's containment guard.
//
// The externalId is normally a CLI-minted UUID, but --external-id lets a user
// supply it verbatim, and the record is written with os.WriteFile + os.Rename.
// So the value steers a WRITE PATH, and filepath.Base is the only thing standing
// between "--external-id ../../config.yaml" and clobbering the user's config.
//
// Surviving mutation: delete the filepath.Base call. The reject list below it
// ("", ".", "..", "/") does NOT catch a traversal — "../../evil" is none of
// those — so without Base, filepath.Join cleans the "../.." and the write lands
// outside the pending dir. Nothing in the suite noticed.
//
// Every case uses t.TempDir(): a unit test must never write into the developer's
// real ~/.config/civitai.

// TestPendingPath_CannotEscapeThePendingDir pins containment STRUCTURALLY — on
// the resolved parent directory of the returned path, not on the spelling of the
// input. A guard that rejected the literal string "../../evil" would pass a
// spelling test and still let "..%2F", "a/../../b" or a nested path through.
func TestPendingPath_CannotEscapeThePendingDir(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "pending")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	hostile := []struct {
		name       string
		externalID string
	}{
		{"parent traversal", "../../evil"},
		{"single parent", "../evil"},
		{"deep traversal", "../../../../../../etc/passwd"},
		{"absolute path", "/etc/passwd"},
		{"absolute into config", "/home/someone/.config/civitai/config.yaml"},
		{"nested relative", "a/../../b"},
		{"subdirectory", "sub/dir/evil"},
		{"trailing slash", "evil/"},
		{"traversal with surrounding space", "  ../../evil  "},
		{"windows-ish separators", `..\..\evil`},
	}

	for _, tc := range hostile {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pendingPath(dir, tc.externalID)
			if err != nil {
				// Refusing outright is an acceptable answer for a hostile value;
				// what is NOT acceptable is returning a path outside dir.
				return
			}
			// 🔴 THE assertion: whatever name survived, it lives directly in dir.
			if parentOf := filepath.Dir(got); parentOf != filepath.Clean(dir) {
				t.Fatalf("🔴 TRAVERSAL: --external-id %q produced %q, whose directory is %q, not the pending dir %q",
					tc.externalID, got, parentOf, dir)
			}
			// Belt and braces: the cleaned path must still be under dir.
			rel, rerr := filepath.Rel(dir, filepath.Clean(got))
			if rerr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				t.Fatalf("🔴 TRAVERSAL: %q escapes %q (rel=%q, err=%v)", got, dir, rel, rerr)
			}
		})
	}
}

// The values pendingPath refuses outright. Each of these reduces to a basename
// that names no file, so there is nothing safe to write.
func TestPendingPath_RejectsUnusableIDs(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"", ".", "..", string(filepath.Separator), "   ", "\t\n"} {
		got, err := pendingPath(dir, id)
		if err == nil {
			t.Errorf("external id %q was accepted and produced %q, want a refusal", id, got)
		}
	}

	// POSITIVE CONTROL: an ordinary id IS accepted by the same function, so the
	// refusals above are a property of these values and not of a function that
	// rejects everything.
	ok, err := pendingPath(dir, "3f2504e0-4f89-41d3-9a0c-0305e82c3301")
	if err != nil {
		t.Fatalf("POSITIVE CONTROL FAILED: a normal external id was refused: %v", err)
	}
	if filepath.Dir(ok) != filepath.Clean(dir) {
		t.Errorf("a normal id landed outside the pending dir: %q", ok)
	}
	if !strings.HasSuffix(ok, ".json") {
		t.Errorf("record path %q does not end in .json", ok)
	}
}

// The containment property, asserted on the FILESYSTEM rather than on a string.
// A path assertion can be defeated by a symlink or by a later change to how the
// record is written; this walks the tree afterwards and checks where bytes
// actually landed.
func TestWritePendingGeneration_HostileExternalIDWritesOnlyInsideThePendingDir(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "pending")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// A file the traversal would clobber if the guard were removed. Its contents
	// are the canary.
	victim := filepath.Join(parent, "evil.json")
	const canary = "ORIGINAL CONTENTS — MUST NOT BE OVERWRITTEN"
	if err := os.WriteFile(victim, []byte(canary), 0o600); err != nil {
		t.Fatalf("write victim: %v", err)
	}

	rec := pendingGeneration{
		ExternalID:  "../evil",
		SubmittedAt: "2026-08-05T00:00:00Z",
		PayloadHash: "deadbeef",
	}
	path, err := writePendingGeneration(dir, rec)
	if err != nil {
		// A refusal is fine; nothing was written, so the canary is safe.
		t.Logf("hostile id refused outright: %v", err)
	} else if filepath.Dir(path) != filepath.Clean(dir) {
		t.Fatalf("🔴 record was written to %q, outside the pending dir %q", path, dir)
	}

	// 🔴 The canary must be untouched.
	got, rerr := os.ReadFile(victim) //nolint:gosec // test-controlled path
	if rerr != nil {
		t.Fatalf("the victim file disappeared: %v", rerr)
	}
	if string(got) != canary {
		t.Fatalf("🔴 TRAVERSAL: --external-id %q overwrote %q outside the pending dir.\n got: %q\nwant: %q",
			rec.ExternalID, victim, got, canary)
	}

	// POSITIVE CONTROL: the writer DOES create files, and it creates them in dir.
	// Without this, an untouched canary could simply mean nothing was ever
	// written by anything.
	okPath, err := writePendingGeneration(dir, pendingGeneration{
		ExternalID:  "3f2504e0-4f89-41d3-9a0c-0305e82c3301",
		SubmittedAt: "2026-08-05T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("POSITIVE CONTROL FAILED: a normal record could not be written: %v", err)
	}
	if _, serr := os.Stat(okPath); serr != nil {
		t.Fatalf("POSITIVE CONTROL FAILED: no file at %q: %v", okPath, serr)
	}
	if filepath.Dir(okPath) != filepath.Clean(dir) {
		t.Fatalf("POSITIVE CONTROL FAILED: normal record landed at %q, outside %q", okPath, dir)
	}

	// And nothing new appeared beside the pending dir: the only entries in the
	// parent are the victim file and the pending dir itself.
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 2 {
		t.Errorf("the parent dir gained entries: %v — a write escaped the pending dir", names)
	}
}
