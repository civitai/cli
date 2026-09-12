package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/genapi"
	civitai "github.com/civitai/cli/pkg/civitai"
)

// THE BEHAVIOURAL HALF OF civitai/cli#552 — the real renderers, driven with a
// forgery payload.
//
// 🔴 THE STRUCTURAL LEDGER NEXT DOOR TYPE-CHECKS PAST A WRONG ARGUMENT. It sees
// "safeTermSingle reaches a cell in this function", which is satisfied by
// sanitising ONE of six columns, by sanitising the wrong variable, or by a
// helper that sanitises a value the renderer then discards. Only driving the
// renderer says what a hostile field does on screen.
//
// The payload carries BOTH vectors at once and the assertion is a single
// contiguous string, which is what makes it precise:
//
//	sent:     "FORGE-x" + "\t" + "FORGED-COL" + "\n" + "FORGED-ROW"
//	on screen: "FORGE-x FORGED-COL FORGED-ROW"
//
// A surviving `\n` splits that string across two lines, so the match fails. A
// surviving `\t` is EATEN by tabwriter and re-emitted as its column padding —
// two or more spaces — so the match fails there too. And because the match is on
// the payload's own words, a renderer that simply drops the field fails as well:
// "the class is gone" and "the value arrived" are one assertion, the same pairing
// TestReadRenderersStripTheInvisibleClass uses for the invisible class.

// 🔴 THE MARKERS ARE SHORT ON PURPOSE. `images search` truncates its BASE MODEL
// column to 24 runes, so a verbose payload is cut there and the contiguity
// assertion fails for a reason that has nothing to do with forgery. The longest
// payload any case below produces is 24 runes; keep new labels under 11
// characters, or the case will fail on truncation and read as a leak.
const (
	forgedCol = "C0L"
	forgedRow = "R0W"
)

// forgeCell is a server value carrying a tab and a newline between three
// distinguishable words.
func forgeCell(label string) string {
	return "FORGE-" + label + "\t" + forgedCol + "\n" + forgedRow
}

// forgeWant is what forgeCell(label) must look like in a cell.
func forgeWant(label string) string {
	return "FORGE-" + label + " " + forgedCol + " " + forgedRow
}

// TestForgeCellCarriesBothVectors is the control ON THE FIXTURE. Every assertion
// in the table below is about bytes that must be present before the renderer
// runs; a payload that lost its tab would let every case report "no forgery" and
// measure nothing.
func TestForgeCellCarriesBothVectors(t *testing.T) {
	in := forgeCell("probe")
	if !strings.ContainsRune(in, '\t') || !strings.ContainsRune(in, '\n') {
		t.Fatalf("CONTROL failure, not a finding: the fixture carries %q — it must contain BOTH a tab "+
			"(the column-injection vector) and a newline (the row-forgery one)", in)
	}
	if got := safeTermSingle(in); got != forgeWant("probe") {
		t.Fatalf("CONTROL failure, not a finding: safeTermSingle(%q) = %q, want %q — the cases below assert "+
			"against this exact shape", in, got, forgeWant("probe"))
	}
	if forgeCell("a") == forgeCell("b") {
		t.Fatal("CONTROL failure, not a finding: forgeCell ignores its label, so every per-field assertion " +
			"collapses into one and a dropped column cannot be named")
	}
	// The payload must be able to forge: fed to the OLD gate it still carries
	// both. This is the negative control on the fixture — it proves the cases
	// below are not passing because the payload is inert.
	if old := safeTerm(in); !strings.ContainsRune(old, '\t') || !strings.ContainsRune(old, '\n') {
		t.Fatalf("CONTROL failure, not a finding: safeTerm(%q) = %q — safeTerm is supposed to KEEP \\n and "+
			"\\t, so if it does not, this payload no longer demonstrates the hazard #552 is about", in, old)
	}
}

// twCols counts the cells in one flushed tabwriter line. tabwriter consumes the
// tab and pads with its padchar (a space here), so runs of 2+ spaces are what
// separate columns on screen — asserting on a literal tab in the OUTPUT cannot
// fail on this path and would read as the test's thesis while proving nothing
// (that trap shipped here once: an assertion on a literal tab in tabwriter's
// output, whose failure message read as the test's thesis while being unable to
// fail at all).
var twColSep = regexp.MustCompile(` {2,}`)

func twCols(line string) int { return len(twColSep.Split(strings.TrimSpace(line), -1)) }

func TestTabwriterRenderersCannotBeForged(t *testing.T) {
	cmdOut := func(render func(c *cobra.Command)) string {
		c, out, _ := genCmd("")
		render(c)
		return out.String()
	}
	writerOut := func(render func(w *bytes.Buffer)) string {
		var buf bytes.Buffer
		render(&buf)
		return buf.String()
	}

	for _, tc := range []struct {
		// surface names the renderer, for the failure message.
		surface string
		// fields are the labels this renderer must put on screen, each with its
		// own marker so a dropped column is named.
		fields []string
		// wantLines, when non-zero, is the number of lines the renderer's own
		// contract says it emits for this fixture — a header plus one row for a
		// table. It is the direct statement of "no row was forged".
		wantLines int
		// squareCols asserts every line carries the same number of columns as the
		// first, which is how an INJECTED column shows up in a table that has a
		// header.
		squareCols bool
		// alsoWant are literal contiguous strings this case's output must carry,
		// for a fixture whose payload is not built by forgeCell. It exists for the
		// one HISTORICALLY MEASURED payload (see the `images search` cases) — a
		// case with neither fields nor alsoWant asserts only line and column
		// counts, and the control below refuses that.
		alsoWant []string
		render   func() string
	}{
		{
			surface:    "`creators search` (printCreatorList)",
			fields:     []string{"crname", "crlink"},
			wantLines:  2,
			squareCols: true,
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printCreatorList(c, []civitai.CreatorItem{{
						Username: civitai.FlexString(forgeCell("crname")),
						Link:     forgeCell("crlink"),
					}})
				})
			},
		},
		{
			surface:    "`tags search` (printTagList)",
			fields:     []string{"tgname", "tglink"},
			wantLines:  2,
			squareCols: true,
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printTagList(c, []civitai.TagItem{{
						Name: forgeCell("tgname"),
						Link: forgeCell("tglink"),
					}})
				})
			},
		},
		{
			surface:    "`models search` (printModelList)",
			fields:     []string{"mlname", "mltype", "mlcreator"},
			wantLines:  2,
			squareCols: true,
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printModelList(c, []civitai.ModelListItem{{
						ID:      7,
						Name:    forgeCell("mlname"),
						Type:    forgeCell("mltype"),
						Creator: &civitai.Creator{Username: civitai.FlexString(forgeCell("mlcreator"))},
					}})
				})
			},
		},
		{
			// Only the VERSION TABLE is a tabwriter here; the header lines above it
			// are plain Fprintf and belong to the residual class named in
			// tabwriter_ledger_test.go's header, so they are left benign on purpose
			// — a forged value there would fail this case for a reason #552 is not
			// about.
			surface: "`models get`'s version table (printModelDetail, nonModelFileMarker)",
			fields:  []string{"mdver", "mdbase", "mdmark"},
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printModelDetail(c, &civitai.ModelDetail{
						ID: 7, Name: "benign", Type: "Checkpoint",
						ModelVersions: []civitai.ModelVersionSummary{{
							ID:        9,
							Name:      forgeCell("mdver"),
							BaseModel: forgeCell("mdbase"),
							Files: []civitai.ModelVersionFile{{
								Primary: true,
								Type:    forgeCell("mdmark"),
							}},
						}},
					})
				})
			},
		},
		{
			surface: "`model-versions get`'s file table (printModelVersionDetail)",
			fields:  []string{"mvfile", "mvftype"},
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printModelVersionDetail(c, &civitai.ModelVersionDetail{
						ID: 9, ModelID: 7, Name: "benign", BaseModel: "SDXL",
						Files: []civitai.ModelVersionFile{{
							Name: forgeCell("mvfile"),
							Type: forgeCell("mvftype"),
						}},
					})
				})
			},
		},
		{
			surface:    "`collections search` (printCollectionList)",
			fields:     []string{"clname", "cltype", "clowner"},
			wantLines:  2,
			squareCols: true,
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printCollectionList(c, []civitai.CollectionListItem{{
						ID:   5,
						Name: forgeCell("clname"),
						Type: forgeCell("cltype"),
						User: &civitai.CollectionUser{Username: civitai.FlexString(forgeCell("clowner"))},
					}})
				})
			},
		},
		{
			surface:    "`articles search` (printArticleList)",
			fields:     []string{"artitle", "arauthor", "ardate"},
			wantLines:  2,
			squareCols: true,
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printArticleList(c, []civitai.ArticleListItem{{
						ID:    4,
						Title: forgeCell("artitle"),
						// shortDate returns the RAW string when it does not parse as
						// RFC3339, so the PUBLISHED column is server text too.
						PublishedAt: forgeCell("ardate"),
						User:        &civitai.ArticleUser{Username: civitai.FlexString(forgeCell("arauthor"))},
					}})
				})
			},
		},
		{
			surface:    "`app list` (printAppList, appCardAuthor)",
			fields:     []string{"apname", "apslug", "apkind", "apcat", "apauth"},
			wantLines:  2,
			squareCols: true,
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printAppList(c, []civitai.AppCard{{
						Name:     forgeCell("apname"),
						Slug:     forgeCell("apslug"),
						Kind:     forgeCell("apkind"),
						Category: forgeCell("apcat"),
						Creator:  &civitai.ListingCreatorChip{Username: civitai.FlexString(forgeCell("apauth"))},
					}})
				})
			},
		},
		{
			// #569 fixed this one; the case is here so a later refactor cannot undo
			// it silently.
			surface:    "`images search` (printImageList)",
			fields:     []string{"imname", "imbase", "imnsfw", "imurl"},
			wantLines:  2,
			squareCols: true,
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printImageList(c, []civitai.ImageItem{{
						ID:        11,
						Username:  civitai.FlexString(forgeCell("imname")),
						BaseModel: forgeCell("imbase"),
						NSFWLevel: forgeCell("imnsfw"),
						URL:       forgeCell("imurl"),
					}})
				})
			},
		},
		{
			// 🔴 THE HISTORICALLY MEASURED PAYLOAD, KEPT AS A SECOND FIXTURE. This
			// is the exact username #569 measured the column-injection forgery
			// with — six tabs, no newline — folded in when
			// TestTabForgeryIsNeutralisedInTheRealImagesTable was deleted as
			// strictly subsumed by the case above (that test asserted a 2-line,
			// square-column render of a WEAKER payload). The generic forgeCell
			// payload carries one tab and one newline; this one carries the shape
			// an attacker actually sends, where every real column of the row is
			// supplied by the attacker and the real values are pushed off-screen.
			surface:    "`images search`, the #569 payload (printImageList)",
			wantLines:  2,
			squareCols: true,
			alsoWant: []string{
				"alice SDXL 9x9 None 0 0 https://evil.example/steal",
				// The REAL column values must still occupy their real columns — a
				// guard that simply ate the username would pass every count above.
				"https://img/1",
			},
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printImageList(c, []civitai.ImageItem{{
						ID: 1, URL: "https://img/1", NSFWLevel: "None", BaseModel: "SDXL",
						Username: civitai.FlexString("alice\tSDXL\t9x9\tNone\t0\t0\thttps://evil.example/steal"),
					}})
				})
			},
		},
		{
			surface:    "`app status` (printSubmissionTable)",
			fields:     []string{"stblock", "stver", "ststatus", "stdeploy", "sturl"},
			wantLines:  2,
			squareCols: true,
			render: func() string {
				deploy := forgeCell("stdeploy")
				live := forgeCell("sturl")
				return writerOut(func(w *bytes.Buffer) {
					printSubmissionTable(w, []appapi.Submission{{
						BlockID:     forgeCell("stblock"),
						Version:     forgeCell("stver"),
						Status:      forgeCell("ststatus"),
						DeployState: &deploy,
						LiveURL:     &live,
						SubmittedAt: "2026-09-01T10:00:00Z",
					}})
				})
			},
		},
		{
			// LiveURL is set so the "Not live yet — <blockId>.civit.ai" sentence,
			// which is NOT a cell, does not render: that line is part of the
			// residual class, not of this guard.
			surface: "`app status --id` (printSubmissionDetail)",
			fields:  []string{"sdblock", "sdver", "sdid", "sdstatus", "sddetail", "sdcommit"},
			render: func() string {
				detail := forgeCell("sddetail")
				commit := forgeCell("sdcommit")
				live := "https://example.civit.ai"
				return writerOut(func(w *bytes.Buffer) {
					printSubmissionDetail(w, &appapi.Submission{
						BlockID:      forgeCell("sdblock"),
						Version:      forgeCell("sdver"),
						ID:           forgeCell("sdid"),
						Status:       forgeCell("sdstatus"),
						DeployDetail: &detail,
						SourceCommit: &commit,
						LiveURL:      &live,
						SubmittedAt:  "2026-09-01T10:00:00Z",
					})
				})
			},
		},
		{
			surface: "`app listing status` (printListingStatus)",
			fields:  []string{"lsstatus"},
			render: func() string {
				return writerOut(func(w *bytes.Buffer) {
					printListingStatus(w, "my-app",
						&appapi.ListingRef{Status: forgeCell("lsstatus")},
						&appapi.ListingEditView{})
				})
			},
		},
		{
			surface: "`app metrics` (printAppMetrics)",
			fields:  []string{"amfrom", "amto", "amgran", "amscope", "amendpoint"},
			render: func() string {
				return writerOut(func(w *bytes.Buffer) {
					printAppMetrics(w, "my-app", &appapi.AppAnalytics{
						Range: appapi.AnalyticsRange{
							From:        forgeCell("amfrom"),
							To:          forgeCell("amto"),
							Granularity: forgeCell("amgran"),
						},
						Engagement: appapi.AnalyticsEngagement{
							APICalls:     10,
							TopScopes:    []appapi.AnalyticsScopeCount{{Scope: forgeCell("amscope"), Count: 1}},
							TopEndpoints: []appapi.AnalyticsEndpointCount{{Endpoint: forgeCell("amendpoint"), Count: 1}},
						},
						Views: &appapi.AnalyticsViews{Count: 1},
					})
				})
			},
		},
		{
			surface: "`workflows get` header (printWorkflow)",
			fields:  []string{"wfid", "wfstatus", "wfcreated"},
			render: func() string {
				return writerOut(func(w *bytes.Buffer) {
					var errb bytes.Buffer
					printWorkflow(w, &errb, &genapi.Workflow{
						ID:        forgeCell("wfid"),
						Status:    forgeCell("wfstatus"),
						CreatedAt: forgeCell("wfcreated"),
					})
				})
			},
		},
		{
			surface:    "`workflows list` (printWorkflowList)",
			fields:     []string{"wlid", "wlstatus", "wlcreated"},
			wantLines:  2,
			squareCols: true,
			render: func() string {
				return writerOut(func(w *bytes.Buffer) {
					var errb bytes.Buffer
					printWorkflowList(w, &errb, &genapi.WorkflowPage{
						Items: []genapi.ListedWorkflow{{
							ID:        forgeCell("wlid"),
							Status:    forgeCell("wlstatus"),
							CreatedAt: forgeCell("wlcreated"),
						}},
					}, workflowsListOpts{})
				})
			},
		},
		{
			surface: "the settlement table (reportWorkflowSettlement)",
			fields:  []string{"settype"},
			render: func() string {
				payload := fmt.Sprintf(`{"id":"wf_1","status":"failed","createdAt":"2026-08-10T09:00:00Z",
					"transactions":{"list":[{"type":%s,"amount":8}]},"steps":[]}`,
					mustJSONString(forgeCell("settype")))
				var wf genapi.Workflow
				if err := json.Unmarshal([]byte(payload), &wf); err != nil {
					t.Fatalf("CONTROL failure, not a finding: settlement fixture does not decode: %v", err)
				}
				return writerOut(func(w *bytes.Buffer) {
					var errb bytes.Buffer
					if !reportWorkflowSettlement(w, &errb, &wf) {
						t.Fatal("CONTROL failure, not a finding: the settlement block did not print, so this " +
							"case asserts nothing")
					}
				})
			},
		},
		{
			surface: "the submit receipt (printSubmitResult)",
			fields:  []string{"srid", "srstatus"},
			render: func() string {
				return writerOut(func(w *bytes.Buffer) {
					var errb bytes.Buffer
					printSubmitResult(w, &errb, &genapi.SubmitResult{
						ID:     forgeCell("srid"),
						Status: forgeCell("srstatus"),
					}, "ext_1", "https://civitai.com", false)
				})
			},
		},
		{
			// printCostMap takes the tabwriter as a PARAMETER — this drives it the
			// way printGenerateQuote does, which is the only way its cells exist.
			surface: "the pre-spend cost table (printCostMap)",
			fields:  []string{"cmkey"},
			render: func() string {
				return writerOut(func(w *bytes.Buffer) {
					tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
					printCostMap(tw, "factors", map[string]float64{forgeCell("cmkey"): 2})
					_ = tw.Flush()
				})
			},
		},
		{
			// 🔴 THE FIELDS ARE SET DIRECTLY, BYPASSING describeVersion, AND THAT IS
			// THE POINT — civitai/cli#575 R1. These cells are normally gated by
			// describeVersion on the way IN, and that used to be the whole claim: a
			// ledger row naming describeVersion as the "upstream". The row resolved
			// only the NAME, so it was equally satisfied by any unrelated sanitising
			// function, and nothing checked that the value in the field had actually
			// been through one. Driving the renderer with a hostile field is what
			// asks the question the row could not: whatever reaches
			// resolvedGraph.checkpoint / .checkpointNote / .loras, does this screen
			// forge? It is a PRE-SPEND surface — the table a user reads before
			// agreeing to be charged.
			surface: "the pre-spend quote screen (printGenerateQuote)",
			fields:  []string{"gqckpt", "gqnote", "gqlora"},
			render: func() string {
				return writerOut(func(w *bytes.Buffer) {
					built := &resolvedGraph{
						checkpoint:     forgeCell("gqckpt"),
						checkpointNote: forgeCell("gqnote"),
						loras:          []string{forgeCell("gqlora")},
					}
					printGenerateQuote(w, io.Discard, built, generateOpts{},
						&genapi.WhatIfResult{Ready: true, Cost: &genapi.WorkflowCost{Base: 1, Total: 1}})
				})
			},
		},
	} {
		t.Run(tc.surface, func(t *testing.T) {
			got := tc.render()
			lines := strings.Split(strings.TrimRight(got, "\n"), "\n")

			// CONTROL on the CASE: line and column counts alone would pass against
			// a renderer that dropped every server field, so a case must name at
			// least one string the output has to carry.
			if len(tc.fields) == 0 && len(tc.alsoWant) == 0 {
				t.Fatalf("CONTROL failure, not a finding: %s names neither `fields` nor `alsoWant`, so it asserts "+
					"only that the shape is square — which a renderer that ate the payload also satisfies.", tc.surface)
			}
			for _, w := range tc.alsoWant {
				if !strings.Contains(got, w) {
					t.Errorf("%s did not render %q contiguously.\n got:\n%s\n\n"+
						"A surviving newline splits the string across lines; a surviving tab is consumed by "+
						"tabwriter as a COLUMN DELIMITER and re-emitted as padding.", tc.surface, w, got)
				}
			}
			for _, f := range tc.fields {
				// CONTROL on the fixture, per field: a payload longer than the
				// narrowest truncated column in this package cannot be asserted
				// contiguously, and would fail below as though the gate had leaked.
				if n := len([]rune(forgeWant(f))); n > 24 {
					t.Fatalf("CONTROL failure, not a finding: the %q payload is %d runes; `images search` "+
						"truncates a column at 24, so this case could fail on truncation rather than on "+
						"forgery. Shorten the label.", f, n)
				}
				if strings.Contains(got, forgeWant(f)) {
					continue
				}
				t.Errorf("%s did not render the %s field as one inert cell.\n want the contiguous string: %q\n got:\n%s\n\n"+
					"A surviving newline splits that string across lines (a forged row at column zero); a "+
					"surviving tab is consumed by tabwriter as a COLUMN DELIMITER and re-emitted as padding "+
					"(an injected, perfectly aligned column); and a renderer that dropped the field fails here "+
					"too, which is deliberate — see safeTermSingle (civitai/cli#552).",
					tc.surface, f, forgeWant(f), got)
			}
			for i, line := range lines {
				if strings.HasPrefix(strings.TrimSpace(line), forgedRow) {
					t.Errorf("%s: line %d begins with the forged payload — server text reached the start of a "+
						"line, where it is indistinguishable from output the CLI wrote itself:\n%s",
						tc.surface, i+1, got)
				}
			}
			if tc.wantLines > 0 && len(lines) != tc.wantLines {
				t.Errorf("%s emitted %d line(s), want %d. One hostile value produced extra rows:\n%s",
					tc.surface, len(lines), tc.wantLines, got)
			}
			if tc.squareCols && len(lines) > 1 {
				want := twCols(lines[0])
				for i, line := range lines[1:] {
					if twCols(line) != want {
						t.Errorf("%s: line %d has %d column(s) but the header has %d — server text injected a "+
							"column, and tabwriter aligned it:\n%s", tc.surface, i+2, twCols(line), want, got)
					}
				}
			}
		})
	}
}

// escForge is the payload for the surfaces that are NOT cells: a cursor-up plus
// line-clear, which is the exact #399 vector safeTerm exists to remove, wrapped
// in words so "the class is gone" and "the value arrived" stay one assertion.
const escForge = "reason-head\x1b[1A\x1b[2KOVERWRITTEN\nFORGED-LINE"

// TestGatedRenderersDoNotForgeOutsideTheirTable — civitai/cli#552, second round.
//
// 🔴 A ROW IN tabwriterRenderers IS A CLAIM ABOUT CELLS, NOT ABOUT A COMMAND.
// Both renderers below hold one, and both print SERVER TEXT one line under their
// own flushed table, where the ledger next door cannot see it and the row reads
// as coverage of the whole surface:
//
//   - printSubmissionDetail wrote *s.RejectionReason, *s.ApprovalNotes,
//     *s.LiveURL and s.BlockID with NO gate at all — a rejection reason of
//     "\x1b[1A\x1b[2KOVERWRITTEN" put a RAW ESC on stdout and overwrote the
//     "Reviewed:" row the same function had just written;
//   - printListingStatus wrote the screenshot id and caption raw, so a caption
//     of "…\nApp:  forged-line" emitted `App:  forged-line` at column zero, five
//     lines under the real `App:` row that function writes through its tabwriter.
//
// The split is by SHAPE, not by surface: a rejection reason is legitimately
// multi-line free text, so it gets safeTerm + indentContinuation (its line breaks
// survive but cannot reach column zero); everything else here is single-line
// metadata and gets safeTermSingle.
func TestGatedRenderersDoNotForgeOutsideTheirTable(t *testing.T) {
	render := func(f func(w *bytes.Buffer)) string {
		var buf bytes.Buffer
		f(&buf)
		return buf.String()
	}
	// noColumnZero is the property every case shares: no line of the rendered
	// block may BEGIN with attacker-supplied text, because a line at column zero
	// is indistinguishable from one the CLI wrote itself.
	noColumnZero := func(t *testing.T, out string, markers ...string) {
		t.Helper()
		for i, line := range strings.Split(out, "\n") {
			for _, m := range markers {
				if strings.HasPrefix(line, m) {
					t.Errorf("line %d begins with the forged payload %q — server text reached column zero:\n%s",
						i+1, m, out)
				}
			}
		}
	}
	// noRawEscape pairs with it: the control class must be gone, and the words
	// must still be there, so a renderer that dropped the field fails too.
	noRawEscape := func(t *testing.T, out string, wantWords ...string) {
		t.Helper()
		if strings.ContainsRune(out, '\x1b') {
			t.Errorf("a RAW ESC reached the output — this is the #399 vector, and safeTerm is what removes it:\n%q", out)
		}
		for _, w := range wantWords {
			if !strings.Contains(out, w) {
				t.Errorf("the value %q was dropped rather than sanitised; a guard that ate the field would "+
					"pass every other check here:\n%q", w, out)
			}
		}
	}

	t.Run("app status --id: the rejection reason keeps its line breaks but not column zero", func(t *testing.T) {
		reason := escForge
		out := render(func(w *bytes.Buffer) {
			printSubmissionDetail(w, &appapi.Submission{
				BlockID: "my-app", Version: "1.0.0", Status: "rejected",
				RejectionReason: &reason,
				SubmittedAt:     "2026-09-01T10:00:00Z",
				LiveURL:         strPtr("https://example.civit.ai"),
			})
		})
		noRawEscape(t, out, "reason-head", "OVERWRITTEN", "FORGED-LINE")
		noColumnZero(t, out, "FORGED-LINE", "OVERWRITTEN")
		// The line break itself must SURVIVE — this is free text, and flattening it
		// would be the other failure mode (a reason is the only thing a rejected
		// author has to act on).
		if !strings.Contains(out, "\n  FORGED-LINE") {
			t.Errorf("the reason's own newline was flattened or left unindented; want a continuation indented "+
				"to the reason's own margin:\n%q", out)
		}
	})

	t.Run("app status --id: the approval notes get the same treatment", func(t *testing.T) {
		notes := escForge
		out := render(func(w *bytes.Buffer) {
			printSubmissionDetail(w, &appapi.Submission{
				BlockID: "my-app", Version: "1.0.0", Status: "approved",
				ApprovalNotes: &notes,
				SubmittedAt:   "2026-09-01T10:00:00Z",
				LiveURL:       strPtr("https://example.civit.ai"),
			})
		})
		noRawEscape(t, out, "reason-head", "OVERWRITTEN", "FORGED-LINE")
		noColumnZero(t, out, "FORGED-LINE", "OVERWRITTEN")
		if !strings.Contains(out, "\n  FORGED-LINE") {
			t.Errorf("the notes' newline was flattened or left unindented:\n%q", out)
		}
	})

	t.Run("app status --id: the live URL is single-line", func(t *testing.T) {
		live := forgeCell("lvurl")
		out := render(func(w *bytes.Buffer) {
			printSubmissionDetail(w, &appapi.Submission{
				BlockID: "my-app", Version: "1.0.0", Status: "approved",
				LiveURL:     &live,
				SubmittedAt: "2026-09-01T10:00:00Z",
			})
		})
		if !strings.Contains(out, forgeWant("lvurl")) {
			t.Errorf("the `Live at:` URL did not render as one inert string; want %q:\n%s", forgeWant("lvurl"), out)
		}
		noColumnZero(t, out, forgedRow)
	})

	t.Run("app status --id: the blockId in the not-live sentence is single-line", func(t *testing.T) {
		out := render(func(w *bytes.Buffer) {
			printSubmissionDetail(w, &appapi.Submission{
				BlockID: forgeCell("nlblock"), Version: "1.0.0", Status: "pending",
				SubmittedAt: "2026-09-01T10:00:00Z",
			})
		})
		var sentence string
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "Not live yet") {
				sentence = line
			}
		}
		if sentence == "" {
			t.Fatalf("CONTROL failure, not a finding: the `Not live yet` sentence did not render, so this case "+
				"asserts nothing:\n%s", out)
		}
		if !strings.Contains(sentence, forgeWant("nlblock")) {
			t.Errorf("the blockId in the `Not live yet` sentence is not one inert string; want %q:\n%q",
				forgeWant("nlblock"), sentence)
		}
		noColumnZero(t, out, forgedRow)
	})

	t.Run("app listing status: the screenshot id and caption are single-line", func(t *testing.T) {
		// 🔴 THE CAPTION PAYLOAD IS THE MEASURED ONE, NOT forgeCell. It forges the
		// label this very function writes through its tabwriter five lines above,
		// which is what makes the `App:` count below a live assertion rather than a
		// string that happens never to appear. The id carries the generic payload,
		// so the contiguity check still names which of the two leaked.
		caption := "GOOD\tTABBED\nApp:  forged-line"
		view := &appapi.ListingEditView{}
		view.Assets.Screenshots = []appapi.ListingScreenshot{{ID: forgeCell("lsid"), Caption: &caption}}
		out := render(func(w *bytes.Buffer) {
			printListingStatus(w, "my-app", &appapi.ListingRef{Status: "draft"}, view)
		})
		if !strings.Contains(out, forgeWant("lsid")) {
			t.Errorf("the screenshot ID did not render %q as one inert string:\n%s", forgeWant("lsid"), out)
		}
		if !strings.Contains(out, "GOOD TABBED App:  forged-line") {
			t.Errorf("the caption did not render as one inert line; want the flattened payload:\n%s", out)
		}
		// The precise #552 shape: the same function writes `App:` at column zero
		// through its tabwriter, so a caption carrying a newline forges a SECOND
		// such line, indistinguishable from the real row above it.
		//
		// 🔴 COUNT LINES THAT BEGIN WITH IT, NOT OCCURRENCES OF THE WORD. The
		// payload's own "App:" legitimately SURVIVES as inert mid-line text — the
		// assertion two lines up requires it to — so counting occurrences is red
		// against a correct fix. What may not happen is a second line STARTING
		// there.
		labelLines := 0
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "App:") {
				labelLines++
			}
		}
		if labelLines != 1 {
			t.Errorf("%d line(s) begin with the `App:` label, want 1 — a caption forged one at column zero:\n%s",
				labelLines, out)
		}
		noColumnZero(t, out, forgedRow, "App:  forged-line")
	})

	t.Run("generate --no-wait: the re-attach hint carries the same id as the receipt", func(t *testing.T) {
		var out, errb bytes.Buffer
		printSubmitResult(&out, &errb, &genapi.SubmitResult{
			ID: forgeCell("nwid"), Status: "processing",
		}, "ext_1", "https://civitai.com", true)
		// 🔴 ONE VALUE, ONE GATE. The receipt's cell used safeTermSingle and the
		// --no-wait hint three lines below used a bare safeTerm on the SAME r.ID,
		// so the id a user is told to paste into `civitai workflows get` could
		// carry a newline the receipt had already flattened.
		if !strings.Contains(errb.String(), forgeWant("nwid")) {
			t.Errorf("the --no-wait hint did not render the workflow id as one inert string; want %q:\n%s",
				forgeWant("nwid"), errb.String())
		}
		noColumnZero(t, errb.String(), forgedRow)
	})
}

// mustJSONString renders s as a JSON string literal for a fixture payload.
func mustJSONString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
