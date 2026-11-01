package blockproto

import "strings"

// civitaiParentOrigins are the Civitai host origins an App runs inside: every
// Civitai domain serves the App pages (`/apps/run/<slug>`) and the preview route
// (`/apps/dev/<blockId>`), so each must be able to FRAME a dev server and SEND
// the app its BLOCK_INIT.
//
// This is the one list in this repo. The scaffolds render it into their dev
// server's `frame-ancestors` CSP (src/dev-embed.{ts,js}) and into the
// postMessage allowlist (`VITE_BLOCK_ALLOWED_PARENT_ORIGINS` in the .env files),
// and `civitai app dev-tunnel`'s preflight (internal/devtunnel) checks an app
// against it.
//
// It matches the host list the SDK falls back to when an app sets no allowlist
// (civitai-app-starters, packages/civitai-sdk/src/core/get-transport.ts,
// DEFAULT_ALLOWED_PARENT_ORIGINS), minus that list's `*.civitai.com` and
// `*.civitaic.com` wildcards: these are exact origins, because the allowlist is
// matched exactly and a CSP wildcard does not cover the bare domain.
var civitaiParentOrigins = [...]string{
	"https://civitai.com",
	"https://civitai.red",
	"https://civitai.green",
}

// CivitaiParentOrigins returns the Civitai host origins, civitai.com first. It
// returns a fresh copy: callers must not be able to change the list.
func CivitaiParentOrigins() []string {
	return append([]string(nil), civitaiParentOrigins[:]...)
}

// CivitaiParentOriginsCSP returns the origins as CSP source expressions, space
// separated: `https://civitai.com https://civitai.red https://civitai.green`.
func CivitaiParentOriginsCSP() string {
	return strings.Join(civitaiParentOrigins[:], " ")
}

// CivitaiParentOriginsCSV returns the origins as a
// `VITE_BLOCK_ALLOWED_PARENT_ORIGINS` value, comma separated.
func CivitaiParentOriginsCSV() string {
	return strings.Join(civitaiParentOrigins[:], ",")
}

// CivitaiParentOriginsJS returns the origins as the elements of a JavaScript
// array literal, one quoted origin per line at two-space indent, each followed
// by a comma, for a scaffold to place between `[` and `]`.
func CivitaiParentOriginsJS() string {
	var b strings.Builder
	for _, o := range civitaiParentOrigins {
		b.WriteString("  '" + o + "',\n")
	}
	return b.String()
}
