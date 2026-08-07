package genapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/civitai/cli/pkg/civitai"
)

// BlobUploadURLPath is the REST route that mints a presigned consumer-blob
// upload URL.
//
// 🔴 It is REST, not tRPC — the one generation-adjacent route that is. Item 12
// of AGENTS.md says generation lives only behind tRPC procedures; this endpoint
// is the exception and reading it as "must be tRPC like the others" produces a
// 404. It is a plain Next.js API route
// (civitai/civitai -> src/pages/api/orchestrator/getConsumerBlobUploadUrl.ts),
// it is a **GET** with no body and no query parameters, and its reply is a bare
// JSON object — NOT wrapped in a tRPC `result.data.json` envelope, so it must
// never be routed through unwrapTRPC.
const BlobUploadURLPath = "/api/orchestrator/getConsumerBlobUploadUrl"

// BlobUploadTarget is the getConsumerBlobUploadUrl reply.
//
// Shape verified against the server's own type
// (`ConsumerBlobPresignResponse` in @civitai/client): exactly `uploadUrl` and
// `expiresAt`, and nothing else. In particular it does NOT carry the eventual
// blob URL — that comes back from the upload POST itself (see UploadedBlob).
type BlobUploadTarget struct {
	UploadURL string `json:"uploadUrl"`
	ExpiresAt string `json:"expiresAt"`
}

// UploadedBlob is the orchestrator's reply to the upload POST — its `Blob`
// type.
//
// URL is a POINTER-shaped `*string` on the server (`url?: null | string`) and a
// plain string here, but the DISTINCTION IS PRESERVED by checking it for empty
// before use: a blob that is accepted but not yet servable comes back with
// `url: null`, which decodes to "". Referencing "" in the graph would produce
// `images:[{"url":""}]` — accepted by the graph's bare `z.string()` and then
// failing deep in the orchestrator as "Input image failed to download from
// URL", after the user has paid. GetImageBlobURL refuses it instead.
type UploadedBlob struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Available bool   `json:"available"`
	URL       string `json:"url"`
}

// GetBlobUploadTarget asks the server for a presigned consumer-blob upload URL.
//
// This call IS authenticated — it is the CLI asking Civitai, on the user's
// behalf, for permission to upload. The credential-free rule applies to the
// SUBSEQUENT request to the returned URL, not to this one; see UploadImageBlob.
func (c *Client) GetBlobUploadTarget(ctx context.Context) (*BlobUploadTarget, error) {
	build := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+BlobUploadURLPath, nil)
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, blobUploadError(status, raw)
	}
	var out BlobUploadTarget
	if err := json.Unmarshal(raw, &out); err != nil || strings.TrimSpace(out.UploadURL) == "" {
		return nil, fmt.Errorf("unexpected getConsumerBlobUploadUrl response: %s", string(raw))
	}
	return &out, nil
}

// blobUploadError maps a presign failure onto the CLI's error kinds so the exit
// code is pinned by errors.Is rather than by message text (AGENTS.md item 7).
//
// The route answers a caller the orchestrator will not serve with **403 and a
// bare text body** (not JSON, not a tRPC error envelope) — see the handler's
// `catch` arm — so the body is surfaced verbatim rather than parsed.
func blobUploadError(status int, raw []byte) error {
	msg := strings.TrimSpace(string(raw))
	if len(msg) > 400 {
		msg = msg[:400] + "…"
	}
	base := fmt.Sprintf("the server refused to issue an image-upload URL (%d)", status)
	if msg != "" {
		base += ": " + msg
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf(
			"%s — uploading a reference image needs a credential with the AI Services scopes (`civitai login --scopes generate`, or a full-scope personal API key); check yours with `civitai whoami`", base))
	default:
		return civitai.TagStatus(status, fmt.Errorf("%s", base))
	}
}

// UploadImageBlob uploads one image's bytes and returns the URL to reference in
// the generation graph's `images[]`.
//
// 🔴 The two hops have DELIBERATELY DIFFERENT credential handling, and that
// asymmetry is the whole point of this function existing:
//
//   - hop 1 (GetBlobUploadTarget) is an authenticated call to Civitai;
//   - hop 2 (up.UploadPresigned) carries NO credential at all.
//
// The upload URL is SERVER-SUPPLIED and is served from a *.civitai.com host,
// which `isTrustedDownloadHost` matches — so routing hop 2 through any
// token-carrying client would send a full-scope personal API key (25 scopes,
// including ModelsDelete and VaultWrite) to a request its own signature already
// authorizes. That is the identical hazard AGENTS.md item 17 documents for blob
// DOWNLOADS, and it is closed the identical way: a seam that never has a
// credential to attach, not a conditional inside one that does.
//
// contentType must be one the orchestrator accepts; the caller derives it from
// the decoded image format rather than from the filename extension.
func (c *Client) UploadImageBlob(ctx context.Context, up civitai.PresignedUploader, contentType string, body []byte) (string, error) {
	target, err := c.GetBlobUploadTarget(ctx)
	if err != nil {
		return "", err
	}
	resp, err := up.UploadPresigned(ctx, target.UploadURL, contentType, body)
	if err != nil {
		return "", fmt.Errorf("uploading the reference image failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody()))
	if err != nil {
		return "", fmt.Errorf("reading the image-upload response failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		out := fmt.Sprintf("the image upload was rejected (%d)", resp.StatusCode)
		if msg != "" {
			out += ": " + msg
		}
		// A presigned URL that has expired reads as a 401/403 here and a token
		// would not fix it — say what actually helps.
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			out += " — the presigned upload URL is signed and short-lived; re-run the command to mint a fresh one"
		}
		return "", civitai.TagStatus(resp.StatusCode, fmt.Errorf("%s", out))
	}

	var blob UploadedBlob
	if err := json.Unmarshal(raw, &blob); err != nil {
		return "", fmt.Errorf("unexpected image-upload response: %s", string(raw))
	}
	if strings.TrimSpace(blob.URL) == "" {
		// See the UploadedBlob doc: an empty/absent url must never reach the
		// graph, where it would be accepted and then fail after the charge.
		return "", fmt.Errorf(
			"the image upload returned no usable URL (blob id %q, available=%t) — the reference image cannot be attached; try again, or pass a public https image URL to --image instead",
			blob.ID, blob.Available)
	}
	return blob.URL, nil
}
