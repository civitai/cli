// Package manifest holds the App Block manifest filename constant and a
// lightweight reader for the fields the CLI needs (slug/version/name).
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Filename is the one mandatory file in an App Block, at the project root.
// The server matches this exact path inside the ZIP (no nesting).
const Filename = "block.manifest.json"

// Manifest is a partial view of block.manifest.json — only the fields the CLI
// reads directly. Full validation is done against the JSON Schema, not this
// struct, so unknown fields are intentionally ignored here.
type Manifest struct {
	BlockID      string `json:"blockId"`
	Version      string `json:"version"`
	Name         string `json:"name"`
	BuildCommand string `json:"buildCommand"`
	OutputDir    string `json:"outputDir"`
}

// Path returns the manifest path for a project directory.
func Path(dir string) string { return filepath.Join(dir, Filename) }

// Load reads and JSON-parses the manifest in dir.
func Load(dir string) (*Manifest, error) {
	p := Path(dir)
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no %s found in %s — is this an App Block project? run `civitai app init` to create one", Filename, dir)
		}
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", Filename, err)
	}
	return &m, nil
}

// LoadRaw reads the manifest and returns the decoded generic value (for schema
// validation) plus the parsed struct.
func LoadRaw(dir string) (any, *Manifest, error) {
	p := Path(dir)
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("no %s found in %s — is this an App Block project? run `civitai app init` to create one", Filename, dir)
		}
		return nil, nil, err
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, nil, fmt.Errorf("%s is not valid JSON: %w", Filename, err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, nil, fmt.Errorf("%s is not valid JSON: %w", Filename, err)
	}
	return generic, &m, nil
}
