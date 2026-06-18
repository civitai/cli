package cli

import _ "embed"

// SchemaJSON is the vendored App Block manifest JSON Schema, embedded so the
// CLI validates against the same contract it ships. The file is the canonical
// copy intended to also be published server-side (see README).
//
//go:embed schema/app-block.manifest.schema.json
var SchemaJSON []byte
