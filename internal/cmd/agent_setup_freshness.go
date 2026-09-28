package cmd

import "fmt"

// The `cli-version` row's FRESHNESS half, and the one network call `--check`
// makes.
//
// 🔴 THE ROW USED TO BE A PRESENCE CHECK WEARING A FRESHNESS CHECK'S NAME. It
// emitted `{"name":"cli-version","ok":true,"detail":"<version>"}` — the running
// version, echoed back, with nothing compared against anything. Measured in a
// real operator's session (ses_f1b9a7d40ffeqlEuYZlMEN0U2L): the row read
// `ok: true, detail "0.1.105"` while `0.1.109` was published, and
// `developer.civitai.com`'s hosted setup prompt tells its reader `"ok" is the
// verdict — read that, not the individual rows`. So a four-versions-stale CLI was
// reported as a green setup by the only surface that could have said otherwise.
// A row whose value is a fact about itself cannot be wrong, and cannot be useful.
//
// 🔴 IT TOUCHES THE NETWORK, AND THAT IS A DELIBERATE REVERSAL OF A DOCUMENTED
// CONTRACT. `--help` and README both said `--check` "contacts nothing"; both now
// say what it does contact. The three properties that make it affordable are
// asserted, not intended:
//
//   - FAIL SOFT. An unreachable endpoint, a timeout, a non-200, an unparseable
//     tag and an opted-out run ALL produce `ok: true` with a detail that says the
//     comparison did not happen. Only a SUCCESSFUL comparison showing the running
//     build to be older may return `ok: false`. That is the half this repo has
//     been burned on twice — `pins-vs-published` froze every open PR, and npm 10's
//     arborist crash did it again — and both times the defect was a gate that went
//     red because a network went away.
//   - BOUNDED. It reuses `updateCheckTimeout` (the same 2.5s bound
//     `civitai version` runs under), so the worst case is one short round-trip.
//   - OPTED OUT BY THE EXISTING CONTROL. `--no-update-check` /
//     CIVITAI_NO_UPDATE_CHECK already govern exactly this endpoint for `version`
//     and `upgrade`, and `updateCheckDisabled` is the one predicate all three
//     read.
//
// 🔴 NOT A NEW `CIVITAI_CHECK_*` VARIABLE, AND THE DISTINCTION IS LOAD-BEARING.
// `CIVITAI_CHECK_PUBLISHED_PINS`, `CIVITAI_CHECK_DOCS_LINKS` and
// `CIVITAI_CHECK_SCAFFOLD_RUNTIME` are opt-IN gates on TESTS — they appear only
// in `_test.go` files and in CI workflow env blocks, and they exist so `make ci`
// stays green with no network. Copying that shape into a user-facing command
// would give the feature a default of OFF, i.e. a freshness check that never runs
// for the operator it was written for. The runtime precedent is the opt-OUT this
// endpoint already has.

// 🔴 IT ADDS NO NEW SEAM. `latestReleaseURL` is already a package var precisely so
// a test can point it at an httptest server, and `pointAtServer` in
// update_check_test.go is the existing helper for doing it — so this path is
// substitutable through machinery three other commands already share. An earlier
// draft introduced a second, function-shaped seam (`agentSetupLatestRelease`) and
// stubbed it in TestMain; that made every new test in this change UNABLE TO COMPILE
// at the merge-base, which would have destroyed the red-at-base evidence for the
// feature. One seam, already there, is both smaller and testable in both
// directions. TestMain points `latestReleaseURL` at a dead loopback address so the
// package stays hermetic by default, and
// TestAgentSetupCheckReadsTheRealReleaseEndpoint proves the production wiring with
// a live local server.

// versionFreshness is the resolved comparison input for the `cli-version` row.
//
// 🔴 TWO FIELDS, NOT A (string, error) PAIR, BECAUSE "OPTED OUT" IS NOT AN ERROR.
// A run with CIVITAI_NO_UPDATE_CHECK set did not fail to check; it was told not
// to. Collapsing the two into an error makes the row's detail describe a network
// problem the user does not have.
type versionFreshness struct {
	// Latest is the published release tag, or "" when no comparison is possible.
	Latest string
	// Reason says why Latest is empty, in words that finish the sentence
	// "could not check whether this is current: …". It is "" when Latest is set,
	// and non-"" whenever Latest is not — TestVersionFreshnessAlwaysCarriesOneOrTheOther
	// pins that exclusivity, because a row built from an empty-empty value would
	// read "could not check whether this is current: ".
	Reason string
}

// resolveVersionFreshness performs the bounded, fail-soft lookup. It returns a
// value for EVERY input, including an opted-out run, and never an error.
func resolveVersionFreshness(disabled bool) versionFreshness {
	if disabled {
		return versionFreshness{Reason: "the update check is disabled (--no-update-check / CIVITAI_NO_UPDATE_CHECK)"}
	}
	ctx, cancel := contextWithUpdateTimeout()
	defer cancel()

	latest, err := fetchLatestRelease(ctx, latestReleaseURL)
	switch {
	case err != nil:
		// 🔴 THE ERROR IS QUOTED, NOT CLASSIFIED. A timeout, a DNS failure, a
		// corporate proxy's 403 and a GitHub rate-limit all land here, and the
		// detail is read by a human deciding whether to care. Bucketing them into
		// "offline" would be a guess this function cannot make.
		return versionFreshness{Reason: "the release lookup failed: " + err.Error()}
	case latest == "":
		return versionFreshness{Reason: "the release lookup returned no tag"}
	case !isParseableVersion(latest):
		// A tag this CLI cannot parse cannot be compared, and claiming the running
		// build is current on that basis is the unfounded "up to date" the update
		// notice already refuses to print.
		return versionFreshness{Reason: fmt.Sprintf("the published tag %q is not a version this CLI can compare", latest)}
	}
	return versionFreshness{Latest: latest}
}

// appTrackTokenNote is the sentence that would have saved 110 minutes.
//
// 🔴 IT NAMES WHAT CIVITAI_TOKEN IS *NOT* FOR, WHICH IS THE QUESTION THE OPERATOR
// ACTUALLY HAD. Every surface told them how to export it and none told them
// whether they had to. In the measured session the agent scaffolded, built,
// tested, validated and SUBMITTED an App with CIVITAI_TOKEN never set: it appears
// in 24 stretches of that session's discussion and in 0 of its 50 shell commands.
// `whoami`, `app dev-token` and `app submit` all read this CLI's own stored
// credential through `config.Load`, and `npm run dev:live` reads the
// `VITE_LIVE_BLOCK_TOKEN` minted from it — none of them consults the process
// environment for a bearer. The variable exists for the MCP servers, which that
// session never called once.
//
// 🔴 IT IS ONE CONSTANT SO THE TWO SURFACES CANNOT DISAGREE. It is printed in the
// `Next:` block's export step — where the reader is standing when the confusion
// starts — and carried in the `agent-token` row's detail, which is what a
// `--check --json` consumer and the hosted prompt read.
const appTrackTokenNote = "You do NOT need " + tokenEnvVar + " to build an App: scaffolding, `npm run dev`, " +
	"`npm run dev:live`, `civitai app validate` and `civitai app submit` all read this CLI's own stored " +
	"credential (or a dev token minted from it), never this variable — it is ONLY for the MCP servers your " +
	"agent calls."

// cliVersionCheck builds the `cli-version` row from the running version and a
// resolved freshness value.
//
// 🔴 EXACTLY ONE ARM RETURNS `ok: false`: both versions parsed AND the running one
// is older. Every other outcome — could-not-check, equal, ahead, or a running
// version this CLI cannot parse (a `dev` build, a Go pseudo-version) — is
// `ok: true` with a detail that says which. TestCLIVersionRowIsFalseOnlyWhenStale
// enumerates the arms so a later edit cannot widen that set by accident.
func cliVersionCheck(current string, f versionFreshness) agentCheckJSON {
	if f.Latest == "" {
		return agentCheckJSON{Name: checkCLIVersion, OK: true,
			Detail: current + " — could not check whether it is current: " + f.Reason +
				". This row never fails on that; re-run with a network to compare"}
	}
	if !isParseableVersion(current) {
		// A dev build or a pseudo-version. It is not stale and it is not current;
		// saying which release it was compared against is the honest answer.
		return agentCheckJSON{Name: checkCLIVersion, OK: true,
			Detail: current + " — not a release version this CLI can compare against the published " +
				f.Latest + ", so freshness is unknown"}
	}
	switch compareVersions(current, f.Latest) {
	case -1:
		return agentCheckJSON{Name: checkCLIVersion, OK: false,
			Detail: current + " — latest is " + f.Latest + "; run `civitai upgrade`. A stale CLI ships " +
				"stale instructions: the AGENTS.md block, the scaffold pins and the docs URLs it writes are " +
				"whatever this build carries"}
	case 0:
		return agentCheckJSON{Name: checkCLIVersion, OK: true, Detail: current + " — the latest published release"}
	default:
		return agentCheckJSON{Name: checkCLIVersion, OK: true,
			Detail: current + " — ahead of the latest published release (" + f.Latest + ")"}
	}
}

// agentTokenCheck builds the `agent-token` row: whether the MCP configuration
// this command writes can actually resolve a credential.
//
// 🔴 IT EXISTS BECAUSE ONE ROW CANNOT CARRY TWO VERDICTS. `authenticated` used to
// report BOTH stores, and on the measured path it emitted `ok: true` beside a
// detail ending "https://orchestration.civitai.com/mcp 401s until you export
// CIVITAI_TOKEN" — a row asserting success over a state it describes as broken,
// which no consumer can act on and the hosted prompt reads as green. Splitting
// them lets each say one true thing: `authenticated` is about THIS CLI's store
// (which really was fine), and this row is about the agent's.
//
// 🔴 IT IS `ok: false` ONLY WHERE A REFERENCE WAS ACTUALLY WRITTEN. For Zed — and
// for `--agent other` — this command writes no environment reference at all, so
// an unset variable breaks nothing it produced and demanding an export would be a
// permanently-red row for a correct setup. Presence of the reference is the
// condition, and it is read from `agentTargets`, not from a list of agent names.
//
// 🔴 AND IT IS EXCLUDED FROM THE VERDICT, for the reason `authenticated` is:
// `agent-setup` stops before auth on purpose, so a setup that did everything this
// CLI can do is a success even with the export outstanding. The row stays, stays
// `false`, and keeps its detail — see checkCountsTowardVerdict.
func agentTokenCheck(env agentEnv, agent string) agentCheckJSON {
	t, known := agentTargets[agent]
	if !known || (t.EnvHeaderSyntax == "" && t.EnvBearerKey == "") {
		return agentCheckJSON{Name: checkAgentToken, OK: true,
			Detail: "no " + tokenEnvVar + " reference is written for " + agent + " (it documents no way to read " +
				"one from the environment), so nothing here resolves that variable. " + appTrackTokenNote}
	}
	if tokenIsExported(env) {
		return agentCheckJSON{Name: checkAgentToken, OK: true,
			Detail: tokenEnvVar + " is set in this environment, which is where " + agent + " resolves the " +
				"reference in its MCP config from"}
	}
	return agentCheckJSON{Name: checkAgentToken, OK: false,
		Detail: tokenEnvVar + " is NOT set in this environment, and " + agent + "'s MCP entries reference it by " +
			"name — so " + agent + " resolves it to an empty string and https://orchestration.civitai.com/mcp " +
			"401s. That is the only thing it affects. " + appTrackTokenNote}
}
