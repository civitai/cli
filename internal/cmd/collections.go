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

const collectionsLimitMax = 100

func newCollectionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collections",
		Short: "Search and inspect collections on Civitai",
		Long: `Read-only access to Civitai collections through the public REST API
(GET /api/v1/collections, GET /api/v1/collections/{id}).

` + readAnonNote + `

Only PUBLIC collections are discoverable. Logging in does not widen this
surface to your own private collections, and there is no create, edit or
add-to-collection path in the CLI — this group is read-only.

` + "`collections search`" + ` finds collections by name; ` + "`collections get <id>`" + `
shows one collection's owner, type, read permission, description and tags.

Neither command pages through a collection's CONTENTS: the public route
answers with collection metadata (` + "`search`" + ` adds an item COUNT), not with the
models or images inside.

` + readJSONNote,
		Example: `  civitai collections search --query "favorites" --limit 5
  civitai collections get 1234
  civitai collections get 1234 --json`,
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

Paging is cursor-only: a keyset cursor on the collection id, so there is
no --page here. ` + limitRule(collectionsLimitMax) + `.
The next cursor is printed under the results; pass it back via --cursor.

Cursor paging is only supported for the default (Newest) sort. This is a server
constraint: for any other --sort (e.g. "Most Followers") the API returns a
nextCursor that it then rejects — a dead cursor that yields no further pages. So
for a non-Newest sort the CLI shows the first page only and does NOT print a
next-page hint; deep paging requires --sort Newest.

` + readAnonShort,
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

			// The collections endpoint only supports cursor pagination for the
			// default (Newest) sort. For any other sort the server still returns a
			// nextCursor, but feeding it back is rejected — a dead cursor. When the
			// requested sort can't cursor-paginate yet the server handed us a
			// cursor, emit a one-line note on STDERR (so --json stdout stays pure)
			// and DON'T print the misleading next-page hint. Exit stays 0.
			pageable := collectionsSortIsCursorPageable(sort)
			if !pageable && res.Metadata.CursorString() != "" {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"note: cursor pagination isn't available for --sort %q (API limitation); "+
						"showing the first page only — use --sort Newest for deep paging.\n", sort)
			}

			if o.json {
				return emitJSON(cmd, res.Raw)
			}
			printCollectionList(cmd, res.Items)
			if pageable {
				printPageFooter(cmd, "civitai collections search", res.Metadata)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "text search query (matches the collection name)")
	cmd.Flags().StringVar(&sort, "sort", "", "sort order (Newest, \"Most Followers\")")
	cmd.Flags().BoolVar(&nsfw, "nsfw", false, "include NSFW results")
	cmd.Flags().IntVar(&limit, "limit", 0, limitFlagUsage(collectionsLimitMax))
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor from a previous response (Newest sort only)")
	bindReadFlags(cmd)
	return cmd
}

func newCollectionsGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a collection by id (GET /api/v1/collections/{id})",
		Long: `Get one collection by id: GET /api/v1/collections/{id}.

The id is the number in a civitai.com/collections/<id> URL. A non-integer or
non-positive argument is refused locally, as a usage mistake, before any
request is made.

Prints the collection's name, owner, type, read permission, public flag,
description (truncated) and tags. Only PUBLIC collections are readable here.

Two differences from the ` + "`search`" + ` row for the same collection, both of them
the API's shape rather than the CLI's: the detail body drops the item COUNT and
adds the TAGS. Neither shape lists the collection's items.

` + readAnonShort,
		Example: `  civitai collections get 1234
  civitai collections get 1234 --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o := readFlags(cmd)
			if n, err := strconv.Atoi(args[0]); err != nil || n <= 0 {
				// A non-integer / non-positive positional arg is a client-side usage
				// mistake, not an API failure — tag it so the entrypoint maps it to
				// the usage exit code rather than the generic one.
				return asUsageError(fmt.Errorf("collection id must be a positive integer, got %q", args[0]))
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

// collectionsSortIsCursorPageable reports whether the Civitai collections
// endpoint (GET /api/v1/collections) supports cursor pagination for the given
// --sort value. Per the API, cursor paging works ONLY for the default (Newest)
// sort; every other accepted sort (currently just "Most Followers") returns a
// nextCursor that the server then rejects, so the CLI must not advertise a
// next-page hint for it. An empty sort means "use the server default", which is
// Newest — hence pageable. Deriving the rule from the documented "cursor =
// Newest only" constraint (rather than a denylist) means a newly added
// non-Newest sort is correctly treated as non-pageable by default.
func collectionsSortIsCursorPageable(sort string) bool {
	return sort == "" || sort == "Newest"
}

func printCollectionList(cmd *cobra.Command, items []civitai.CollectionListItem) {
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
			owner = safeTerm(c.User.Username)
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%d\n", c.ID, safeTerm(c.Name), dashIfEmpty(safeTerm(c.Type)), owner, c.ItemCount)
	}
	_ = tw.Flush()
}

func printCollectionDetail(cmd *cobra.Command, c *civitai.CollectionDetail) {
	out := cmd.OutOrStdout()
	owner := "-"
	if c.User != nil && c.User.Username != "" {
		owner = safeTerm(c.User.Username)
	}
	fmt.Fprintf(out, "%s (id %d)\n", safeTerm(c.Name), c.ID)
	fmt.Fprintf(out, "  owner:  %s\n", owner)
	fmt.Fprintf(out, "  type:   %s\n", dashIfEmpty(safeTerm(c.Type)))
	fmt.Fprintf(out, "  read:   %s\n", dashIfEmpty(safeTerm(c.Read)))
	fmt.Fprintf(out, "  public: %t\n", c.IsPublic)
	if c.Description != "" {
		fmt.Fprintf(out, "  about:  %s\n", safeTerm(truncate(c.Description, 200)))
	}
	if len(c.Tags) > 0 {
		fmt.Fprintf(out, "  tags:   %s\n", joinCollectionTags(c.Tags))
	}
}

func joinCollectionTags(tags []civitai.CollectionTag) string {
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

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// truncate shortens s to at most max RUNES, appending an ellipsis.
//
// 🔴 The cut is on RUNE boundaries, not bytes. `s[:max]` sliced bytes, so a
// truncation landing inside a multi-byte rune emitted that rune's leading bytes
// alone — rendered as U+FFFD in every terminal. Taglines, descriptions and base
// model names are user-supplied and routinely non-ASCII, so this was reachable
// output corruption rather than a theoretical one. The cheap byte-length check
// stays first: it is exact for ASCII and a safe lower bound otherwise (a string
// whose BYTE length fits can never need cutting), so the walk below only runs on
// the values that might.
//
// 🔴 The walk replaced a second `if len([]rune(s)) <= max` guard, which was
// UNREACHABLE-ISH in the tests that existed for it: every case either failed the
// byte check outright or was far under `max`, so disabling that branch left the
// package green while `truncate("café", 4)` returned `"café…"`. A single pass
// closes the window structurally — a string with at most `max` runes falls out
// of the loop and is returned whole — and it drops the 4×len allocation
// `[]rune(s)` made on every server-supplied description.
//
// Residual, stated so no reader over-reads it: this is rune-safe, NOT
// grapheme-safe. Cutting a ZWJ emoji sequence mid-cluster yields valid UTF-8
// with a dangling joiner (`"👩‍👩‍👧"` + `"…"`), which renders oddly but emits no
// U+FFFD. The callers are terminal columns for names, taglines, descriptions and
// base models; a grapheme segmenter is a dependency this repo does not carry for
// that. Do not add a comment claiming more than the loop does.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	n := 0
	for i := range s { // i is the BYTE index of each rune start
		if n == max {
			return s[:i] + "…"
		}
		n++
	}
	return s
}
