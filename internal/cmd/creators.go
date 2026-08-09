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
		Long: `Read-only access to Civitai creators through the public REST API
(GET /api/v1/creators).

` + readAnonNote + `

A creator row is a USERNAME, a published-model COUNT and a LINK. The group is
search-only: the public API has no per-creator profile route, so there is no
` + "`creators get`" + ` to add.

The follow-up is ` + "`civitai models search --username <name>`" + `, which is what
actually lists a creator's models. Use ` + "`civitai users get <name>`" + ` if you want
the user record (id, avatar) behind the name.

` + readJSONNote,
		Example: `  civitai creators search --query artist --limit 10
  civitai creators search --query artist --json`,
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
		Use:   "search",
		Short: "Search creators (GET /api/v1/creators)",
		Long: `Search creators via GET /api/v1/creators.

--query matches the username; omit it to page the whole creator list.
` + limitRule(creatorsLimitMax) + `.

Paging is --page only. This endpoint answers with the classic page envelope
(total items, current page, total pages) and no cursor, so there is no --cursor
here; the footer prints the next --page for you.

Each row is USERNAME, MODELS (that creator's published model count) and LINK
(the equivalent models query on the website). To list the models themselves,
run ` + "`civitai models search --username <name>`" + `.

` + readAnonShort,
		Example: `  civitai creators search --query artist --limit 10
  civitai creators search --limit 50 --page 2`,
		Args: cobra.NoArgs,
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
