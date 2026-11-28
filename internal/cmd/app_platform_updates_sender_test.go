package cmd

import (
	"bytes"
	"testing"

	"github.com/civitai/cli/internal/appapi"
)

// The sender line of a platform update, whole. Since civitai/civitai#10717 the
// owner's record names a verified staff update "CivitaiOfficial (App
// Assistant)" and names nobody (null) otherwise; the line prints that name as
// sent — no " (verified Civitai staff)" suffix after it — a null sender reads
// "An unnamed sender", and only verifyCommit's answer adds the could-not-verify
// note.
func TestPlatformUpdateSummarySenderLine(t *testing.T) {
	const (
		id     = "apu_01JSENDERLINE00000000001"
		sha    = "5b0e1c7d9a3f4e2b8c6d1a0f9e8d7c6b5a4f3e2d"
		reason = "Move to the v2 storage hooks"
	)
	official := "CivitaiOfficial (App Assistant)"
	blank := "   "
	couldNot := "  Could not verify that this update came from Civitai staff (the server did not confirm commit " + sha +
		"); review it with `git show " + sha + "`.\n"

	cases := []struct {
		name                    string
		username                *string
		verified, verifiedKnown bool
		want                    string
	}{
		{"verified, official sender", &official, true, true,
			"  CivitaiOfficial (App Assistant) updated your app: " + reason + "\n"},
		{"verified, null sender", nil, true, true,
			"  An unnamed sender updated your app: " + reason + "\n"},
		{"verified, blank sender", &blank, true, true,
			"  An unnamed sender updated your app: " + reason + "\n"},
		{"unverified, null sender", nil, false, true,
			"  An unnamed sender updated your app: " + reason + "\n" + couldNot},
		{"unverified, official sender", &official, false, true,
			"  CivitaiOfficial (App Assistant) updated your app: " + reason + "\n" + couldNot},
		{"not asked, null sender", nil, false, false,
			"  Platform update " + id + " from an unnamed sender: " + reason + "\n"},
		{"not asked, official sender", &official, false, false,
			"  Platform update " + id + " from CivitaiOfficial (App Assistant): " + reason + "\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := appapi.PlatformUpdate{ID: id, CommitSha: sha, Reason: reason,
				By: appapi.PlatformUpdateBy{Username: tc.username, VerifiedStaff: tc.username != nil}}
			var out bytes.Buffer
			printPlatformUpdateSummary(&out, u, tc.verified, tc.verifiedKnown)
			if got := out.String(); got != tc.want {
				t.Fatalf("output:\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}
