package config

import (
	"os"
	"testing"

	"github.com/civitai/cli/internal/config/configtest"
)

// TestMain runs this package's tests with HOME, APPDATA and the XDG roots
// pointed at a temp tree, so no test can read or write the developer's real
// civitai config on any OS (issue #931). internal/config's
// TestEveryConfigReachingPackageIsolatesHome requires it.
func TestMain(m *testing.M) { os.Exit(configtest.Run(m)) }
