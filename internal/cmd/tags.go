package cmd

import (
	"context"
	"fmt"
	"net/url"
	"text/tabwriter"

	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

const tagsLimitMax = 200

func newTagsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tags",
		Short: "Search model tags on Civitai",
		Long: `Read-only access to Civitai MODEL tags through the public REST API
(GET /api/v1/tags).

` + readAnonNote + `

These are the model taxonomy — the same names ` + "`civitai models search --tag`" + `
filters on, which is what this group is for: find the tag, then search with it.

The group is search-only, and deliberately so: the public route answers with a
tag NAME and a LINK per tag and nothing else, so there is nothing for a
` + "`tags get`" + ` to fetch.

Not to be confused with ` + "`civitai articles search --tags`" + `, which takes numeric
tag IDS rather than these names — a different filter on a different endpoint.

` + readJSONNote,
		Example: `  civitai tags search --query anime --limit 10
  civitai tags search --query anime --json`,
	}
	cmd.AddCommand(newTagsSearchCmd())
	return cmd
}

func newTagsSearchCmd() *cobra.Command {
	var (
		query       string
		limit, page int
	)
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search tags (GET /api/v1/tags)",
		Long: `Search model tags via GET /api/v1/tags.

--query matches the tag name; omit it to page the whole tag list.
` + limitRule(tagsLimitMax) + `.

Paging is --page only. This endpoint answers with the classic page envelope
(total items, current page, total pages) and no cursor, so there is no --cursor
here; the footer prints the next --page for you.

Each row is a tag NAME and a LINK. The name is what
` + "`civitai models search --tag <name>`" + ` takes — that is the follow-up this
command exists to set up. The link is the equivalent models query on the
website.

` + readAnonShort,
		Example: `  civitai tags search --query anime --limit 10
  civitai tags search --limit 50 --page 2`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkLimit(limit, tagsLimitMax); err != nil {
				return err
			}
			o := readFlags(cmd)
			q := url.Values{}
			addIfSet(q, "query", query)
			addIfPositive(q, "limit", limit)
			addIfPositive(q, "page", page)

			client, _, err := newReader(o)
			if err != nil {
				return err
			}
			res, err := client.SearchTags(context.Background(), q)
			if err != nil {
				return err
			}
			if o.json {
				return emitJSON(cmd, res.Raw)
			}
			printTagList(cmd, res.Items)
			printPageFooter(cmd, "civitai tags search", res.Metadata)
			return nil
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "text search query")
	cmd.Flags().IntVar(&limit, "limit", 0, "results per page")
	cmd.Flags().IntVar(&page, "page", 0, "page number")
	bindReadFlags(cmd)
	return cmd
}

func printTagList(cmd *cobra.Command, items []civitai.TagItem) {
	out := cmd.OutOrStdout()
	if len(items) == 0 {
		fmt.Fprintln(out, "No tags found.")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tLINK")
	for _, t := range items {
		fmt.Fprintf(tw, "%s\t%s\n", safeTerm(t.Name), safeTerm(t.Link))
	}
	_ = tw.Flush()
}
