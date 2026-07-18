package cmd

import (
	"context"
	"fmt"
	"net/url"
	"text/tabwriter"

	"github.com/civitai/cli/internal/api"
	"github.com/spf13/cobra"
)

const tagsLimitMax = 200

func newTagsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tags",
		Short: "Search model tags on Civitai",
		Long: `Read-only access to Civitai model tags via the public REST API
(GET /api/v1/tags). Works anonymously.`,
		Example: `  civitai tags search --query anime --limit 10`,
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
		Use:     "search",
		Short:   "Search tags (GET /api/v1/tags)",
		Example: `  civitai tags search --query anime --limit 10`,
		Args:    cobra.NoArgs,
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

func printTagList(cmd *cobra.Command, items []api.TagItem) {
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
