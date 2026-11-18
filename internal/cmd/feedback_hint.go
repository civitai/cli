package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/pkg/civitai"
)

// Failure-point hints for `civitai feedback send`.
//
// At a SMALL, WHITELISTED set of places where the Civitai platform itself — not
// the developer's invocation — got in the way, the CLI prints one extra line:
// a ready-to-run `civitai feedback send` with --about and --ref prefilled. That
// is how an agent that never read the docs learns the channel exists at the
// moment it has something to report.
//
// 🔴 THE WHITELIST IS THE DESIGN. A hint on every error would teach readers to
// skip it, and a hint on a USER error (a typo'd flag, a missing token, a
// version that is not above the approved one) would send Civitai reports about
// the reporter. Each site below names the condition that makes it platform
// friction; feedbackHintSites is the ledger, and
// TestEveryFeedbackHintSiteIsLedgered pins it as a set, so adding a site is a
// decision someone makes in a test diff.
//
// 🔴 A HINT NEVER CHANGES AN EXIT CODE OR STDOUT. The error-path hints are
// written to stderr beside the error they annotate, after the error has been
// decided; nothing here returns an error or wraps one. The agent-setup footer is
// the one hint on stdout, because it is part of that command's human "Next:"
// block and is never printed under --json or --check.
//
// Decision record: claudedocs/decisions/41-platform-feedback.md §7.

// feedbackHintSite is one whitelisted failure point. about and ref become the
// flags of the suggested command; message is the quoted message argument, a
// placeholder the reader replaces.
type feedbackHintSite struct {
	about   string
	ref     string
	message string
}

// The sites. Each `ref` is a site key in the shape `civitai feedback send --ref`
// accepts (appapi.ValidPlatformFeedbackRef); each `about` is a member of
// appapi.PlatformFeedbackAbouts. Both are checked by test, and each suggested
// command is RUN under --dry-run by test, so a hint cannot name a flag or a
// value the binary refuses.
var (
	// The server answered `app submit` with a refusal that is not about the
	// caller's credential or rate: a 400 for a bundle the local validate
	// passed, a 413 for one under the vendored size ceiling, or a 5xx. See
	// submitRefusalIsPlatformFriction.
	feedbackHintSubmitRefused = feedbackHintSite{
		about: "review", ref: "submit.server-refused",
		message: "app submit was refused by the server: <the error above>",
	}
	// `app doctor`'s rollout probe got an answer this CLI cannot read a verdict
	// out of (appapi.RolloutUnknown) — the host answered, and not in the shape
	// the contract describes.
	feedbackHintDoctorRollout = feedbackHintSite{
		about: "cli", ref: "doctor.rollout-unreadable",
		message: "app doctor could not read the App Blocks rollout answer: <the rollout lines above>",
	}
	// `app dev-tunnel` minted a session and then could not open the reverse
	// tunnel to the platform's endpoint.
	feedbackHintTunnelDial = feedbackHintSite{
		about: "cli", ref: "tunnel.dial-failed",
		message: "app dev-tunnel could not open the reverse tunnel: <the error above>",
	}
	// A 2xx response body this CLI could not decode (civitai.ErrUnreadableResponse).
	feedbackHintDecode = feedbackHintSite{
		about: "cli", ref: "decode.unreadable-response",
		message: "<the command you ran> got a response the CLI could not read: <the error above>",
	}
	// The agent-setup footer: not a failure, the one place a new project's agent
	// is told the channel exists. No --ref: nothing failed at a site.
	feedbackHintAgentSetup = feedbackHintSite{
		about:   "cli",
		message: "<what you ran, what you expected, what happened>",
	}
)

// feedbackHintSites is the ledger of every site. TestEveryFeedbackHintSiteIsLedgered
// pins it; the hint tests walk it.
var feedbackHintSites = []feedbackHintSite{
	feedbackHintSubmitRefused,
	feedbackHintDoctorRollout,
	feedbackHintTunnelDial,
	feedbackHintDecode,
	feedbackHintAgentSetup,
}

// feedbackHintPrefix opens every hint line, so a reader (or a grep) can tell it
// from the error it follows.
const feedbackHintPrefix = "Feedback: "

// command is the suggested invocation, exactly as printed.
func (s feedbackHintSite) command() string {
	parts := []string{"civitai feedback send --about", s.about}
	if s.ref != "" {
		parts = append(parts, "--ref", s.ref)
	}
	parts = append(parts, `"`+s.message+`"`)
	return strings.Join(parts, " ")
}

// line is the whole hint line, without a trailing newline.
func (s feedbackHintSite) line() string {
	return feedbackHintPrefix + "if this is the Civitai platform getting in your way, report it to Civitai: " + s.command()
}

// printFeedbackHint writes one site's hint line.
func printFeedbackHint(w io.Writer, s feedbackHintSite) {
	fmt.Fprintln(w, s.line())
}

// agentSetupFeedbackLine is the footer agent-setup prints under its "Next:"
// block. It names the --about areas, because the footer is not tied to one.
func agentSetupFeedbackLine() string {
	return feedbackHintPrefix + "when the Civitai platform itself gets in your way, report it with " +
		feedbackHintAgentSetup.command() + " (--about: " + strings.Join(appapi.PlatformFeedbackAbouts, ", ") +
		"); check on it later with `civitai feedback status`. Not `civitai app feedback` — that is your app's user inbox."
}

// FeedbackHint returns the hint line for an error that reached the entrypoint,
// or "" when the error is not one of the whitelisted platform failures. It is
// the hook cmd/civitai's main uses for failures that can arise in any command —
// today only an unreadable response — so the site is not open-coded per command.
func FeedbackHint(err error) string {
	if errors.Is(err, civitai.ErrUnreadableResponse) {
		return feedbackHintDecode.line()
	}
	return ""
}

// submitRefusalIsPlatformFriction reports whether a failed `app submit` upload
// is one the platform caused, and so earns the hint.
//
// It is true only when the SERVER ANSWERED with one of:
//   - 400 — the local validate passed and the server refused anyway: the
//     local mirror and the server disagree (decision 01), or the body was
//     refused for a reason the response does not name (#423's `Invalid JSON`);
//   - 413 — the body was under the vendored ceiling and the server refused its
//     size (decision 31: the ceiling is not a server mirror);
//   - any 5xx.
//
// Everything else is the caller's to fix or not a refusal at all: 401/403
// (credential, Apps access), 404, 409 (a version that already exists), 429,
// our own preflight size refusal, and any failure where nothing was sent.
func submitRefusalIsPlatformFriction(err error) bool {
	var refused *appapi.SubmitRefusedError
	if !errors.As(err, &refused) {
		return false
	}
	s := refused.Status
	return s == http.StatusBadRequest || s == http.StatusRequestEntityTooLarge || s >= http.StatusInternalServerError
}
