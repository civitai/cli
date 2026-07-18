package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ── type-aware folder routing (`download --layout`) ──────────────────────────
//
// A model VERSION can bundle files of DIFFERENT types (e.g. a checkpoint that
// ships a companion VAE). Dumping them all into one directory (`--all --out-dir
// X`) mis-files the VAE into the checkpoint folder and pollutes the app's model
// dropdown. `--layout` routes each downloaded file into the correct subfolder
// for the target app, keyed by the file/model TYPE.
//
// Folder maps below are the documented defaults for each app:
//
//   AUTOMATIC1111 / Forge (relative to the webui root, i.e. --root):
//     Checkpoint      → models/Stable-diffusion
//     VAE             → models/VAE
//     LORA/LoCon/DoRA → models/Lora
//     TextualInversion→ embeddings
//     Hypernetwork    → models/hypernetworks
//     Controlnet      → models/ControlNet
//     Upscaler        → models/ESRGAN
//   (sources: AUTOMATIC1111/stable-diffusion-webui wiki "Command-Line-Arguments-
//    and-Settings"; the sd-webui-controlnet extension's models/ControlNet default)
//
//   ComfyUI (relative to the ComfyUI root, i.e. --root):
//     Checkpoint      → models/checkpoints
//     VAE             → models/vae
//     LORA/LoCon/DoRA → models/loras
//     TextualInversion→ models/embeddings
//     Hypernetwork    → models/hypernetworks
//     Controlnet      → models/controlnet
//     Upscaler        → models/upscale_models
//   (source: docs.comfy.org "Core Concepts › Models" — the models/ subfolders)
//
// An unmapped type (Poses, Wildcards, Workflows, Archive, Training Data, Config,
// Other, or anything unrecognized) is routed to --root itself with a stderr note
// rather than being silently misplaced.

// resourceCategory is the normalized routing bucket a file belongs to.
type resourceCategory int

const (
	catUnknown resourceCategory = iota
	catCheckpoint
	catVAE
	catLora
	catEmbedding
	catHypernetwork
	catControlnet
	catUpscaler
)

// layoutFolders maps a layout name → per-category subfolder (relative to --root).
// A category absent from the inner map (or catUnknown) routes to --root.
var layoutFolders = map[string]map[resourceCategory]string{
	"a1111": {
		catCheckpoint:   filepath.Join("models", "Stable-diffusion"),
		catVAE:          filepath.Join("models", "VAE"),
		catLora:         filepath.Join("models", "Lora"),
		catEmbedding:    "embeddings",
		catHypernetwork: filepath.Join("models", "hypernetworks"),
		catControlnet:   filepath.Join("models", "ControlNet"),
		catUpscaler:     filepath.Join("models", "ESRGAN"),
	},
	"comfyui": {
		catCheckpoint:   filepath.Join("models", "checkpoints"),
		catVAE:          filepath.Join("models", "vae"),
		catLora:         filepath.Join("models", "loras"),
		catEmbedding:    filepath.Join("models", "embeddings"),
		catHypernetwork: filepath.Join("models", "hypernetworks"),
		catControlnet:   filepath.Join("models", "controlnet"),
		catUpscaler:     filepath.Join("models", "upscale_models"),
	},
}

// knownLayouts lists the accepted --layout values (for validation + help).
var knownLayouts = []string{"a1111", "comfyui"}

// validLayout reports whether name is a supported layout.
func validLayout(name string) bool {
	_, ok := layoutFolders[name]
	return ok
}

// categoryForModelType maps a Civitai MODEL type (Checkpoint, LORA, …) to its
// routing bucket. Unrecognized types return catUnknown.
func categoryForModelType(modelType string) resourceCategory {
	switch strings.ToLower(strings.TrimSpace(modelType)) {
	case "checkpoint":
		return catCheckpoint
	case "vae":
		return catVAE
	case "lora", "locon", "dora", "lycoris":
		return catLora
	case "textualinversion", "embedding", "aestheticgradient":
		return catEmbedding
	case "hypernetwork":
		return catHypernetwork
	case "controlnet":
		return catControlnet
	case "upscaler":
		return catUpscaler
	default:
		return catUnknown
	}
}

// fileCategory determines the routing bucket for a single file. A file whose
// own type is an ancillary type with a distinct home (VAE) routes by that type —
// this is the checkpoint-with-bundled-VAE fix. The main weights file (type
// "Model") and files with no type route by the PARENT model type instead. Any
// other explicit ancillary type (Archive, Training Data, Config, …) has no model
// folder and returns catUnknown (→ routed to --root with a note).
func fileCategory(fileType, modelType string) resourceCategory {
	switch strings.ToLower(strings.TrimSpace(fileType)) {
	case "vae":
		return catVAE
	case "model", "":
		return categoryForModelType(modelType)
	default:
		// Explicit non-weights deliverable with no standard model folder.
		return catUnknown
	}
}

// routeDir returns the destination directory for a file under a layout, plus a
// stderr note when the file's type is unmapped and it falls back to --root. root
// defaults to "." when empty.
func routeDir(layout, root, fileType, modelType, fileName string) (dir, note string) {
	if root == "" {
		root = "."
	}
	cat := fileCategory(fileType, modelType)
	sub, ok := layoutFolders[layout][cat]
	if !ok || cat == catUnknown {
		t := strings.TrimSpace(fileType)
		if t == "" {
			t = "(none)"
		}
		return root, fmt.Sprintf("%s: type %q has no %s folder — writing to --root %q", fileName, t, layout, root)
	}
	return filepath.Join(root, sub), ""
}

// mixedTypeWarning builds the one-line warning printed when `--all` (without
// `--layout`) would place files of DIFFERING known types into one directory —
// the mis-file footgun. It returns "" when the set is single-typed (≤1 distinct
// known category) so a homogeneous download stays quiet.
//
// The REFERENCE category is the primary/weights category — never merely
// files[0]'s. If an ancillary file (e.g. a bundled VAE) is listed first, using
// files[0] as the reference would perversely name the actual weights file as the
// "mis-filed" one. Instead the reference is: the weights category of the primary
// file if one is known, else the most common known category (weights typically
// dominate), with ties broken by first appearance. The off-type files named are
// those whose known category differs from that reference.
func mixedTypeWarning(files []fileTypeInfo) string {
	// Collect distinct KNOWN categories + their counts; unknown types don't count
	// as a mismatch (we can't confidently say where they'd go).
	counts := map[resourceCategory]int{}
	order := []resourceCategory{}
	for _, f := range files {
		if c := fileCategory(f.fileType, f.modelType); c != catUnknown {
			if counts[c] == 0 {
				order = append(order, c)
			}
			counts[c]++
		}
	}
	if len(counts) < 2 {
		return ""
	}
	// Reference category = the primary file's known category if any, else the
	// most common known category (ties → first appearance).
	ref := catUnknown
	for _, f := range files {
		if f.primary {
			if c := fileCategory(f.fileType, f.modelType); c != catUnknown {
				ref = c
			}
			break
		}
	}
	if ref == catUnknown {
		best := 0
		for _, c := range order { // order preserves first-appearance for tie-breaks
			if counts[c] > best {
				best = counts[c]
				ref = c
			}
		}
	}
	var off []string
	for _, f := range files {
		c := fileCategory(f.fileType, f.modelType)
		if c != catUnknown && c != ref {
			off = append(off, f.name)
		}
	}
	if len(off) == 0 {
		return ""
	}
	return fmt.Sprintf("this version mixes file types — %s would be mis-filed alongside the others; pass --layout <a1111|comfyui> to route each file to its correct folder", strings.Join(off, ", "))
}

// fileTypeInfo is the minimal per-file view mixedTypeWarning needs.
type fileTypeInfo struct {
	name      string
	fileType  string
	modelType string
	primary   bool // the version's primary (weights) file — anchors the reference category
}
