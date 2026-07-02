package cli

import "embed"

// SchemaJSON is the vendored App manifest JSON Schema, embedded so the
// CLI validates against the same contract it ships. The file is the canonical
// copy intended to also be published server-side (see README).
//
//go:embed schema/app-block.manifest.schema.json
var SchemaJSON []byte

// ExamplesFS holds the real example manifests (copied from the shipping
// civitai-block-* apps). They are embedded so the validate test can assert the
// "examples validate clean" claim against the same files the README points to.
//
//go:embed examples/*.block.manifest.json
var ExamplesFS embed.FS
