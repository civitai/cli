package appapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

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
	var out struct {
		Packages []SDKFloor `json:"packages"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, redact.CredentialBodyError("unexpected "+SDKFloorPath+" response", raw, err)
	}
	return out.Packages, nil
}
