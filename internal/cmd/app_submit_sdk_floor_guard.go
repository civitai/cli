package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/validate"
	"github.com/civitai/cli/pkg/civitai"
)

// THE OUTDATED-SDK GUARD.
//
// The platform publishes a MINIMUM version for each App Blocks SDK package
// (GET /api/v1/blocks/sdk-floor — @civitai/sdk and @civitai/blocks-react
// today). An app built on an older one misses host messages the platform now
// sends, so `app submit` REFUSES while the app's lockfile installs a package
// below its minimum, and names the upgrade. --allow-outdated-sdk submits anyway,
// and the packages it overrode travel with the submit (Provenance.OutdatedSDK →
// `sdkFloorOverride`), where the moderator reviewing it is shown them.
//
// 🔴 THE MINIMUM IS THE SERVER'S, NOT npm `latest` AND NOT A CONSTANT HERE. A
// constant would go stale with every CLI binary in the field; the server owns
// what it accepts. So a server that publishes none (404 — it predates the
// route) gets a one-line note and NO refusal, and a read that fails any other
// way warns and proceeds: this guard is an accident preventer, and a transient
// API blip must not block every submit in the fleet.
//
// 🔴 THE INSTALLED VERSION IS THE LOCKFILE'S, NOT package.json's RANGE. `^0.31.0`
// says nothing about what the build installs (and on 0.x a caret never leaves
// its minor), so the version judged is the one validate.LockedVersion reads
// from the lockfile the platform build installs from — the same reader the
// SDK-mount gate in internal/validate uses.
//
// It runs where the version guard runs: on the upload path only, after the
// confirmation, before packaging. --package-only and the no-token fallback
// never reach the server, so they neither read the minimum nor refuse on it.
// The server ALSO flags a below-minimum lockfile on its own (a non-blocking
// `sdk-below-floor` advisory), so a CLI that predates this guard is still told.

// ErrOutdatedSDK classifies the outdated-SDK refusal. Like ErrVersionRegression
// it is attached (civitai.Tag) to the message-bearing error and is deliberately
// NOT ErrBadRequest: the invocation is well-formed; what is wrong is the
// project, so it exits 1 with the other project-state verdicts.
var ErrOutdatedSDK = errors.New("the app installs an SDK package below the platform minimum")

// AllowOutdatedSDKFlag is the waiver's name, spelled once so the refusal, the
// help and the flag registration cannot disagree.
const AllowOutdatedSDKFlag = "--allow-outdated-sdk"

// sdkFloorReader reads the platform's published minimums (appapi.GetSDKFloor).
type sdkFloorReader func(ctx context.Context) ([]appapi.SDKFloor, error)

// checkSDKFloor returns the packages the app installs below the platform
// minimum when allow is set (to be recorded with the submit), refuses with
// ErrOutdatedSDK when it is not, and returns nothing when none are below or the
// check cannot be made.
//
// Every non-refusal branch that loses information says so on warn; the two
// that carry none (no floored package in the app at all, everything at or
// above its minimum) are silent.
func checkSDKFloor(ctx context.Context, read sdkFloorReader, warn io.Writer, dir, buildCommand string, allow bool) ([]appapi.OutdatedSDK, error) {
	floors, err := read(ctx)
	if errors.Is(err, appapi.ErrSDKFloorNotPublished) {
		fmt.Fprintln(warn, "Note: this server publishes no SDK minimum, so the outdated-SDK check is skipped.")
		return nil, nil
	}
	if err != nil {
		warnf(warn, "could not read the platform's SDK minimum (%v) — submitting without the outdated-SDK check.", err)
		return nil, nil
	}

	lockfile, addCmd := validate.LockfileFor(buildCommand)
	declared := declaredDependencies(dir)
	var below []appapi.OutdatedSDK
	for _, f := range floors {
		name, minimum := strings.TrimSpace(f.Name), strings.TrimSpace(f.Minimum)
		if name == "" || minimum == "" {
			continue
		}
		installed, ok := validate.LockedVersion(dir, buildCommand, name)
		if !ok {
			// Not in the lockfile. Silent when the app does not depend on it at
			// all (the common case for blocks-react); a warning when package.json
			// says it does, because then the check could not be made.
			if declared[name] {
				warnf(warn, "cannot read the installed version of %s from %s — its minimum (%s) is not checked.",
					name, lockfile, minimum)
			}
			continue
		}
		isBelow, ok := validate.VersionBelow(installed, minimum)
		if !ok {
			warnf(warn, "cannot order %s %s against the platform minimum %s — not checked.", name, installed, minimum)
			continue
		}
		if isBelow {
			below = append(below, appapi.OutdatedSDK{Package: name, Installed: installed, Minimum: minimum})
		}
	}
	if len(below) == 0 {
		return nil, nil
	}

	var list strings.Builder
	upgrades := make([]string, 0, len(below))
	for _, b := range below {
		fmt.Fprintf(&list, "\n  %s %s (platform minimum %s)", b.Package, b.Installed, b.Minimum)
		upgrades = append(upgrades, b.Package+"@latest")
	}
	upgrade := addCmd + " " + strings.Join(upgrades, " ")
	if allow {
		warnf(warn, "submitting with %s — %s installs below the platform minimum:%s\n"+
			"The override and these versions are recorded with the submission and shown to the moderator who reviews it.",
			AllowOutdatedSDKFlag, lockfile, list.String())
		return below, nil
	}
	return nil, civitai.Tag(ErrOutdatedSDK, fmt.Errorf(
		"refusing to submit — %s installs SDK packages below the platform minimum:%s\n"+
			"Upgrade, commit the updated %s, and submit again:\n  %s\n"+
			"Or pass %s to submit anyway; the moderator who reviews it is shown the versions and the override",
		lockfile, list.String(), lockfile, upgrade, AllowOutdatedSDKFlag))
}

// declaredDependencies is the set of packages package.json names in
// dependencies or devDependencies. Unreadable or absent → empty.
func declaredDependencies(dir string) map[string]bool {
	out := map[string]bool{}
	body, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return out
	}
	var pkg struct {
		Dependencies    map[string]json.RawMessage `json:"dependencies"`
		DevDependencies map[string]json.RawMessage `json:"devDependencies"`
	}
	if json.Unmarshal(body, &pkg) != nil {
		return out
	}
	for name := range pkg.Dependencies {
		out[name] = true
	}
	for name := range pkg.DevDependencies {
		out[name] = true
	}
	return out
}
