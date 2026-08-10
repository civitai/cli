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

func newUsersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Look up users on Civitai",
		Long: `Read-only access to Civitai users through the public REST API.

` + readAnonNote + `

NOTE: the only public users route is the SEARCH endpoint GET /api/v1/users,
keyed by ?query= or ?ids=. The per-id route /api/v1/users/{userId} is an
INTERNAL webhook (POST plus a system token) and is not usable from the CLI, so
` + "`users get`" + ` resolves a user through that public search.

That is also why there is no ` + "`users search`" + `: the search endpoint is already
what ` + "`users get`" + ` calls, and it answers with a handful of fuzzy neighbours
rather than a browsable, pageable list — it has no pagination envelope at all.

What comes back is identity only: id, username and avatar URL. For a user's
models use ` + "`civitai models search --username <name>`" + `; for their published
model COUNT use ` + "`civitai creators search --query <name>`" + `.

` + readJSONNote,
		Example: `  civitai users get some-username
  civitai users get 5
  civitai users get 5 --json`,
	}
	cmd.AddCommand(newUsersGetCmd())
	return cmd
}

func newUsersGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <username-or-id>",
		Short: "Look up a user by username or id (public search: GET /api/v1/users)",
		Long: `Look up a user by username or numeric id through the public user search
(GET /api/v1/users). A numeric argument is sent as ?ids= and returns exactly
that user; anything else is sent as ?query=.

A NAME lookup is FUZZY on the server side — the endpoint answers with the
closest-matching users, not with your user. So the CLI requires an exact
(case-insensitive) username match before it prints anybody: a typo lists the
near misses and fails as not-found rather than confidently printing the wrong
person. When several users match, the exact one is printed and the rest are
listed under "other matches". Pass the numeric id when you need certainty.

An unknown user comes back from this endpoint as an empty HTTP 200 rather than
a 404. The CLI still reports it as NOT FOUND, so a script gets the same signal
it would from a real 404.

The result is identity only — id, username, avatar URL. There are no stats and
no model list on this route.

` + readAnonShort,
		Example: `  civitai users get some-username
  civitai users get 5 --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o := readFlags(cmd)
			arg := args[0]
			q := url.Values{}
			numeric := false
			if _, err := strconv.Atoi(arg); err == nil {
				q.Set("ids", arg)
				numeric = true
			} else {
				q.Set("query", arg)
			}

			client, _, err := newReader(o)
			if err != nil {
				return err
			}
			res, err := client.SearchUsers(context.Background(), q)
			if err != nil {
				return err
			}
			if o.json {
				return emitJSON(cmd, res.Raw)
			}
			if len(res.Items) == 0 {
				// "users get" resolves through the public user SEARCH (the per-id
				// route is an internal webhook), so a missing user comes back as an
				// empty 200, not an HTTP 404. Tag it as not-found so a scripted
				// lookup still gets the not-found exit code, matching a real 404.
				return civitai.Tag(civitai.ErrNotFound, fmt.Errorf("no user found for %q", arg))
			}
			// A numeric id lookup returns exactly the requested user. A NAME query
			// hits the search endpoint, which returns ≤5 FUZZY neighbours — so we
			// require an exact (case-insensitive) username match and refuse to
			// present a fuzzy top-hit as "the" user. Otherwise a typo'd or
			// non-existent name (which still returns plausible neighbours) would
			// confidently print the WRONG person.
			match := res.Items[0]
			if !numeric {
				found := false
				for _, u := range res.Items {
					if strings.EqualFold(u.Username, arg) {
						match, found = u, true
						break
					}
				}
				if !found {
					names := make([]string, 0, len(res.Items))
					for _, u := range res.Items {
						names = append(names, orDash(safeTerm(u.Username)))
					}
					return civitai.Tag(civitai.ErrNotFound, fmt.Errorf("no user found with exact username %q; closest matches: %s (use the numeric id for an exact lookup)", arg, strings.Join(names, ", ")))
				}
			}
			printUser(cmd, match)
			if !numeric && len(res.Items) > 1 {
				fmt.Fprintf(cmd.OutOrStdout(), "\nother matches:\n")
				tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
				for _, u := range res.Items {
					if u.ID == match.ID {
						continue
					}
					fmt.Fprintf(tw, "  %d\t%s\n", u.ID, orDash(safeTerm(u.Username)))
				}
				_ = tw.Flush()
			}
			return nil
		},
	}
	bindReadFlags(cmd)
	return cmd
}

func printUser(cmd *cobra.Command, u civitai.UserItem) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%s (id %d)\n", orDash(safeTerm(u.Username)), u.ID)
	if u.Image != "" {
		fmt.Fprintf(out, "  image: %s\n", safeTerm(u.Image))
	}
}
