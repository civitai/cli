package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

const modelsLimitMax = 100

func newModelsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "models",
		Short: "Search and inspect models on Civitai",
		Long: `Read-only access to Civitai models through the public REST API
(GET /api/v1/models, GET /api/v1/models/{id}).

` + readAnonNote + `

` + "`models search`" + ` is the discovery surface — filter by --query / --tag /
--username / --type / --base-model, order with --sort / --period, and page with
--limit / --page / --cursor. ` + "`models get <id>`" + ` returns one model with its
full version list.

A search hit already embeds .modelVersions[] — every version's files, hashes
and trained words — so iterating search results rarely needs a follow-up
` + "`model-versions get`" + ` per version.

What lives one level down: a model is the PAGE, a model VERSION is the
downloadable unit. ` + "`civitai download`" + ` and ` + "`civitai generate --checkpoint`" + `
both take a version id, which ` + "`models get`" + ` lists.

` + readJSONNote,
		Example: `  civitai models search --query "pony" --limit 5
  civitai models search --type LORA --base-model Illustrious --limit 20
  civitai models get 4384
  civitai models get 4384 --json`,
	}
	cmd.AddCommand(newModelsSearchCmd())
	cmd.AddCommand(newModelsGetCmd())
	return cmd
}

func newModelsSearchCmd() *cobra.Command {
	var (
		query, tag, username, typ, sort, period, cursor string
		baseModels                                      []string
		nsfw                                            bool
		limit, page                                     int
	)
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search models (GET /api/v1/models)",
		Long: `Search models via GET /api/v1/models.

Filters: --query (free text), --tag, --username, --type, --base-model
(repeatable — the API ORs the given values) and --nsfw.
` + serverOwnedEnumNote + `
--base-model in particular is matched LITERALLY, so a misspelling returns zero
results rather than an error; the CLI prints a stderr note when that happens.

Paging: ` + limitRule(modelsLimitMax) + `.
--page is shallow paging, --cursor is deep paging, and the next cursor is
printed under the results. The API caps page × limit at 1000 and answers 429
past it — the CLI recognises that deep-paging cap and reports it as a usage
mistake rather than as a rate limit, so a retry loop does not spin on it.

--period changes the SORT, not the DOWNLOADS column: the API returns only the
all-time download count, so a later row can legitimately show more downloads
than an earlier one. The column is labelled DL(all-time) for that reason.

` + readAnonShort,
		Example: `  civitai models search --query "pony" --limit 5
  civitai models search --type LORA --sort "Most Downloaded" --period Month
  civitai models search --base-model Pony --base-model Illustrious --limit 20
  civitai models search --username some-creator --cursor '<cursor>'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkLimit(limit, modelsLimitMax); err != nil {
				return err
			}
			o := readFlags(cmd)
			q := url.Values{}
			addIfSet(q, "query", query)
			addIfSet(q, "tag", tag)
			addIfSet(q, "username", username)
			addIfSet(q, "types", typ) // API query param is `types`
			// baseModels is a repeatable filter → repeated `baseModels=` query
			// keys, which the API's zod union (string | string[]) parses as an
			// array (baseModel IN (...), i.e. OR across the given values).
			for _, bm := range baseModels {
				if bm != "" {
					q.Add("baseModels", bm)
				}
			}
			// The API sorts by the given --period but only ever returns the
			// all-time stats.downloadCount (there is no per-period download
			// total), so the DOWNLOADS column can show a lower count on an
			// earlier row — which reads as a broken sort. Warn so the number
			// isn't misread as the period's count.
			if cmd.Flags().Changed("period") {
				fmt.Fprintln(cmd.ErrOrStderr(),
					"note: the DL(all-time) column is the all-time download count; the API does not return per-period download totals, so it does not reflect --period.")
			}
			addIfSet(q, "sort", sort)
			addIfSet(q, "period", period)
			addIfSet(q, "cursor", cursor)
			addIfPositive(q, "limit", limit)
			addIfPositive(q, "page", page)
			if cmd.Flags().Changed("nsfw") {
				q.Set("nsfw", strconv.FormatBool(nsfw))
			}

			client, _, err := newReader(o)
			if err != nil {
				return err
			}
			res, err := client.SearchModels(context.Background(), q)
			if err != nil {
				return err
			}
			// --base-model is matched literally by the API, so a typo silently
			// yields zero results rather than an error. Nudge the user toward the
			// likely cause. Empty results are not a failure, so exit stays 0.
			if cmd.Flags().Changed("base-model") && len(res.Items) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(),
					"note: 0 results — check --base-model spelling (it's matched literally).")
			}
			if o.json {
				return emitJSON(cmd, res.Raw)
			}
			printModelList(cmd, res.Items)
			printPageFooter(cmd, "civitai models search", res.Metadata)
			return nil
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "text search query")
	cmd.Flags().StringVar(&tag, "tag", "", "filter by tag name")
	cmd.Flags().StringVar(&username, "username", "", "filter by creator username")
	cmd.Flags().StringVar(&typ, "type", "", "filter by model type (e.g. Checkpoint, LORA, TextualInversion)")
	cmd.Flags().StringSliceVar(&baseModels, "base-model", nil, "filter by base model; repeatable (e.g. --base-model Pony --base-model \"Illustrious\"). Distinguishes video checkpoints (\"Wan Video 2.2 T2V-A14B\") that all share --type Checkpoint")
	cmd.Flags().StringVar(&sort, "sort", "", "sort order (e.g. \"Highest Rated\", \"Most Downloaded\", Newest)")
	cmd.Flags().StringVar(&period, "period", "", "time period (AllTime, Year, Month, Week, Day)")
	cmd.Flags().BoolVar(&nsfw, "nsfw", false, "include NSFW results")
	cmd.Flags().IntVar(&limit, "limit", 0, "results per page (1-100)")
	cmd.Flags().IntVar(&page, "page", 0, "page number (shallow paging; prefer --cursor for deep paging)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor from a previous response")
	bindReadFlags(cmd)
	return cmd
}

func newModelsGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a model by id (GET /api/v1/models/{id})",
		Long: `Get one model by id: GET /api/v1/models/{id}.

The id is the number in a civitai.com/models/<id> URL. A non-integer argument
is refused locally, as a usage mistake, before any request is made.

The human output lists every published VERSION (id, name, base model) — that
version id is what ` + "`civitai download --version`" + ` and
` + "`civitai generate --checkpoint`" + ` take. A version whose primary file is not
model weights is tagged with its actual file type ([Archive], [Training Data],
[Other]); it still downloads, the tag just says it is not a .safetensors.

--json carries much more than the human view — the description HTML, per-file
hashes and download URLs, and the full stats block.

` + readAnonShort,
		Example: `  civitai models get 4384
  civitai models get 4384 --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o := readFlags(cmd)
			if _, err := strconv.Atoi(args[0]); err != nil {
				// A non-integer positional arg is a client-side usage mistake, not
				// an API failure — tag it so the entrypoint maps it to the usage
				// exit code rather than the generic one.
				return asUsageError(fmt.Errorf("model id must be a positive integer, got %q", args[0]))
			}
			client, _, err := newReader(o)
			if err != nil {
				return err
			}
			m, raw, err := client.GetModel(context.Background(), args[0])
			if err != nil {
				return err
			}
			if o.json {
				return emitJSON(cmd, raw)
			}
			printModelDetail(cmd, m)
			return nil
		},
	}
	bindReadFlags(cmd)
	return cmd
}

func printModelList(cmd *cobra.Command, items []civitai.ModelListItem) {
	out := cmd.OutOrStdout()
	if len(items) == 0 {
		fmt.Fprintln(out, "No models found.")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	// The API returns only the all-time stats.downloadCount — there is no
	// per-period download total — so label the column all-time to avoid the
	// "row 2 has more downloads than row 1" read-as-broken-sort confusion when
	// results are sorted by --period (e.g. --sort "Most Downloaded" --period Month).
	fmt.Fprintln(tw, "ID\tNAME\tTYPE\tCREATOR\tDL(all-time)\tTHUMBSUP")
	for _, m := range items {
		creator := "-"
		if m.Creator != nil && m.Creator.Username != "" {
			creator = safeTerm(m.Creator.Username)
		}
		name := safeTerm(m.Name)
		if m.NSFW {
			name += " [nsfw]"
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%d\t%d\n", m.ID, name, safeTerm(m.Type), creator, m.Stats.DownloadCount, m.Stats.ThumbsUpCount)
	}
	_ = tw.Flush()
}

func printModelDetail(cmd *cobra.Command, m *civitai.ModelDetail) {
	out := cmd.OutOrStdout()
	creator := "-"
	if m.Creator != nil && m.Creator.Username != "" {
		creator = safeTerm(m.Creator.Username)
	}
	fmt.Fprintf(out, "%s (id %d)\n", safeTerm(m.Name), m.ID)
	fmt.Fprintf(out, "  type:      %s\n", safeTerm(m.Type))
	fmt.Fprintf(out, "  creator:   %s\n", creator)
	fmt.Fprintf(out, "  nsfw:      %t\n", m.NSFW)
	fmt.Fprintf(out, "  downloads: %d   thumbsUp: %d   comments: %d\n", m.Stats.DownloadCount, m.Stats.ThumbsUpCount, m.Stats.CommentCount)
	if len(m.Tags) > 0 {
		fmt.Fprintf(out, "  tags:      %s\n", joinTags(m.Tags))
	}
	fmt.Fprintf(out, "  versions (%d):\n", len(m.ModelVersions))
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	for _, v := range m.ModelVersions {
		marker := nonModelFileMarker(v.Files)
		if marker != "" {
			marker = "  " + marker
		}
		fmt.Fprintf(tw, "    %d\t%s\t%s%s\n", v.ID, safeTerm(v.Name), safeTerm(v.BaseModel), marker)
	}
	_ = tw.Flush()
}

// joinTags renders a comma-separated tag / trained-word list, capped at 12, with
// each element sanitized (safeTerm) since tags and trigger words are
// server-controlled and printed to the terminal.
func joinTags(tags []string) string {
	const max = 12
	shown := tags
	overflow := 0
	if len(tags) > max {
		shown = tags[:max]
		overflow = len(tags) - max
	}
	parts := make([]string, len(shown))
	for i, t := range shown {
		parts[i] = safeTerm(t)
	}
	s := strings.Join(parts, ", ")
	if overflow > 0 {
		s += fmt.Sprintf(", … (+%d)", overflow)
	}
	return s
}
