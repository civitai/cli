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

const articlesLimitMax = 100

func newArticlesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "articles",
		Short: "Search and inspect articles on Civitai",
		Long: `Read-only access to Civitai articles through the public REST API
(GET /api/v1/articles, GET /api/v1/articles/{id}).

` + readAnonNote + `

Articles are the site's long-form guides. ` + "`articles search`" + ` finds them;
` + "`articles get <id> --content`" + ` renders the BODY as readable text/markdown —
headings, paragraphs, lists, links and code blocks, with the HTML stripped and
entities decoded — so a guide is readable in the terminal without a browser.

--json returns the raw API body, including the UNTOUCHED HTML content, and
takes precedence over --content.

` + readJSONNote,
		Example: `  civitai articles search --query "workflow" --limit 5
  civitai articles get 1234
  civitai articles get 1234 --content`,
	}
	cmd.AddCommand(newArticlesSearchCmd())
	cmd.AddCommand(newArticlesGetCmd())
	return cmd
}

func newArticlesSearchCmd() *cobra.Command {
	var (
		query, tags, username, sort, cursor string
		nsfw                                bool
		limit                               int
	)
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search articles (GET /api/v1/articles)",
		Long: `Search articles via GET /api/v1/articles.

Filters: --query (matches the TITLE, not the body), --tags, --username and
--nsfw. --tags takes numeric tag IDS, comma-separated (e.g. --tags 5,12), not
tag names — and ` + "`civitai tags search`" + ` renders names, not ids, so it cannot
supply this filter for you.

--sort takes a server-owned value set.
` + serverOwnedEnumNote + `

Paging is cursor-only: the article feed is a keyset feed, so there is no --page
here. ` + limitRule(articlesLimitMax) + `.
The next cursor is printed under the results; pass it back via --cursor.

The REACTIONS column is likes plus favourites, from the list endpoint's own
stats. ` + "`articles get <id>`" + ` reports all-time view / like / favourite /
comment / collected counts instead — a different stats block, so the two need
not agree — and --content renders the guide itself.

` + readAnonShort,
		Example: `  civitai articles search --query "comfyui" --limit 5
  civitai articles search --sort "Most Reactions" --nsfw
  civitai articles search --username some-creator --cursor '<cursor>'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkLimit(limit, articlesLimitMax); err != nil {
				return err
			}
			o := readFlags(cmd)
			q := url.Values{}
			addIfSet(q, "query", query)
			addIfSet(q, "tags", tags) // comma-delimited tag ids
			addIfSet(q, "username", username)
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
			res, err := client.SearchArticles(context.Background(), q)
			if err != nil {
				return err
			}
			if o.json {
				return emitJSON(cmd, res.Raw)
			}
			printArticleList(cmd, res.Items)
			printPageFooter(cmd, "civitai articles search", res.Metadata)
			return nil
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "text search query (matches the article title)")
	cmd.Flags().StringVar(&tags, "tags", "", "filter by tag ids (comma-separated, e.g. 5,12)")
	cmd.Flags().StringVar(&username, "username", "", "filter by author username")
	cmd.Flags().StringVar(&sort, "sort", "", "sort order (Newest, \"Recently Updated\", \"Most Reactions\", \"Most Comments\", \"Most Bookmarks\", \"Most Collected\")")
	cmd.Flags().BoolVar(&nsfw, "nsfw", false, "include NSFW results")
	cmd.Flags().IntVar(&limit, "limit", 0, limitFlagUsage(articlesLimitMax))
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor from a previous response")
	bindReadFlags(cmd)
	return cmd
}

func newArticlesGetCmd() *cobra.Command {
	var content bool
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get an article by id (GET /api/v1/articles/{id})",
		Long: `Get one article by id (GET /api/v1/articles/{id}).

By default it prints the article's metadata (title, author, stats, tags). Pass
--content to also render the article BODY — the actual guide — as readable plain
text / lightweight markdown (headings, paragraphs, lists, links, code blocks;
HTML tags stripped and entities decoded). --json returns the raw API body
(including the untouched HTML content) and takes precedence over --content.

` + readAnonShort,
		Example: `  civitai articles get 1234
  civitai articles get 1234 --content
  civitai articles get 1234 --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o := readFlags(cmd)
			if n, err := strconv.Atoi(args[0]); err != nil || n <= 0 {
				// A non-integer / non-positive positional arg is a client-side usage
				// mistake, not an API failure — tag it so the entrypoint maps it to
				// the usage exit code rather than the generic one.
				return asUsageError(fmt.Errorf("article id must be a positive integer, got %q", args[0]))
			}
			client, _, err := newReader(o)
			if err != nil {
				return err
			}
			a, raw, err := client.GetArticle(context.Background(), args[0])
			if err != nil {
				return err
			}
			if o.json {
				// --json wins: emit the raw body untouched (--content is ignored).
				return emitJSON(cmd, raw)
			}
			printArticleDetail(cmd, a)
			if content {
				printArticleContent(cmd, a)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&content, "content", false, "also render the article body as readable text/markdown (ignored with --json, which returns raw)")
	bindReadFlags(cmd)
	return cmd
}

// printArticleContent renders the article body below the metadata block. An
// empty body prints a clear note rather than nothing.
func printArticleContent(cmd *cobra.Command, a *civitai.ArticleDetail) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "\n─── content ───")
	text := htmlToText(a.Content)
	if strings.TrimSpace(text) == "" {
		fmt.Fprintln(out, "(this article has no rendered body — try --json for the raw payload)")
		return
	}
	fmt.Fprintln(out, text)
}

func printArticleList(cmd *cobra.Command, items []civitai.ArticleListItem) {
	out := cmd.OutOrStdout()
	if len(items) == 0 {
		fmt.Fprintln(out, "No articles found.")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTITLE\tAUTHOR\tPUBLISHED\tREACTIONS\tCOMMENTS")
	for _, a := range items {
		author := "-"
		if a.User != nil && a.User.Username != "" {
			author = safeTerm(a.User.Username)
		}
		title := safeTerm(a.Title)
		if a.NSFWLevel > 1 {
			title += " [nsfw]"
		}
		reactions := a.Stats.LikeCount + a.Stats.FavoriteCount
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%d\t%d\n", a.ID, title, author, dashIfEmpty(safeTerm(shortDate(a.PublishedAt))), reactions, a.Stats.CommentCount)
	}
	_ = tw.Flush()
}

func printArticleDetail(cmd *cobra.Command, a *civitai.ArticleDetail) {
	out := cmd.OutOrStdout()
	author := "-"
	if a.User != nil && a.User.Username != "" {
		author = safeTerm(a.User.Username)
	}
	fmt.Fprintf(out, "%s (id %d)\n", safeTerm(a.Title), a.ID)
	fmt.Fprintf(out, "  author:    %s\n", author)
	fmt.Fprintf(out, "  published: %s\n", dashIfEmpty(safeTerm(shortDate(a.PublishedAt))))
	fmt.Fprintf(out, "  nsfwLevel: %d\n", a.NSFWLevel)
	if a.Stats != nil {
		fmt.Fprintf(out, "  views: %d   likes: %d   favorites: %d   comments: %d   collected: %d\n",
			a.Stats.ViewCountAllTime, a.Stats.LikeCountAllTime, a.Stats.FavoriteCountAllTime,
			a.Stats.CommentCountAllTime, a.Stats.CollectedCountAllTime)
	}
	if len(a.Tags) > 0 {
		fmt.Fprintf(out, "  tags:      %s\n", joinArticleTags(a.Tags))
	}
}

func joinArticleTags(tags []civitai.ArticleTag) string {
	const max = 12
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, safeTerm(t.Name))
	}
	if len(names) <= max {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:max], ", ") + fmt.Sprintf(", … (+%d)", len(names)-max)
}
