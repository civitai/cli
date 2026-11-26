package appapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/civitai/cli/internal/redact"
)

// SDKFloorPath is the platform's published minimum versions of the App Blocks
// SDK packages (GET; civitai/civitai src/pages/api/v1/blocks/sdk-floor.ts). It
// is unauthenticated and static, so reading it costs no token and no rate-limit
// slot.
const SDKFloorPath = "/api/v1/blocks/sdk-floor"

// SDKFloor is one package the platform publishes a minimum version for.
type SDKFloor struct {
	Name    string `json:"name"`
	Minimum string `json:"minimum"`
}

// OutdatedSDK is one package an app installs below its published minimum. It
// is what `app submit --allow-outdated-sdk` sends as `sdkFloorOverride`, so
// the field names track the server's schema exactly.
type OutdatedSDK struct {
	Package   string `json:"package"`
	Installed string `json:"installed"`
	Minimum   string `json:"minimum"`
}

// ErrSDKFloorNotPublished is the server answering 404 for SDKFloorPath: a
// server that predates the published minimum. The caller skips the check; it
// never refuses on it.
var ErrSDKFloorNotPublished = errors.New("this server publishes no SDK minimum")

// GetSDKFloor reads the platform's published SDK minimums.
func (c *Client) GetSDKFloor(ctx context.Context) ([]SDKFloor, error) {
	build := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+SDKFloorPath, nil)
	}
	// No token: the route is public, and sending one would make this read
	// depend on a credential refresh it does not need.
	status, raw, err := c.doOnceWith(c.HTTP, build, "")
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, ErrSDKFloorNotPublished
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("server returned %d for %s", status, SDKFloorPath)
	}
	// A pointer, so a reply with NO `packages` key (or `"packages": null`) is
	// told apart from an explicit `"packages": []`. The empty list is the server
	// saying "no floor today"; a missing key is a reply this client does not
	// understand, and answering nil for it would switch the guard off with no
	// signal. So it is an error, and the caller takes its warn-and-proceed path.
	var out struct {
		Packages *[]SDKFloor `json:"packages"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, redact.CredentialBodyError("unexpected "+SDKFloorPath+" response", raw, err)
	}
	if out.Packages == nil {
		return nil, redact.CredentialBodyError("unexpected "+SDKFloorPath+" response: no packages list", raw, nil)
	}
	return *out.Packages, nil
}

// The server's sdkFloorOverride schema (civitai/civitai#10593,
// submitVersionSchema): at most 8 entries, `package` at most 214 characters,
// `installed` and `minimum` at most 64. A submit that exceeds any of them is
// answered 400, so an override past them would turn --allow-outdated-sdk into a
// failed submit.
const (
	sdkOverrideMaxEntries    = 8
	sdkOverrideMaxPackageLen = 214
	sdkOverrideMaxVersionLen = 64
)

// capSDKFloorOverride returns the override cut to the server's limits: the
// first sdkOverrideMaxEntries entries, each string truncated on a rune boundary
// to at most its cap in BYTES. A byte count is never below the UTF-16 length
// zod measures, so the result is inside the server's limit whatever the text.
// The override is a record for the moderator, not a key anything looks up, so a
// truncated pre-release string loses nothing a submit depends on.
func capSDKFloorOverride(in []OutdatedSDK) []OutdatedSDK {
	if len(in) == 0 {
		return nil
	}
	if len(in) > sdkOverrideMaxEntries {
		in = in[:sdkOverrideMaxEntries]
	}
	out := make([]OutdatedSDK, len(in))
	for i, o := range in {
		out[i] = OutdatedSDK{
			Package:   truncateBytes(o.Package, sdkOverrideMaxPackageLen),
			Installed: truncateBytes(o.Installed, sdkOverrideMaxVersionLen),
			Minimum:   truncateBytes(o.Minimum, sdkOverrideMaxVersionLen),
		}
	}
	return out
}

// truncateBytes cuts s to at most n bytes without splitting a UTF-8 sequence.
func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
