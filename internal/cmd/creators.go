package cmd

import (
	"context"
	"fmt"
	"net/url"
	"text/tabwriter"

	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

const creatorsLimitMax = 200

func newCreatorsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "creators",
		Short: "Search creators on Civitai",
		Long: `Read-only access to Civitai creators via the public REST API
(GET /api/v1/creators). Works anonymously.`,
		Example: `  civitai creators search --query artist --limit 10`,
	}
	cmd.AddCommand(newCreatorsSearchCmd())
	return cmd
}

func newCreatorsSearchCmd() *cobra.Command {
	var (
		query       string
		limit, page int
	)
	cmd := &cobra.Command{
		Use:     "search",
		Short:   "Search creators (GET /api/v1/creators)",
		Example: `  civitai creators search --query artist --limit 10`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkLimit(limit, creatorsLimitMax); err != nil {
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
			res, err := client.SearchCreators(context.Background(), q)
			if err != nil {
				return err
			}
			if o.json {
				return emitJSON(cmd, res.Raw)
			}
			printCreatorList(cmd, res.Items)
			printPageFooter(cmd, "civitai creators search", res.Metadata)
			return nil
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "text search query")
	cmd.Flags().IntVar(&limit, "limit", 0, "results per page")
	cmd.Flags().IntVar(&page, "page", 0, "page number")
	bindReadFlags(cmd)
	return cmd
}

func printCreatorList(cmd *cobra.Command, items []civitai.CreatorItem) {
	out := cmd.OutOrStdout()
	if len(items) == 0 {
		fmt.Fprintln(out, "No creators found.")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "USERNAME\tMODELS\tLINK")
	for _, c := range items {
		fmt.Fprintf(tw, "%s\t%d\t%s\n", orDash(safeTerm(c.Username)), c.ModelCount, safeTerm(c.Link))
	}
	_ = tw.Flush()
}
