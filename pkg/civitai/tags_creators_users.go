package civitai

import (
	"context"
	"net/url"
)

// TagItem is a `GET /api/v1/tags` item: the tag name + a convenience link to
// the models filtered by it.
type TagItem struct {
	Name string `json:"name"`
	Link string `json:"link"`
}

// TagSearchResult bundles parsed tags + metadata with the body that decoded,
// for --json passthrough. Raw is NOT promised to be the server's own bytes: see
// EscapeJSONStringControlChars for the one case where it is the repaired body
// instead.
type TagSearchResult struct {
	Items    []TagItem `json:"items"`
	Metadata Metadata  `json:"metadata"`
	Raw      []byte    `json:"-"`
}

// SearchTags calls GET /api/v1/tags.
func (c *Client) SearchTags(ctx context.Context, q url.Values) (*TagSearchResult, error) {
	var res TagSearchResult
	raw, err := c.getInto(ctx, "/api/v1/tags", q, &res)
	if err != nil {
		return nil, err
	}
	res.Raw = raw
	return &res, nil
}

// CreatorItem is a `GET /api/v1/creators` item.
type CreatorItem struct {
	// Username is a FlexString, not a string: an all-digit username arrives as a
	// bare JSON number. See FlexString — do not "fix" it back to a string.
	Username   FlexString `json:"username"`
	ModelCount int        `json:"modelCount"`
	Link       string     `json:"link"`
}

// CreatorSearchResult bundles parsed creators + metadata with the body that
// decoded, for --json passthrough. Raw is NOT promised to be the server's own
// bytes: see EscapeJSONStringControlChars for the one case where it is the
// repaired body instead.
type CreatorSearchResult struct {
	Items    []CreatorItem `json:"items"`
	Metadata Metadata      `json:"metadata"`
	Raw      []byte        `json:"-"`
}

// SearchCreators calls GET /api/v1/creators.
func (c *Client) SearchCreators(ctx context.Context, q url.Values) (*CreatorSearchResult, error) {
	var res CreatorSearchResult
	raw, err := c.getInto(ctx, "/api/v1/creators", q, &res)
	if err != nil {
		return nil, err
	}
	res.Raw = raw
	return &res, nil
}

// UserItem is the subset of a `GET /api/v1/users` search item the CLI renders.
// The public users search returns basic identity fields; richer fields
// (status/avatar) are only included for internal system requests.
type UserItem struct {
	ID int `json:"id"`
	// Username is a FlexString, not a string: an all-digit username can arrive as
	// a bare JSON number, and this field must decode it. The path that reaches it
	// is `civitai users get <name>` — the ?query= search, whose "closest matches"
	// list may name an all-digit user (TestUsersGetListsANumericCandidate).
	// `civitai users get <digits>` is NOT that path: internal/cmd/users.go routes
	// any strconv.Atoi-parsable argument to ?ids=, an ID lookup, so a user whose
	// USERNAME is all digits is not reachable by name through this command.
	// See FlexString — do not "fix" it back to a string.
	Username FlexString `json:"username"`
	Image    string     `json:"image"`
}

// UserSearchResult bundles parsed users with the body that decoded, for --json
// passthrough. Raw is NOT promised to be the server's own bytes: see
// EscapeJSONStringControlChars for the one case where it is the repaired body
// instead. The public users endpoint returns only `{ items }` (no pagination
// metadata).
type UserSearchResult struct {
	Items []UserItem `json:"items"`
	Raw   []byte     `json:"-"`
}

// SearchUsers calls GET /api/v1/users — the public user search. This is the
// only public users read route: the per-id `/api/v1/users/{userId}` route is an
// internal webhook (POST + system token) and is NOT usable by the CLI, so
// `civitai users get` resolves a user through this search (by ?query= for a
// name, or ?ids= for a numeric id).
func (c *Client) SearchUsers(ctx context.Context, q url.Values) (*UserSearchResult, error) {
	var res UserSearchResult
	raw, err := c.getInto(ctx, "/api/v1/users", q, &res)
	if err != nil {
		return nil, err
	}
	res.Raw = raw
	return &res, nil
}
