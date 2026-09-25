// Package manifest holds the App manifest filename constant and a
// lightweight reader for the fields the CLI needs (slug/version/name).
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync"

	cli "github.com/civitai/cli"
)

// blockIDField matches the top-level `"blockId": "<value>"` pair. Slug values
// never contain a double-quote, so `[^"]*` captures the value exactly. Group 1 is
// the key + separator prefix, preserved verbatim on rewrite.
var blockIDField = regexp.MustCompile(`("blockId"\s*:\s*)"[^"]*"`)

// Filename is the one mandatory file in an App, at the project root.
// The server matches this exact path inside the ZIP (no nesting).
const Filename = "block.manifest.json"

// Manifest is a partial view of block.manifest.json — only the fields the CLI
// reads directly. Full validation is done against the JSON Schema, not this
// struct, so unknown fields are intentionally ignored here.
type Manifest struct {
	BlockID      string   `json:"blockId"`
	Version      string   `json:"version"`
	Name         string   `json:"name"`
	BuildCommand string   `json:"buildCommand"`
	OutputDir    string   `json:"outputDir"`
	Scopes       []string `json:"scopes"`
	Auth         string   `json:"auth"`
}

// authKinds is the set of `auth` values the vendored manifest schema admits,
// DERIVED from schema/app-block.manifest.schema.json's own `enum` rather than
// re-typed here.
//
// 🔴 IT IS DERIVED BECAUSE A SECOND HAND-WRITTEN COPY SILENTLY REGENERATES THE
// BUG THIS FIELD EXISTS TO FIX. The schema half is not hand-maintained:
// scripts/check-canonical-schema.sh diffs it against the live canonical URL on
// every CI run, so a new server-side auth kind arrives as a red `schema-drift`
// and is resolved by a re-vendor chore — which is exactly how `auth` itself
// arrived (#693, a five-line commit). A re-typed allowlist does not follow that
// commit: `app validate` would accept the new kind, this reader would degrade it
// to "", the mint would omit declaredAuth, and the author would silently get a
// block token — the same symptom declaring `auth` was added to remove.
//
// 🔴 THE MEMBERSHIP TEST IS LOAD-BEARING FOR MORE THAN CORRECTNESS: it is what
// keeps the value the caller ECHOES to the terminal a vendored literal instead
// of arbitrary manifest text. `civitai app dev-tunnel` prints this value on a
// "Declaring auth:" line with no sanitizer, safely only because nothing outside
// the schema's own enum can reach it — compare sanitizeScopeForDisplay, which
// exists because the scopes line does carry author-supplied strings. Do NOT
// "future-proof" this by passing m.Auth through unchecked.
var authKinds = sync.OnceValue(func() map[string]struct{} {
	var doc struct {
		Properties struct {
			Auth struct {
				Enum []string `json:"enum"`
			} `json:"auth"`
		} `json:"properties"`
	}
	out := make(map[string]struct{})
	if err := json.Unmarshal(cli.SchemaJSON, &doc); err != nil {
		return out // an unparseable schema admits nothing; LoadAuth then sends nothing
	}
	for _, k := range doc.Properties.Auth.Enum {
		if k != "" {
			out[k] = struct{}{}
		}
	}
	return out
})

// AuthKinds returns the `auth` values the vendored schema admits, sorted. Used
// to tell an author what they may have meant, and by the guard test that pins
// the derivation against the schema.
func AuthKinds() []string {
	out := make([]string, 0, len(authKinds()))
	for k := range authKinds() {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// LoadAuth reads the manifest's `auth` (the schema's `block-token` / `oauth`)
// with the same degrade-to-nothing rules as LoadScopes: a missing manifest, an
// unreadable file or malformed JSON all read as "" and are never an error.
//
// The second return distinguishes the two ways of getting "" that a caller must
// treat DIFFERENTLY: false means the manifest declared no `auth` at all (there
// is nothing to say), true means it declared one the vendored schema does not
// admit — a typo the author cannot otherwise see here, because `dev-tunnel` does
// not run the validator that would report it.
func LoadAuth(dir string) (auth string, unrecognized bool) {
	raw, err := os.ReadFile(Path(dir))
	if err != nil {
		return "", false
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", false
	}
	if m.Auth == "" {
		return "", false
	}
	if _, ok := authKinds()[m.Auth]; ok {
		return m.Auth, false
	}
	return "", true
}

// LoadScopes reads the `scopes` array from the manifest in dir, degrading
// gracefully: a missing manifest, an unreadable file, or malformed JSON all
// return nil (no scopes) with no error. It is used by `civitai app dev-tunnel`
// (and `civitai app dev-token`) to send the dev's LOCAL manifest scopes for the
// server's no-row mint path — the slug arg still identifies a registered app even
// when the manifest is absent, so a read failure must never block minting.
func LoadScopes(dir string) []string {
	raw, err := os.ReadFile(Path(dir))
	if err != nil {
		return nil
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m.Scopes
}

// Path returns the manifest path for a project directory.
func Path(dir string) string { return filepath.Join(dir, Filename) }

// Load reads and JSON-parses the manifest in dir.
func Load(dir string) (*Manifest, error) {
	p := Path(dir)
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no %s found in %s — is this an App project? run `civitai app create` to create one", Filename, dir)
		}
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, invalidJSON(raw, err)
	}
	return &m, nil
}

// SetBlockID rewrites the manifest's `blockId` field in place, preserving every
// other field, its ordering, and the file's formatting (it edits only the
// blockId value in the raw bytes). It validates that the result still parses as
// JSON before writing, and writes atomically (temp file + rename) preserving the
// original file mode, so a failure never clobbers the existing manifest. Used by
// `civitai app dev-token` to auto-rename a slug that collides with another
// account's app.
func SetBlockID(dir, newBlockID string) error {
	p := Path(dir)
	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no %s found in %s — is this an App project?", Filename, dir)
		}
		return err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	if _, err := json.Marshal(newBlockID); err != nil { // defensive; string always marshals
		return err
	}

	loc := blockIDField.FindSubmatchIndex(raw)
	var updated []byte
	if loc != nil {
		// loc[2]:loc[3] is group 1 (the `"blockId": ` prefix); loc[1] is the end
		// of the whole match (just past the closing quote of the old value).
		updated = make([]byte, 0, len(raw)+len(newBlockID))
		updated = append(updated, raw[:loc[3]]...)
		updated = append(updated, []byte(strconv.Quote(newBlockID))...)
		updated = append(updated, raw[loc[1]:]...)
	} else {
		// No blockId key present: fall back to a structural rewrite (order not
		// preserved, but the file is re-emitted as valid indented JSON).
		var generic map[string]any
		if err := json.Unmarshal(raw, &generic); err != nil {
			return invalidJSON(raw, err)
		}
		generic["blockId"] = newBlockID
		b, err := json.MarshalIndent(generic, "", "  ")
		if err != nil {
			return err
		}
		updated = append(b, '\n')
	}

	// Never write something that no longer parses.
	if !json.Valid(updated) {
		return fmt.Errorf("refusing to write %s: result is not valid JSON after setting blockId=%q", Filename, newBlockID)
	}

	tmp, err := os.CreateTemp(dir, ".block.manifest.*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(updated); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(tmpName, p)
}

// LoadRaw reads the manifest and returns the decoded generic value (for schema
// validation) plus the parsed struct.
func LoadRaw(dir string) (any, *Manifest, error) {
	p := Path(dir)
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("no %s found in %s — is this an App project? run `civitai app create` to create one", Filename, dir)
		}
		return nil, nil, err
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, nil, invalidJSON(raw, err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, nil, invalidJSON(raw, err)
	}
	return generic, &m, nil
}
