// Command civitai is the unified Civitai CLI.
package main

import (
	"fmt"
	"os"

	"github.com/civitai/cli/internal/cmd"
)

// Build metadata, injected at release time via goreleaser ldflags
// (see .goreleaser.yaml / .github/workflows/release.yml). They default to
// "dev"/"none"/"unknown" for `go install` / plain source builds.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetBuildInfo(version, commit, date)
	if err := cmd.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
