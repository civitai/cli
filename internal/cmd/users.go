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

func newUsersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Look up users on Civitai",
		Long: `Read-only access to Civitai users via the public REST API.

NOTE: the public users route is the search endpoint GET /api/v1/users (keyed by
?query= or ?ids=). The per-id route /api/v1/users/{userId} is an INTERNAL
webhook (POST + system token) and is NOT usable by the CLI, so "users get"
resolves a user through the public search. Works anonymously.`,
		Example: `  civitai users get some-username
  civitai users get 5`,
	}
	cmd.AddCommand(newUsersGetCmd())
	return cmd
}

func newUsersGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <username-or-id>",
		Short: "Look up a user by username or id (public search: GET /api/v1/users)",
		Long: `Look up a user by username or numeric id via the public user search
(GET /api/v1/users). A numeric argument is matched via ?ids=; anything else via
?query= (returning the best-matching users, from which an exact username match
is selected when present).`,
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
				return fmt.Errorf("no user found for %q", arg)
			}
			// Prefer an exact username match for a name query; otherwise the first
			// result (a numeric id lookup returns exactly the requested user).
			match := res.Items[0]
			if !numeric {
				for _, u := range res.Items {
					if strings.EqualFold(u.Username, arg) {
						match = u
						break
					}
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
					fmt.Fprintf(tw, "  %d\t%s\n", u.ID, orDash(u.Username))
				}
				_ = tw.Flush()
			}
			return nil
		},
	}
	bindReadFlags(cmd)
	return cmd
}

func printUser(cmd *cobra.Command, u api.UserItem) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%s (id %d)\n", orDash(u.Username), u.ID)
	if u.Image != "" {
		fmt.Fprintf(out, "  image: %s\n", u.Image)
	}
}
