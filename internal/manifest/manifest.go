// Package manifest holds the App manifest filename constant and a
// lightweight reader for the fields the CLI needs (slug/version/name).
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
			return nil, fmt.Errorf("no %s found in %s — is this an App project? run `civitai app init` to create one", Filename, dir)
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
			return nil, nil, fmt.Errorf("no %s found in %s — is this an App project? run `civitai app init` to create one", Filename, dir)
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
