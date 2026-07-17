package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/civitai/cli/internal/api"
	"github.com/spf13/cobra"
)

const collectionsLimitMax = 100

func newCollectionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collections",
		Short: "Search and inspect collections on Civitai",
		Long: `Read-only access to Civitai collections via the public REST API
(GET /api/v1/collections). These endpoints are public and work anonymously; if
you are logged in (civitai login) your token is sent automatically. Only public
collections are discoverable.`,
		Example: `  civitai collections search --query "favorites" --limit 5
  civitai collections get 1234`,
	}
	cmd.AddCommand(newCollectionsSearchCmd())
	cmd.AddCommand(newCollectionsGetCmd())
	return cmd
}

func newCollectionsSearchCmd() *cobra.Command {
	var (
		query, sort, cursor string
		nsfw                bool
		limit               int
	)
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search collections (GET /api/v1/collections)",
		Long: `Search public collections via GET /api/v1/collections.

Pagination is cursor-based (a keyset cursor on the collection id — there is no
--page). The next cursor is printed after the results; pass it back via --cursor.
Cursor paging is only supported for the default (Newest) sort.`,
		Example: `  civitai collections search --query "anime" --limit 5
  civitai collections search --sort Newest --cursor <cursor>`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkLimit(limit, collectionsLimitMax); err != nil {
				return err
			}
			o := readFlags(cmd)
			q := url.Values{}
			addIfSet(q, "query", query)
			addIfSet(q, "sort", sort)
			addIfSet(q, "cursor", cursor)
			addIfPositive(q, "limit", limit)
			if cmd.Flags().Changed("nsfw") {
				q.Set("nsfw", strconv.FormatBool(nsfw))
			}

			client, _, err := newReader(o)
			if err != nil {
				return err
			}
			res, err := client.SearchCollections(context.Background(), q)
			if err != nil {
				return err
			}
			if o.json {
				return emitJSON(cmd, res.Raw)
			}
			printCollectionList(cmd, res.Items)
			printPageFooter(cmd, "civitai collections search", res.Metadata)
			return nil
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "text search query (matches the collection name)")
	cmd.Flags().StringVar(&sort, "sort", "", "sort order (Newest, \"Most Followers\")")
	cmd.Flags().BoolVar(&nsfw, "nsfw", false, "include NSFW results")
	cmd.Flags().IntVar(&limit, "limit", 0, "results per page (1-100)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor from a previous response (Newest sort only)")
	bindReadFlags(cmd)
	return cmd
}

func newCollectionsGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a collection by id (GET /api/v1/collections/{id})",
		Example: `  civitai collections get 1234
  civitai collections get 1234 --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o := readFlags(cmd)
			if n, err := strconv.Atoi(args[0]); err != nil || n <= 0 {
				return fmt.Errorf("collection id must be a positive integer, got %q", args[0])
			}
			client, _, err := newReader(o)
			if err != nil {
				return err
			}
			col, raw, err := client.GetCollection(context.Background(), args[0])
			if err != nil {
				return err
			}
			if o.json {
				return emitJSON(cmd, raw)
			}
			printCollectionDetail(cmd, col)
			return nil
		},
	}
	bindReadFlags(cmd)
	return cmd
}

func printCollectionList(cmd *cobra.Command, items []api.CollectionListItem) {
	out := cmd.OutOrStdout()
	if len(items) == 0 {
		fmt.Fprintln(out, "No collections found.")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tTYPE\tOWNER\tITEMS")
	for _, c := range items {
		owner := "-"
		if c.User != nil && c.User.Username != "" {
			owner = c.User.Username
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%d\n", c.ID, c.Name, dashIfEmpty(c.Type), owner, c.ItemCount)
	}
	_ = tw.Flush()
}

func printCollectionDetail(cmd *cobra.Command, c *api.CollectionDetail) {
	out := cmd.OutOrStdout()
	owner := "-"
	if c.User != nil && c.User.Username != "" {
		owner = c.User.Username
	}
	fmt.Fprintf(out, "%s (id %d)\n", c.Name, c.ID)
	fmt.Fprintf(out, "  owner:  %s\n", owner)
	fmt.Fprintf(out, "  type:   %s\n", dashIfEmpty(c.Type))
	fmt.Fprintf(out, "  read:   %s\n", dashIfEmpty(c.Read))
	fmt.Fprintf(out, "  public: %t\n", c.IsPublic)
	if c.Description != "" {
		fmt.Fprintf(out, "  about:  %s\n", truncate(c.Description, 200))
	}
	if len(c.Tags) > 0 {
		fmt.Fprintf(out, "  tags:   %s\n", joinCollectionTags(c.Tags))
	}
}

func joinCollectionTags(tags []api.CollectionTag) string {
	const max = 12
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.Name)
	}
	if len(names) <= max {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:max], ", ") + fmt.Sprintf(", … (+%d)", len(names)-max)
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
