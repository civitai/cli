package cmd

import (
	"strings"
	"testing"
)

// help_text_accuracy_test.go pins help prose that community reports showed
// claiming something the code does not do. Each case asserts the WHOLE corrected
// sentence as it appears in `--help` output (a reword fails here on purpose) and
// that the false phrasing is gone.

func helpOut(t *testing.T, args ...string) string {
	t.Helper()
	out, _, err := run(t, append(args, "--help")...)
	if err != nil {
		t.Fatalf("%v --help: %v", args, err)
	}
	return out
}

func assertHelp(t *testing.T, out string, want, gone []string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("help is missing %q; got:\n%s", w, out)
		}
	}
	for _, g := range gone {
		if strings.Contains(out, g) {
			t.Errorf("help still says %q; got:\n%s", g, out)
		}
	}
}

// #884: --model takes ModelVersions[0], the first version the API lists (the
// creator's version order), not the earliest published one.
func TestDownloadHelpDescribesModelDefaultAsFirstListed(t *testing.T) {
	assertHelp(t, helpOut(t, "download"),
		[]string{
			"…or resolve a model's default version with --model. The default is the FIRST\n" +
				"version the API lists for the model — the top of the creator's version order,\n" +
				"usually but not always the newest, and NOT the first published:",
			"resolve+download a MODEL's default version (the first the API lists: the creator's top version, not the first published) instead of a version id",
		},
		[]string{"default (first published) version"})
}

// #881: `images search --nsfw` selects the server's NSFW-only browsing levels;
// it does not widen a mixed feed.
func TestImagesSearchHelpSaysNSFWFilters(t *testing.T) {
	assertHelp(t, helpOut(t, "images", "search"),
		[]string{"FILTER to NSFW (mature-rated) images only — it does not add them to the default mix; --nsfw=false filters to safe-for-work only"},
		[]string{"include NSFW results"})
}

// #879: the config file lives under os.UserConfigDir(), which differs per OS.
func TestLoginHelpNamesThePerOSConfigPath(t *testing.T) {
	assertHelp(t, helpOut(t, "login"),
		[]string{"to config.yaml in your per-OS config directory, owner-readable only:\n" +
			"~/.config/civitai/ on Linux ($XDG_CONFIG_HOME/civitai/ when set),\n" +
			"~/Library/Application Support/civitai/ on macOS, %AppData%\\civitai\\ on Windows."},
		[]string{"(~/.config/civitai/config.yaml, owner-readable only)"})
}

// #874: local authoring runs without a token; only platform calls need one.
func TestRootHelpScopesTheAppTokenRule(t *testing.T) {
	assertHelp(t, helpOut(t),
		[]string{"Local authoring (app create,\n" +
			"    app validate) runs without a token; every app command that talks to the\n" +
			"    platform needs one, including the App-store browse commands app list /\n" +
			"    app view:"},
		[]string{"EVERY civitai app command needs a"})
}

// #873: the by-hash example must resolve anonymously. 879DB523C3 is the AutoV2
// hash of model 4384 version 128713's primary file; 5D8D26E2A6 404s anonymously.
func TestModelVersionsHelpUsesAResolvableHash(t *testing.T) {
	for _, args := range [][]string{{"model-versions"}, {"model-versions", "by-hash"}} {
		assertHelp(t, helpOut(t, args...),
			[]string{"civitai model-versions by-hash 879DB523C3"},
			[]string{"5D8D26E2A6"})
	}
}
