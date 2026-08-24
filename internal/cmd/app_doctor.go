package cmd

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// `civitai app doctor` — the listing-completeness diagnosis (civitai/civitai
// #4341's client half).
//
// 🔴 IT IS A DIAGNOSIS, NOT A REPAIR. Every finding it prints is the SERVER's
// (`computeListingProblems`); the CLI adds only the route that fixes it. That
// split is what lets the command be a pure read — `appListings.listMine` opens
// no shadow revision, unlike `app listing status`, so this one IS safe to poll.
//
// 🔴 THE EXIT CODE IS THE PRODUCT. A release script's question is "may this ship
// yet", and the answer has to be readable without parsing stderr: any BLOCKING
// problem on any app the command reported exits 1, everything else exits 0. See
// ErrListingBlocked for why 1 and not 2.

// ErrListingBlocked is the sentinel `app doctor` returns when at least one
// BLOCKING problem was found. It carries the non-zero exit, and nothing else —
// the findings have already been printed by the time it is returned.
//
// 🔴 EXIT 1, AND THAT IS A DECISION RATHER THAN A FALLTHROUGH. Exit 2 is
// documented as a mistake about the INVOCATION, and every flag and argument is
// well-formed when this fires; what is wrong is the LISTING. That is the same
// shape as an invalid manifest, which exitCodeDocs already publishes under code
// 1 as a validation verdict, and it is the code `app validate` uses for the
// same reason. It is deliberately left UNTAGGED for the exit mapper (tagging it
// civitai.ErrBadRequest — the only route to exit 2 — would move it), which
// TestDoctorBlockingExitsGeneric in cmd/civitai is what makes deliberate.
var ErrListingBlocked = errors.New("the listing has blocking problems")

// listingEditPath is the web listing editor, the fix route for the three TEXT
// problems. `%s` is the appListingId `listMine` returned.
//
// 🔴 IT IS AN appListingId PATH, NOT A SLUG PATH, and the two are different id
// spaces — `civitai/civitai:src/pages/apps/listing/[appListingId]/edit.tsx`.
// Building it from the slug would produce a well-formed URL that 404s.
const listingEditPath = "/apps/listing/%s/edit"

// The eight problem codes `computeListingProblems` emits, spelled out.
//
// 🔴 THIS IS A REMEDY TABLE, NOT A SEVERITY TABLE. The CLI never decides whether
// a code blocks — the server sends `severity` and the command reads it (see
// appapi.SeverityBlocking). What lives here is the only thing the server does
// not know: which CLI command, if any, repairs the finding.
const (
	problemMissingIcon      = "missing-icon"
	problemMissingCover     = "missing-cover"
	problemNoScreenshots    = "no-screenshots"
	problemEmptyDescription = "empty-description"
	problemEmptyTagline     = "empty-tagline"
	problemEmptyCategory    = "empty-category"
	problemBlockedMedia     = "blocked-media"
	problemScanningMedia    = "scanning-media"
)

// doctorRemedy renders the FIX line for one problem code.
//
// 🔴 EVERY ROUTE NAMED HERE IS ONE THIS CREDENTIAL CAN ACTUALLY TAKE, and that
// is the rule the table is built to, not a nicety. Advice pointing at a proc the
// CLI's OAuth token 403s on is worse than no advice: the author follows it, is
// refused, and concludes the CLI is broken.
//
//   - The three media codes route to `app listing set-icon|set-cover|
//     add-screenshot`, whose procs have carried `AppBlocksSubmit` since
//     civitai/civitai#3472 and are owner-or-accepted-seat bound — so they work
//     for `role: editor` too, and for an OFFSITE listing (civitai/cli#422's
//     by-slug fallback, measured live 2026-08-17).
//   - `blocked-media` routes to the same three, because replacing the asset IS
//     the fix. Which of the three is named by the SERVER's label, not by the
//     code — see appapi.ListingProblem.Label — so this arm cannot narrow it and
//     does not pretend to.
//   - `scanning-media` has NO fix route at all and says so. Printing a command
//     for it would tell an author to act on a state that resolves itself.
//   - The three TEXT codes route to the WEB editor, and NOT to a CLI flag. The
//     `appListings.updateListing` / `updateRevisionDraft` procs that write them
//     became CLI-reachable in civitai/civitai#4341, so a CLI route is now
//     possible — it is deliberately not in THIS command. `doctor` exits non-zero
//     as a release gate; a diagnostic that also mutates the thing it is grading
//     is a different command with a different contract. And the write itself is
//     state-dependent in a way that needs its own tests: on an APPROVED listing
//     a material change stages onto a shadow revision (`updateListing` refuses a
//     shadow id outright; `updateRevisionDraft` refuses a top-level listing), so
//     "set the tagline" is two procs and a branch, not a flag. Tracked as the
//     follow-on to this PR.
func doctorRemedy(p appapi.ListingProblem, slug, editURL string) string {
	switch p.Code {
	case problemMissingIcon:
		return "civitai app listing set-icon <file> --slug " + slug
	case problemMissingCover:
		return "civitai app listing set-cover <file> --slug " + slug
	case problemNoScreenshots:
		return "civitai app listing add-screenshot <file> --slug " + slug
	case problemBlockedMedia:
		// 🔴 THE LABEL ON THE LINE ABOVE NAMES THE SLOT, AND THIS ARM DOES NOT
		// TRY TO. `blocked-media` is emitted once per affected asset KIND and
		// the kind lives ONLY in the server's label ("Replace the blocked icon
		// …"); the code is kind-less. Guessing one of the three here would name
		// the wrong command two times out of three, so all three are offered and
		// the label is what disambiguates.
		return "replace the blocked asset named above, with --slug " + slug + " — " +
			"civitai app listing set-icon, civitai app listing set-cover, civitai app listing add-screenshot"
	case problemScanningMedia:
		// 🔴 NO COMMAND AT ALL, deliberately. A still-scanning asset resolves
		// itself; printing a fix would tell an author to act on a state that is
		// already in motion, and the likeliest action (re-attaching) restarts
		// the very scan they are waiting for.
		return "nothing to do — the scan finishes on its own; re-run `civitai app doctor` in a minute"
	case problemEmptyDescription, problemEmptyTagline, problemEmptyCategory:
		return "edit the listing in the browser: " + editURL
	}
	// 🔴 An UNKNOWN code is a NEW server code, not a bug in the caller, and it
	// gets the honest answer rather than a guessed command. The label and the
	// severity still print, so a code this CLI has never heard of is still
	// actionable — and still counted toward the exit verdict, which is what
	// matters most.
	return "no CLI route for this problem yet — open the listing in the browser: " + editURL
}

// doctorSeverity classifies one problem for RENDERING and for the exit verdict.
//
// 🔴 UNKNOWN IS NOT BLOCKING. A severity string this CLI does not recognise is
// grouped with the advisories and reported under its own heading rather than
// failing the build: a release gate that starts refusing every release the day
// the server adds a ninth vocabulary word is a gate people disable. The finding
// is never hidden — it is printed with its code, its label and its severity
// verbatim.
func doctorIsBlocking(p appapi.ListingProblem) bool {
	return p.Severity == appapi.SeverityBlocking
}

// listingStatusRemoved is the ONE lifecycle status that takes a listing out of
// the gate. The server's vocabulary is `draft|pending|approved|rejected|removed`
// (`AppListing.status`), and `removed` is the only member that is not on a path
// to publication: `rejected` is resubmitted, `draft` and `pending` are on their
// way in, `approved` is live.
const listingStatusRemoved = "removed"

// doctorGates reports whether an app's BLOCKING problems set the exit code.
//
// 🔴 A DELISTED LISTING IS REPORTED BUT DOES NOT GATE. The publish floor is a
// statement about a listing that is trying to publish; a `removed` one is not,
// so letting it fail the build makes the no-arg form permanently red for a
// reason nobody can act on. Measured on production 2026-08-24: of 11 blocking
// problems across 21 real listings, TEN were on `removed` apps. The command
// would have exited 1 forever and told nobody anything.
//
// 🔴 THE ROWS ARE STILL PRINTED, and that is not a detail. Hiding a population
// by default is how it stops existing for anyone — this codebase already paid
// for that once, with rejected submissions that had no surface at all. The
// verdict shrinks; the report does not.
//
// 🔴 ONLY AN EXPLICIT `removed` IS EXCLUDED, and an unrecognised status GATES.
// "We could not tell that it is delisted" is not "it is delisted", and the two
// directions are not symmetric: wrongly excluding means a real blocking problem
// silently stops failing the build, which is the failure this command exists to
// prevent. So the unknown case takes the loud direction.
//
// The exclusion is not permanent, and nothing is stranded by it: an owner who
// republishes a removed listing moves its status off `removed`, and the next run
// gates on it again with no CLI change.
func doctorGates(status string) bool {
	return !strings.EqualFold(strings.TrimSpace(status), listingStatusRemoved)
}

// doctorAppJSON is one app in the `--json` payload. PUBLISHED CONTRACT — the
// README documents it, so a field is renamed or dropped only deliberately.
//
// 🔴 `blocking` AND `advisory` ARE SEPARATE ARRAYS RATHER THAN ONE LIST PLUS A
// severity FIELD, because the question a consumer has is "may this ship" and
// that must be answerable without re-implementing the severity split the human
// rendering already did. Each array is never null — an app with none gets `[]`,
// so `.blocking | length` answers for every app.
type doctorAppJSON struct {
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	AppListingID string `json:"appListingId"`
	// AppBlockID is explicitly null for an offsite listing and for an app whose
	// first version is not approved yet — a real state, not an omission.
	AppBlockID *string `json:"appBlockId"`
	Status     string  `json:"status"`
	Role       string  `json:"role"`
	// Delisted means this listing's status is `removed`, so its blocking
	// problems are REPORTED but do NOT set the exit code.
	//
	// 🔴 IT IS A FIELD ON THE ROW, NOT A SEPARATE ARRAY, AND THAT IS THE
	// DISCLOSURE DECISION. Moving delisted apps into a sibling key would let a
	// consumer iterate `apps` and never see them — the population-becomes-
	// unreachable failure this codebase already paid for once, when 100% of
	// rejected submissions had no surface at all. Every app the caller can work
	// on is in `apps`; this bit says which of them the verdict counted.
	Delisted bool                `json:"delisted"`
	Blocking []doctorProblemJSON `json:"blocking"`
	Advisory []doctorProblemJSON `json:"advisory"`
}

// doctorProblemJSON is one finding. `code`, `label` and `severity` are the
// SERVER's own values, passed through unmodified; `fix` is the CLI's contribution
// and is the same sentence the human rendering prints.
type doctorProblemJSON struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Severity string `json:"severity"`
	Fix      string `json:"fix"`
}

// doctorSummaryJSON is the whole-run verdict, so a script can branch without
// walking the apps.
//
// 🔴 `Blocking` AND `Gating` ARE DIFFERENT NUMBERS AND ONLY ONE OF THEM SETS
// `ok`. `Blocking` counts every blocking problem found, so it matches the
// `blocking` arrays a consumer can see. `Gating` counts only those on listings
// that can still publish — delisted (`removed`) listings are excluded — and
// THAT is the exit code. Publishing one number would have forced a choice
// between a total that disagrees with the arrays and a verdict that disagrees
// with the exit code; both are worse than naming the two quantities.
//
// Measured on production 2026-08-24 with 21 real listings: 11 blocking, of
// which 10 sat on `removed` apps. A single number would have made the no-arg
// form permanently red for reasons nobody can act on.
type doctorSummaryJSON struct {
	Apps     int `json:"apps"`
	Blocking int `json:"blocking"`
	Advisory int `json:"advisory"`
	// Gating is the subset of Blocking that sets the exit code.
	Gating int `json:"gating"`
	// Delisted is how many of Apps are `removed`.
	Delisted int `json:"delisted"`
}

// doctorJSON is the `--json` payload.
//
// 🔴 `ok` IS THE SAME FIELD `app validate --json` PUBLISHES, and it means the
// same thing: the command's verdict, and the structured form of the exit code.
// It follows `summary.gating`, NOT `summary.blocking` — advisories never move
// it, and neither does a blocking problem on a DELISTED listing. A script that
// wants "is anything wrong anywhere" reads `summary.blocking`; a script that
// wants "may this ship" reads `ok`.
type doctorJSON struct {
	OK      bool              `json:"ok"`
	Apps    []doctorAppJSON   `json:"apps"`
	Summary doctorSummaryJSON `json:"summary"`
}

func newAppDoctorCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "doctor [slug]",
		Short: "Diagnose what is incomplete or blocked on your App store listings",
		Long: `Report what is missing or blocked on your App store listings, and how to fix it.

With no argument it checks EVERY listing you own or hold an accepted
collaborator seat on. Pass an app slug to check just that one.

Findings come from the platform, grouped per app, blocking first:

  BLOCKING   the listing cannot publish until it is fixed — a missing icon or
             cover, or an asset the content scan BLOCKED.
  ADVISORY   recommended, but nothing is held up — no screenshots, or an empty
             description, tagline or category.

An app with nothing wrong is reported as complete, explicitly. A blank space is
not an answer.

The three TEXT problems (description / tagline / category) are fixed in the
browser: this command prints the listing's editor URL. The media problems are
fixed with ` + "`civitai app listing set-icon`" + ` / ` + "`set-cover`" + ` /
` + "`add-screenshot`" + `, and the exact command is printed beside each finding.

DELISTED LISTINGS: an app whose status is 'removed' is still reported, in its
own section, but its blocking problems do NOT set the exit code. The publish
floor is a statement about a listing that is trying to publish, and a delisted
one is not — without this, one old removed app would fail every run forever.

EXIT CODES: 1 when a blocking problem was found on a listing that can still
publish, 0 otherwise — including when only advisories were found, when the only
blocking problems are on delisted listings, and when you have no listings at
all. So it gates a release script directly:

    civitai app doctor my-app || exit 1

Every other exit code keeps its usual meaning (3 not authorized, 4 no such app,
5 transport). --json emits the same verdict as one object and uses the same exit
codes, so a script must branch on the code before trusting the payload.

Unlike ` + "`civitai app listing status`" + `, this is a PURE READ — it opens no
revision draft on a live listing, so it is safe to run in a loop.`,
		Example: `  civitai app doctor                 # every app you can work on
  civitai app doctor my-app          # just one
  civitai app doctor --json | jq -e .ok
  civitai app doctor my-app || echo "not ready to publish"`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Token() == "" {
				return civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN)"))
			}
			client, err := newListingClient()
			if err != nil {
				return err
			}
			slug := ""
			if len(args) == 1 {
				slug = strings.TrimSpace(args[0])
			}
			rows, err := client.ListMyListings(cmdCtx(cmd))
			if err != nil {
				return err
			}
			return runAppDoctor(cmd.OutOrStdout(), rows, slug, cfg.BaseURL(), jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false,
		"emit the findings as JSON (scriptable); the exit code is unchanged — 1 when anything is blocking")
	return cmd
}

// doctorNoSuchApp is the refusal for a slug that names none of the caller's
// listings. It is tagged ErrNotFound (exit 4) rather than being reported as a
// clean run, because "you own no app called that" and "that app is fine" are
// opposite answers and a release gate must not confuse them.
func doctorNoSuchApp(slug string, rows []appapi.MyListing) error {
	known := make([]string, 0, len(rows))
	for _, r := range rows {
		known = append(known, r.Slug)
	}
	sort.Strings(known)
	msg := fmt.Sprintf("no listing of yours is called %q", slug)
	switch {
	case len(known) == 0:
		msg += " — you own no App listings and hold no collaborator seats yet; `civitai app submit` creates one"
	default:
		msg += " — you can work on: " + strings.Join(known, ", ")
	}
	return civitai.Tag(civitai.ErrNotFound, errors.New(msg))
}

// runAppDoctor is the command core: given the rows the server returned, it
// selects, renders and returns the verdict. Separated from the cobra wiring so
// the verdict can be exercised without a server, and so the SAME selection and
// the SAME counting feed both renderings.
func runAppDoctor(out io.Writer, rows []appapi.MyListing, slug, baseURL string, jsonOut bool) error {
	selected := rows
	if slug != "" {
		selected = nil
		for _, r := range rows {
			// 🔴 appapi.SameSlug, not `==` — the shared predicate (see
			// internal/appapi/slug.go). The user types this value, so a padded
			// or mis-cased spelling reaches here routinely, and an exact compare
			// would answer "no such app" for an app the caller owns.
			if appapi.SameSlug(slug, r.Slug) {
				selected = append(selected, r)
			}
		}
		if len(selected) == 0 {
			return doctorNoSuchApp(slug, rows)
		}
	}

	payload := doctorPayload(selected, baseURL)
	if jsonOut {
		// 🔴 The JSON path does NOT go through the human renderer, and must not:
		// internal/ui/CONVENTION.md rule 1 is that machine-readable output
		// carries no styling, and that renderer emits ui.Success/ui.Warn/ui.Code.
		if err := writeJSON(out, payload); err != nil {
			return err
		}
	} else {
		printDoctorReport(out, payload)
	}
	// 🔴 `Gating`, NOT `Blocking`. See doctorGates: a delisted listing's blocking
	// problems are printed and do not set the exit code.
	if payload.Summary.Gating > 0 {
		// 🔴 The findings are already on stdout; this error exists ONLY to carry
		// the exit code, so its message must not read as a second, competing
		// report of the same facts. It quotes the GATING count for the same
		// reason — a message naming a bigger number than the verdict acted on
		// would send a reader hunting for problems the gate deliberately ignored.
		return fmt.Errorf("%w: %d blocking problem(s) on publishable listing(s) — see the report above",
			ErrListingBlocked, payload.Summary.Gating)
	}
	return nil
}

// doctorPayload builds the whole verdict — the ONE place the severity split and
// the counting happen, so the human rendering and `--json` can never disagree
// about whether a run was clean.
func doctorPayload(rows []appapi.MyListing, baseURL string) doctorJSON {
	editBase := strings.TrimRight(baseURL, "/")
	out := doctorJSON{
		Apps:    make([]doctorAppJSON, 0, len(rows)),
		Summary: doctorSummaryJSON{Apps: len(rows)},
	}
	// 🔴 GATING APPS FIRST, DELISTED LAST — in the PAYLOAD, not only in the
	// human rendering. The renderer and `--json` read the same slice, which is
	// what keeps them from disagreeing about a finding or a count; ordering here
	// rather than at the printer keeps that property. Stable within each group,
	// so the server's own order survives.
	ordered := make([]appapi.MyListing, 0, len(rows))
	for _, r := range rows {
		if doctorGates(r.Status) {
			ordered = append(ordered, r)
		}
	}
	for _, r := range rows {
		if !doctorGates(r.Status) {
			ordered = append(ordered, r)
		}
	}
	for _, r := range ordered {
		editURL := editBase + fmt.Sprintf(listingEditPath, r.AppListingID)
		gates := doctorGates(r.Status)
		app := doctorAppJSON{
			Slug:         r.Slug,
			Name:         r.Name,
			AppListingID: r.AppListingID,
			AppBlockID:   r.AppBlockID,
			Status:       r.Status,
			Role:         r.Role,
			Delisted:     !gates,
			Blocking:     []doctorProblemJSON{},
			Advisory:     []doctorProblemJSON{},
		}
		for _, p := range r.Problems {
			row := doctorProblemJSON{
				Code:     p.Code,
				Label:    p.Label,
				Severity: p.Severity,
				Fix:      doctorRemedy(p, r.Slug, editURL),
			}
			if doctorIsBlocking(p) {
				app.Blocking = append(app.Blocking, row)
				continue
			}
			app.Advisory = append(app.Advisory, row)
		}
		out.Summary.Blocking += len(app.Blocking)
		out.Summary.Advisory += len(app.Advisory)
		if gates {
			// The ONLY place the exit code is accumulated.
			out.Summary.Gating += len(app.Blocking)
		} else {
			out.Summary.Delisted++
		}
		out.Apps = append(out.Apps, app)
	}
	out.OK = out.Summary.Gating == 0
	return out
}

// printDoctorReport renders the human view from the SAME payload `--json`
// emits, so the two cannot disagree about a finding or a count.
func printDoctorReport(w io.Writer, payload doctorJSON) {
	st := ui.For(w)
	if len(payload.Apps) == 0 {
		// 🔴 An empty run gets a SENTENCE, not a blank. Silence here is
		// indistinguishable from a read that failed and was swallowed.
		fmt.Fprintf(w, "No App listings to check — you own none and hold no collaborator seats. %s creates one.\n",
			st.Code("civitai app submit"))
		return
	}
	delistedHeaderShown := false
	for i, app := range payload.Apps {
		if i > 0 {
			fmt.Fprintln(w)
		}
		// 🔴 THE DELISTED SECTION IS ANNOUNCED, ONCE, WHERE IT STARTS. The
		// payload is already ordered gating-first (see doctorPayload), so this
		// fires on the first delisted row. Without the heading a reader sees an
		// app carrying BLOCKING findings while the command exits 0 and has no
		// way to tell that from a broken gate.
		if app.Delisted && !delistedHeaderShown {
			delistedHeaderShown = true
			fmt.Fprintln(w, st.Info("Delisted (status 'removed') — reported for completeness; these do NOT set the exit code."))
		}
		fmt.Fprintf(w, "%s  (%s, %s)\n", st.Bold(app.Slug), app.Status, app.Role)
		if len(app.Blocking) == 0 && len(app.Advisory) == 0 {
			// 🔴 EXPLICITLY CLEAN. An app with nothing wrong must not render as
			// a gap: a missing section reads the same as a broken read, and the
			// whole point of the command is a verdict a script and a human can
			// both trust.
			fmt.Fprintln(w, "  "+st.Success("No problems — this listing is complete."))
			continue
		}
		printDoctorGroup(w, st, "BLOCKING (cannot publish until fixed)", app.Blocking, st.Warn)
		printDoctorGroup(w, st, "ADVISORY (recommended, nothing is held up)", app.Advisory, st.Info)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%d app(s) checked — %d blocking, %d advisory.\n",
		payload.Summary.Apps, payload.Summary.Blocking, payload.Summary.Advisory)
	// 🔴 THE DISCREPANCY IS EXPLAINED WHEREVER IT CAN EXIST, which is whenever a
	// delisted app was reported at all — not only when one carries a blocking
	// problem. A reader who sees the section above needs the rule stated whether
	// or not it changed the arithmetic this time, and a line that appears only
	// sometimes is a line nobody learns to look for.
	if payload.Summary.Delisted > 0 {
		fmt.Fprintf(w, "%d of them are delisted; %d blocking problem(s) on publishable listings set the exit code.\n",
			payload.Summary.Delisted, payload.Summary.Gating)
	}
}

// printDoctorGroup renders one severity group, or nothing when it is empty.
func printDoctorGroup(w io.Writer, st ui.Styler, heading string, problems []doctorProblemJSON, style func(string) string) {
	if len(problems) == 0 {
		return
	}
	fmt.Fprintf(w, "  %s\n", style(heading))
	for _, p := range problems {
		fmt.Fprintf(w, "    %s  %s\n", p.Code, p.Label)
		fmt.Fprintf(w, "      Fix: %s\n", st.Code(p.Fix))
	}
}
