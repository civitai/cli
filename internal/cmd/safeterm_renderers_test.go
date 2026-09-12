package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/civitai/cli/internal/appapi"
	civitai "github.com/civitai/cli/pkg/civitai"
)

// civitai/cli#399 — THE READ-PATH RENDERERS, DRIVEN WITH A HOSTILE FIXTURE.
//
// #399's measurement: deleting `safeTerm(...)` at 20 of 25 sampled sites left
// the whole suite green. Every one of those surfaces prints text a stranger
// uploaded — a model name, a tag, a username, an app tagline — and safeTerm is
// the only thing between it and the terminal. Its doc comment says what that
// buys: cursor moves and line-clears that overwrite the CLI's own
// `SHA256 verified` line, an OSC-52 clipboard set, a bidi control that makes
// the displayed order differ from the bytes.
//
// #399 asked for exactly this shape rather than 150 tests: "one table-driven
// test per renderer family that feeds a hostile string through the real
// renderer and asserts the paired predicate the reason-path tests already use".
//
// 🔴 THE PREDICATE IS PAIRED AND PER-FIELD, WHICH IS STRONGER THAN THE
// REASON-PATH TESTS IT COPIES. invisibleOrBidiRunes (safeterm_invisible_test.go)
// is the independently-built class check — `unicode.Cf` plus the measured U+2800
// counterexample, NOT saferune.Stripped, so it can disagree with the code under
// test. On its own it is satisfied by a renderer that prints nothing, so every
// case also names the fields it must show, and each field carries its OWN
// visible marker. A renderer that drops one column, or that strips the whole
// string instead of the class, fails on that field by name — a single shared
// "FIXTURE arrived" control cannot see either.

// classProbe is one rune from each half of the hazard: U+200B is INVISIBLE (a
// separator no wrapper splits on), U+202E REORDERS what is displayed, U+2800 is
// the blank-but-graphic residue the `Cf` category cut missed (#382), and
// U+001B is the ESC that starts every cursor move and line-clear. Putting all
// four inside one field means a renderer cannot pass by handling only the
// famous one.
//
// 🔴 THE ESC IS CAUGHT BY THE ADJACENCY HALF, NOT THE CLASS HALF, AND THAT IS
// DELIBERATE. invisibleOrBidiRunes is `unicode.Cf` plus U+2800 — U+001B is
// `Cc`, so it is invisible to that predicate. What catches it is wantStripped:
// safeTerm removes the ESC, so the two visible halves become adjacent, and a
// renderer that let the ESC through fails the per-field assertion by name. The
// class half alone could not see `\x1b[1A\x1b[2K` — the escape that overwrites
// the CLI's own `SHA256 verified` line — reaching the terminal, which is the
// concrete attack safeTerm's doc comment names.
//
// It is a BARE ESC on purpose: safeTerm strips the ESC and KEEPS the rest of a
// sequence (`a\x1b[2Kb` -> `a[2Kb`, pinned in safeterm_invisible_test.go), so a
// probe carrying `\x1b[2K` would leave `[2K` between the halves and break
// wantStripped even where the gate is working.
const classProbe = "\u200b\u202e\u2800\x1b"

// hostileField is a server-supplied value carrying the class BETWEEN two
// visible halves, so "the class is gone" and "the words arrived" are the same
// assertion: the halves are only adjacent if the runes between them were
// removed rather than the whole value dropped.
func hostileField(label string) string { return "FIXTURE-" + label + classProbe + "-tail" }

// wantStripped is what hostileField(label) must look like on screen.
func wantStripped(label string) string { return "FIXTURE-" + label + "-tail" }

func TestReadRenderersStripTheInvisibleClass(t *testing.T) {
	// cmdOut renders through a cobra command with a captured stdout, which is
	// how the `cmd *cobra.Command` renderers are reached.
	cmdOut := func(render func(c *cobra.Command)) string {
		c, out, _ := genCmd("")
		render(c)
		return out.String()
	}

	for _, tc := range []struct {
		// surface names the renderer, for the failure message.
		surface string
		// fields are the labels this renderer must put on screen. They are
		// pairwise distinct so a failure names the column that went missing.
		fields []string
		render func() string
	}{
		{
			surface: "`models search` table (printModelList)",
			fields:  []string{"mlname", "mltype", "mlcreator"},
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printModelList(c, []civitai.ModelListItem{{
						ID:      7,
						Name:    hostileField("mlname"),
						Type:    hostileField("mltype"),
						Creator: &civitai.Creator{Username: civitai.FlexString(hostileField("mlcreator"))},
					}})
				})
			},
		},
		{
			surface: "`models get` detail (printModelDetail, joinTags)",
			fields:  []string{"mdname", "mdtype", "mdcreator", "mdtag", "mdver", "mdbase"},
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printModelDetail(c, &civitai.ModelDetail{
						ID:      7,
						Name:    hostileField("mdname"),
						Type:    hostileField("mdtype"),
						Creator: &civitai.Creator{Username: civitai.FlexString(hostileField("mdcreator"))},
						Tags:    []string{hostileField("mdtag")},
						ModelVersions: []civitai.ModelVersionSummary{{
							ID:        9,
							Name:      hostileField("mdver"),
							BaseModel: hostileField("mdbase"),
						}},
					})
				})
			},
		},
		{
			surface: "`model-versions get` detail (printModelVersionDetail)",
			fields: []string{"mvname", "mvmodel", "mvmtype", "mvbase", "mvair",
				"mvtrigger", "mvurl", "mvfile", "mvftype"},
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printModelVersionDetail(c, &civitai.ModelVersionDetail{
						ID:           9,
						ModelID:      7,
						Name:         hostileField("mvname"),
						BaseModel:    hostileField("mvbase"),
						AIR:          hostileField("mvair"),
						DownloadURL:  hostileField("mvurl"),
						TrainedWords: []string{hostileField("mvtrigger")},
						Model: &civitai.ModelVersionModel{
							Name: hostileField("mvmodel"),
							Type: hostileField("mvmtype"),
						},
						Files: []civitai.ModelVersionFile{{
							Name: hostileField("mvfile"),
							Type: hostileField("mvftype"),
						}},
					})
				})
			},
		},
		{
			surface: "`users get` (printUser)",
			fields:  []string{"uname", "uimage"},
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printUser(c, civitai.UserItem{
						ID:       3,
						Username: civitai.FlexString(hostileField("uname")),
						Image:    hostileField("uimage"),
					})
				})
			},
		},
		{
			surface: "`tags search` table (printTagList)",
			fields:  []string{"tname", "tlink"},
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printTagList(c, []civitai.TagItem{{
						Name: hostileField("tname"),
						Link: hostileField("tlink"),
					}})
				})
			},
		},
		{
			surface: "`creators search` table (printCreatorList)",
			fields:  []string{"cname", "clink"},
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printCreatorList(c, []civitai.CreatorItem{{
						Username: civitai.FlexString(hostileField("cname")),
						Link:     hostileField("clink"),
					}})
				})
			},
		},
		{
			surface: "`collections search` table (printCollectionList)",
			fields:  []string{"clsname", "clstype", "clsowner"},
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printCollectionList(c, []civitai.CollectionListItem{{
						ID:   5,
						Name: hostileField("clsname"),
						Type: hostileField("clstype"),
						User: &civitai.CollectionUser{Username: civitai.FlexString(hostileField("clsowner"))},
					}})
				})
			},
		},
		{
			surface: "`collections get` detail (printCollectionDetail, joinCollectionTags)",
			fields:  []string{"cdname", "cdtype", "cdread", "cdabout", "cdowner", "cdtag"},
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printCollectionDetail(c, &civitai.CollectionDetail{
						ID:          5,
						Name:        hostileField("cdname"),
						Type:        hostileField("cdtype"),
						Read:        hostileField("cdread"),
						Description: hostileField("cdabout"),
						User:        &civitai.CollectionUser{Username: civitai.FlexString(hostileField("cdowner"))},
						Tags:        []civitai.CollectionTag{{Name: hostileField("cdtag")}},
					})
				})
			},
		},
		{
			surface: "`articles search` table (printArticleList)",
			fields:  []string{"altitle", "alauthor"},
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printArticleList(c, []civitai.ArticleListItem{{
						ID:          4,
						Title:       hostileField("altitle"),
						PublishedAt: "2026-08-05T12:00:00Z",
						User:        &civitai.ArticleUser{Username: civitai.FlexString(hostileField("alauthor"))},
					}})
				})
			},
		},
		{
			surface: "`articles get` detail (printArticleDetail, joinArticleTags)",
			fields:  []string{"adtitle", "adauthor", "adtag"},
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printArticleDetail(c, &civitai.ArticleDetail{
						ID:          4,
						Title:       hostileField("adtitle"),
						PublishedAt: "2026-08-05T12:00:00Z",
						User:        &civitai.ArticleUser{Username: civitai.FlexString(hostileField("adauthor"))},
						Tags:        []civitai.ArticleTag{{Name: hostileField("adtag")}},
					})
				})
			},
		},
		{
			surface: "`app list` table (printAppList, appCardAuthor)",
			fields:  []string{"alname", "alslug", "alkind", "alcat", "alauth"},
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printAppList(c, []civitai.AppCard{{
						Name:     hostileField("alname"),
						Slug:     hostileField("alslug"),
						Kind:     hostileField("alkind"),
						Category: hostileField("alcat"),
						Creator:  &civitai.ListingCreatorChip{Username: civitai.FlexString(hostileField("alauth"))},
					}})
				})
			},
		},
		{
			surface: "`app view` detail, offsite (printAppDetail)",
			fields: []string{"avname", "avslug", "avtag", "avkind", "avcat", "avrating",
				"avauth", "avsub", "avurl", "avclient", "avdesc"},
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printAppDetail(c, &civitai.AppDetail{
						Name:          hostileField("avname"),
						Slug:          hostileField("avslug"),
						Tagline:       hostileField("avtag"),
						Kind:          hostileField("avkind"),
						Category:      hostileField("avcat"),
						ContentRating: hostileField("avrating"),
						Description:   hostileField("avdesc"),
						Creator:       &civitai.ListingCreatorChip{Username: civitai.FlexString(hostileField("avauth"))},
						KindData: civitai.AppDetailKindData{
							Kind:            "offsite",
							SubKind:         hostileField("avsub"),
							ExternalURL:     hostileField("avurl"),
							ConnectClientID: hostileField("avclient"),
						},
					})
				})
			},
		},
		{
			// The onsite branch is a DIFFERENT arm of printAppDetail's switch, so
			// the offsite case above cannot see a missing safeTerm on liveUrl.
			surface: "`app view` detail, onsite live URL (printAppDetail)",
			fields:  []string{"onname", "onlive"},
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printAppDetail(c, &civitai.AppDetail{
						Name: hostileField("onname"),
						KindData: civitai.AppDetailKindData{
							Kind:    "onsite",
							LiveURL: hostileField("onlive"),
						},
					})
				})
			},
		},
		{
			surface: "the `app view` 404-but-owned advice (appViewOwnedAdvice)",
			fields:  []string{"avostatus"},
			render: func() string {
				return appViewOwnedAdvice("my-app", &appapi.Submission{
					Status: hostileField("avostatus"),
				})
			},
		},
		{
			surface: "`images search` table (printImageList)",
			fields:  []string{"ilname", "ilbase", "ilnsfw", "ilurl"},
			render: func() string {
				return cmdOut(func(c *cobra.Command) {
					printImageList(c, []civitai.ImageItem{{
						ID:        11,
						Username:  civitai.FlexString(hostileField("ilname")),
						BaseModel: hostileField("ilbase"),
						NSFWLevel: hostileField("ilnsfw"),
						URL:       hostileField("ilurl"),
					}})
				})
			},
		},
		{
			// 🔴 BOTH COLUMNS, BECAUSE THE LEDGER ROW CLAIMS BOTH. The row for
			// formatFileList says "name and type", and a fixture leaving Type
			// unset makes the type half VACUOUS: dashIfEmpty turns an empty Type
			// into "-", so deleting `safeTerm(dashIfEmpty(f.Type))` left the whole
			// internal/cmd suite green (measured). A second, distinct marker is
			// what makes the row's sentence and the assertion the same claim.
			surface: "the download plan's file list (formatFileList)",
			fields:  []string{"ffl", "ffltype"},
			render: func() string {
				return formatFileList([]civitai.ModelVersionFile{{
					Name: hostileField("ffl"),
					Type: hostileField("ffltype"),
				}})
			},
		},
		{
			// 🔴 THIS CASE IS WHY THE TEST EXISTS RATHER THAN A REVIEW. It is the
			// one surface in this table that was NOT already sanitised: the marker
			// interpolated the uploader's `files[].type` into a HEADER LINE raw,
			// while the identical field two lines below it in the same renderer
			// went through safeTerm. Nothing could see the difference until the
			// function was driven with a hostile value (civitai/cli#399).
			surface: "the non-weights primary-file marker (nonModelFileMarker)",
			fields:  []string{"nmfm"},
			render: func() string {
				return nonModelFileMarker([]civitai.ModelVersionFile{{
					Primary: true,
					Type:    hostileField("nmfm"),
				}})
			},
		},
	} {
		t.Run(tc.surface, func(t *testing.T) {
			got := tc.render()
			if bad := invisibleOrBidiRunes(got); len(bad) > 0 {
				t.Errorf("%s put %s on the terminal. safeTerm is the ONE gate on server-supplied text "+
					"reaching a terminal: an invisible rune is a separator no wrapper splits on, and a bidi "+
					"control changes the order the line is DISPLAYED in, so what the user reads is not what "+
					"the bytes say (civitai/cli#399, #393):\n%q", tc.surface, strings.Join(bad, ", "), got)
			}
			for _, f := range tc.fields {
				if strings.Contains(got, wantStripped(f)) {
					continue
				}
				t.Errorf("%s did not render %q as %q. Either the field stopped being printed — in which "+
					"case the class check above is VACUOUS for it — or the whole value was dropped instead "+
					"of just the class, which loses what the user asked to see:\n%q",
					tc.surface, f, wantStripped(f), got)
			}
		})
	}
}

// TestHostileFieldCarriesTheClass is the control ON THE FIXTURE.
//
// 🔴 EVERY ASSERTION ABOVE IS ABOUT BYTES THAT MUST BE PRESENT BEFORE THE
// RENDERER RUNS. If hostileField ever stopped carrying the class — a rune
// dropped from classProbe, an escape written where a literal was meant — every
// case would report "no invisible rune reached the terminal" and be measuring
// nothing. This is the one place that checks the input side.
func TestHostileFieldCarriesTheClass(t *testing.T) {
	in := hostileField("probe")
	bad := invisibleOrBidiRunes(in)
	if len(bad) != 3 {
		t.Fatalf("CONTROL failure, not a finding: the fixture carries %d rune(s) of the class (%v), want 3 — "+
			"the renderer cases assert on what safeTerm removes from it: %q", len(bad), bad, in)
	}
	// The ESC is the FOURTH probe rune and it is deliberately NOT one of the
	// three above: U+001B is `Cc`, so invisibleOrBidiRunes cannot see it and the
	// count stays 3. It is checked separately because nothing else would notice
	// it falling out of classProbe, and without it the adjacency half of every
	// case stops being able to see a renderer that lets `\x1b[1A\x1b[2K` through.
	if !strings.ContainsRune(in, 0x1b) {
		t.Fatalf("CONTROL failure, not a finding: the fixture carries no U+001B, so every case's per-field "+
			"assertion has stopped covering cursor-move and line-clear escapes: %q", in)
	}
	if got := safeTerm(in); got != wantStripped("probe") {
		t.Fatalf("CONTROL failure, not a finding: safeTerm(%q) = %q, want %q — the cases assert the halves "+
			"end up adjacent, so this is what they are asserting against", in, got, wantStripped("probe"))
	}
	// The label must be DISTINGUISHING, not just present: two fields sharing a
	// marker would let one satisfy the other's assertion, and the whole point of
	// per-field markers is that a dropped column is named.
	if hostileField("probe") == hostileField("other") {
		t.Fatal("CONTROL failure, not a finding: hostileField ignores its label, so every case's per-field " +
			"assertions collapse into one")
	}
}
