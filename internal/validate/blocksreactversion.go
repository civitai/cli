package validate

// blocksreactversion.go is the VERSION GATE on the SDK-mount finding
// (readyack.go's sdkMountChecks).
//
// WHY IT EXISTS. The SDK-mount finding is true only for `@civitai/blocks-react`
// versions BEFORE civitai/civitai-app-starters#632 ("every hook connects the
// transport at mount"). From the first release carrying that fix, every hook
// connects in a mount effect, so an app using only `useBuzzWorkflow` acks
// correctly — and an ungated finding would be a false warning (rc=1 under
// `--strict`) at a correct app, the expensive direction. So the finding fires
// only when the LOCKFILE-RESOLVED blocks-react version is older than that
// release.
//
// WHY THE LOCKFILE, NOT package.json. The range in package.json does not say
// what is installed: `page-money` scaffolds `^0.71.0`, and on 0.x a caret stays
// inside the minor, so `^0.71.0` never resolves to a 0.72.x fix — while `^0.72.0`
// may resolve either side of it. The lockfile is what the platform build
// installs from (`npm ci` / frozen installs; lockfile.go makes it mandatory), so
// it is the version that actually ships.
//
// "CANNOT OBSERVE" IS SILENCE. Once the threshold is set, a lockfile that is
// missing, unreadable, over the size cap, unparseable, has no entry for the
// package, or resolves it to more than one version gives no version — and the
// finding stays quiet (a false negative, the cheap direction; a missing lockfile
// is already a hard error from lockfileChecks).

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/civitai/cli/internal/blockproto"
	"gopkg.in/yaml.v3"
)

// blocksReactMountFixVersion is the FIRST `@civitai/blocks-react` release that
// contains civitai/civitai-app-starters#632. The SDK-mount finding fires only
// for a lockfile-resolved version strictly below it.
//
// 🔴🔴 PLACEHOLDER — NOT YET SET. As of 2026-10-12 #632 is an OPEN patch
// changeset and the latest published version is 0.72.0. The exact version it
// will publish as cannot be read from the changeset config: changesets bumps by
// the HIGHEST pending changeset at release time, and other open PRs carry
// pending changesets for this package (e.g. #631, itself on a release hold), so
// #632 may ship as 0.72.1, 0.72.2 or a minor. "" means "no released version
// contains the fix", which is TRUE TODAY: every published version is pre-fix, so
// the finding fires for every version — the same behaviour as having no gate.
// SET THIS TO THE REAL RELEASE the moment #632 is published (read it from the
// npm registry or the package CHANGELOG, never from the changeset), or the
// finding becomes a false warning at every correct app on the fixed SDK.
const blocksReactMountFixVersion = ""

// blocksReactPreFix reports whether the SDK-mount finding applies to the
// project in dir, against the fix release `fix` ("" = none released yet).
// Taking `fix` as a parameter is what lets the tests exercise the gate at a
// real threshold while the constant is still the placeholder.
func blocksReactPreFix(dir, buildCommand, fix string) bool {
	if fix == "" {
		return true
	}
	fixV, ok := parseSemver(fix)
	if !ok {
		return false
	}
	locked, ok := lockedBlocksReactVersion(dir, buildCommand)
	if !ok {
		return false
	}
	v, ok := parseSemver(locked)
	if !ok {
		return false
	}
	return v.less(fixV)
}

// lockedBlocksReactVersion returns the version of `@civitai/blocks-react` that
// the lockfile the platform build installs from resolves the app's DIRECT
// dependency to. The lockfile is chosen exactly as lockfileChecks chooses it
// (packageManagerFor on buildCommand).
func lockedBlocksReactVersion(dir, buildCommand string) (string, bool) {
	pm := packageManagerFor(strings.TrimSpace(buildCommand))
	path := filepath.Join(dir, pm.lockfile)
	// Same gate as lockfileContentDefect: a symlinked lockfile is not in the
	// submitted bundle, and an oversized one is not read.
	if !regularFileExists(path) {
		return "", false
	}
	info, err := os.Lstat(path)
	if err != nil || info.Size() > maxLockfileBytes {
		return "", false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	for bytes.HasPrefix(body, []byte("\xef\xbb\xbf")) {
		body = body[3:]
	}
	switch pm.name {
	case pmPnpm.name:
		return pnpmLockedVersion(body, blockproto.BlocksReactPackage)
	case pmYarn.name:
		return yarnLockedVersion(body, blockproto.BlocksReactPackage)
	default:
		return npmLockedVersion(body, blockproto.BlocksReactPackage)
	}
}

// npmLockedVersion reads package-lock.json: `packages["node_modules/<name>"]`
// (lockfileVersion 2 and 3), falling back to the v1 `dependencies[<name>]`.
// Only the ROOT install is read — a copy nested under another package is not
// the one the app imports.
func npmLockedVersion(body []byte, name string) (string, bool) {
	type entry struct {
		Version string `json:"version"`
	}
	var lock struct {
		Packages     map[string]entry `json:"packages"`
		Dependencies map[string]entry `json:"dependencies"`
	}
	if err := json.Unmarshal(body, &lock); err != nil {
		return "", false
	}
	if e, ok := lock.Packages["node_modules/"+name]; ok && e.Version != "" {
		return e.Version, true
	}
	if e, ok := lock.Dependencies[name]; ok && e.Version != "" {
		return e.Version, true
	}
	return "", false
}

// pnpmLockedVersion reads pnpm-lock.yaml. The direct dependency's resolved
// version sits under `importers["."]` (lockfile v6+ with importers, and v9) or
// at the top level (single-project v5/v6), either as a bare string (v5) or as
// `{specifier, version}` (v6+). A peer suffix — `0.71.0(react@19.0.0)` (v6+) or
// `0.71.0_react@19.0.0` (v5) — is stripped.
func pnpmLockedVersion(body []byte, name string) (string, bool) {
	type depSets struct {
		Dependencies    map[string]yaml.Node `yaml:"dependencies"`
		DevDependencies map[string]yaml.Node `yaml:"devDependencies"`
	}
	var lock struct {
		Dependencies    map[string]yaml.Node `yaml:"dependencies"`
		DevDependencies map[string]yaml.Node `yaml:"devDependencies"`
		Importers       map[string]depSets   `yaml:"importers"`
	}
	if err := yaml.Unmarshal(body, &lock); err != nil {
		return "", false
	}
	sets := []depSets{{lock.Dependencies, lock.DevDependencies}}
	if root, ok := lock.Importers["."]; ok {
		sets = append([]depSets{root}, sets...)
	}
	for _, s := range sets {
		for _, m := range []map[string]yaml.Node{s.Dependencies, s.DevDependencies} {
			n, ok := m[name]
			if !ok {
				continue
			}
			var v string
			switch n.Kind {
			case yaml.ScalarNode:
				v = n.Value
			case yaml.MappingNode:
				var e struct {
					Version string `yaml:"version"`
				}
				if n.Decode(&e) != nil {
					return "", false
				}
				v = e.Version
			}
			if i := strings.IndexAny(v, "(_"); i >= 0 {
				v = v[:i]
			}
			if v != "" {
				return v, true
			}
		}
	}
	return "", false
}

// yarnLockedVersion reads yarn.lock, v1 (`"name@^1":` then `  version "1.0.0"`)
// and berry (`"name@npm:^1":` then `  version: 1.0.0`). Every entry whose header
// names the package is read; more than one DISTINCT version means we cannot say
// which one the app imports, so it is unobservable.
func yarnLockedVersion(body []byte, name string) (string, bool) {
	found := map[string]bool{}
	inEntry := false
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line[0] != ' ' && line[0] != '\t' {
			inEntry = false
			header := strings.TrimSuffix(line, ":")
			for _, spec := range strings.Split(header, ",") {
				spec = strings.Trim(strings.TrimSpace(spec), `"`)
				if strings.HasPrefix(spec, name+"@") {
					inEntry = true
				}
			}
			continue
		}
		if !inEntry {
			continue
		}
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "version") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(t, "version"))
		v = strings.TrimSpace(strings.TrimPrefix(v, ":"))
		v = strings.Trim(v, `"`)
		if v != "" {
			found[v] = true
		}
	}
	if len(found) != 1 {
		return "", false
	}
	for v := range found {
		return v, true
	}
	return "", false
}

// semver is the subset of SemVer 2.0 precedence this gate needs.
type semver struct {
	major, minor, patch int
	pre                 string // "" for a release
}

// parseSemver parses MAJOR.MINOR.PATCH[-PRE][+BUILD]. Anything else is not a
// version, and the caller treats that as unobservable.
func parseSemver(s string) (semver, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var v semver
	if i := strings.IndexByte(s, '-'); i >= 0 {
		v.pre, s = s[i+1:], s[:i]
		if v.pre == "" {
			return semver{}, false
		}
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p == "" {
			return semver{}, false
		}
		nums[i] = n
	}
	v.major, v.minor, v.patch = nums[0], nums[1], nums[2]
	return v, true
}

// less is SemVer precedence for this subset: numeric triple first, then a
// pre-release sorts BELOW its release. Two pre-releases of one triple compare
// by identifiers (numeric ones numerically, numeric below alphanumeric).
func (a semver) less(b semver) bool {
	if a.major != b.major {
		return a.major < b.major
	}
	if a.minor != b.minor {
		return a.minor < b.minor
	}
	if a.patch != b.patch {
		return a.patch < b.patch
	}
	switch {
	case a.pre == b.pre:
		return false
	case a.pre == "":
		return false
	case b.pre == "":
		return true
	}
	ai, bi := strings.Split(a.pre, "."), strings.Split(b.pre, ".")
	for i := 0; i < len(ai) && i < len(bi); i++ {
		if ai[i] == bi[i] {
			continue
		}
		an, aerr := strconv.Atoi(ai[i])
		bn, berr := strconv.Atoi(bi[i])
		switch {
		case aerr == nil && berr == nil:
			return an < bn
		case aerr == nil:
			return true
		case berr == nil:
			return false
		}
		return ai[i] < bi[i]
	}
	return len(ai) < len(bi)
}
