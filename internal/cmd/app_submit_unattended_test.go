package cmd

import "testing"

// TestSubmitIsUnattendedReadsEachSignalAlone pins that each of the three
// signals ALONE makes a submit unattended, and that with none of them it is
// attended. One case per signal, so dropping any one term is a red row.
func TestSubmitIsUnattendedReadsEachSignalAlone(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		tty  bool
		want bool
	}{
		{"none: a person at a terminal", nil, true, false},
		{"OIDC token only", map[string]string{GitHubOIDCTokenEnv: "eyJ.fake.jwt"}, true, true},
		{"GITHUB_ACTIONS only", map[string]string{"GITHUB_ACTIONS": "true"}, true, true},
		{"generic CI only", map[string]string{"CI": "1"}, true, true},
		{"no TTY only", nil, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearUnattendedSignals(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			withStdinTTY(t, tc.tty)
			if got := submitIsUnattended(); got != tc.want {
				t.Errorf("submitIsUnattended() = %v, want %v", got, tc.want)
			}
		})
	}
}
