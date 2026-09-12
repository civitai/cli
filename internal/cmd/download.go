package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// downloadExampleVersionID is the model-VERSION id every `civitai download`
// example uses as a BARE POSITIONAL id — in this command's help, in the root
// command's help, and in the README.
//
// 🔴 It must be a number that is a valid VERSION id and NOT also a valid MODEL
// id. A bare positional that is BOTH makes `download` stop and refuse to guess
// (exit 2, see resolveDownloadVersion), so an ambiguous id turns the first
// command a new user copy-pastes into an error. That is issue #227: the shipped
// example was 128713, which is DreamShaper's v8 VERSION id *and* the MODEL id of
// an unrelated NSFW-titled model, so the error message also sent the reader
// there. `--version 128713` / `--model 4384` stay fine — they name the kind
// explicitly and can never be ambiguous, which is why they are still used below.
//
// Verified against civitai.com on 2026-08-06 (redo this before changing it):
//
//	GET /api/v1/model-versions/691639 -> 200  (FLUX "Dev", model 618692)
//	GET /api/v1/models/691639         -> 404  (so the bare id cannot be ambiguous)
//	civitai download 691639 --dry-run -> rc=0, plans flux_dev.safetensors
const downloadExampleVersionID = "691639"

// unambiguousExampleIDs is the set of ids an example is allowed to pass as a
// BARE POSITIONAL to `civitai download`. The invariant is NOT "must be a version
// id" — it is "must not be BOTH", because only a number that resolves as a model
// id *and* a version id triggers the refuse-to-guess stop. A model-id-only entry
// is fine: `download` auto-resolves it to that model's default version and says
// so. Values record what each one resolved as when it was verified.
//
// Adding an id here is a claim about the LIVE API, not a formatting choice.
// Verify both routes before you add one — /api/v1/models/<id> AND
// /api/v1/model-versions/<id> — and reject any id where both answer 200.
//
// 🔴 The keys are LITERALS, never `downloadExampleVersionID`. Keying the headline
// entry by the constant made the allowlist self-fulfilling: repointing the
// constant at any id moved the key with it, so the membership check passed by
// construction. Measured — that mutant SURVIVED, and only the separate hardcoded
// -128713 denylist caught the one case; repointing the constant at a DIFFERENT
// ambiguous id (403131) was green. A literal key forces a human to re-verify.
var unambiguousExampleIDs = map[string]string{
	"691639": `version only (200) — FLUX "Dev", model 618692`,
	// Ships a Model *and* a distinctly-named VAE, which is what makes the
	// README's `--file vae` / `--all` / `--layout` fan-out examples runnable.
	// 691639's two files share a name (that is the point of the `--file <id>`
	// example), so it cannot demonstrate those.
	"290640": `version only (200) — Pony Diffusion V6 XL "V6", model 257749`,
	// A MODEL id on purpose: it is what the `download` help uses to show that
	// pasting a model id from `models search` auto-resolves to the default
	// version. Verified 2026-08-06: `civitai download 4384 --dry-run` prints
	// "note: 4384 is a model id — downloading its default version 128713", rc=0.
	"4384": `model only (200); /model-versions/4384 404s — DreamShaper`,
}

// downloadOpts holds the resolved flag values for `civitai download`.
type downloadOpts struct {
	modelID  string // --model (mutually exclusive with the positional version id)
	version  string // --version: explicit version id (skips the ambiguous model-id gate)
	yes      bool   // --yes: proceed past the ambiguous model-id stop (download the version as typed)
	file     string // --file: select one file by name (exact | unique substring)
	all      bool   // --all: download every file in the version
	out      string // --out: target path (single-file only)
	outDir   string // --out-dir: directory for server-named files
	layout   string // --layout: type-aware folder routing (a1111|comfyui)
	root     string // --root: base dir for --layout routing (default ".")
	forBase  string // --for-base: warn on a confident base-model family mismatch
	noVerify bool   // --no-verify: skip SHA256 verification
	force    bool   // --force: re-download even if the target already exists
	anon     bool   // --anon: force an anonymous request
	dryRun   bool   // --dry-run: print the resolved plan and exit without transferring

	// modelType is the PARENT model type (Checkpoint, LORA, …), resolved from the
	// version detail at runtime and used by --layout routing. Not a flag.
	modelType string
}

func newDownloadCmd() *cobra.Command {
	o := &downloadOpts{}
	cmd := &cobra.Command{
		Use:   "download [version-id]",
		Short: "Download a model version's file(s)",
		Long: `Download the file(s) of a model VERSION from Civitai.

Identify the version deterministically by its numeric version id:

  civitai download 691639

…or resolve a model's default (first published) version with --model:

  civitai download --model 4384

The positional id is normally a model-VERSION id, but 'civitai models search'
and 'civitai models get' list MODEL ids — so handing a model id as the positional
(e.g. 'civitai download 4384') just works: the CLI recognizes it's a model id and
downloads that model's default version (printing a note that it did). When a
pasted number is BOTH a valid model id and a valid version id (common for low/mid
numbers), the CLI STOPS and asks you to disambiguate rather than silently
downloading an unrelated model's version — re-run with --model <id> (the model's
default version) or --version <id> (that version as-is). Use --version to name a
version id explicitly and skip that stop; --yes proceeds on the version
interpretation and echoes exactly which version it is downloading.

Use --dry-run to print the resolved plan (files, sizes, SHA256, target paths,
and whether auth is required) without transferring anything.

By default the version's PRIMARY file is downloaded into the current directory
under its server-provided name. Any file type downloads — model weights, but
also non-weights deliverables like a "Workflows" model's Archive, training data,
or other artifacts. Use --file to pick a specific file, or --all to download
every file. Downloads stream to "<target>.part" and are renamed into place only
on success, so an interrupted run never leaves a truncated final file.

Selecting one of two same-named files: a version can ship two files with the
SAME name (e.g. an fp16 and an fp8 both named flux_dev.safetensors). --file
accepts a numeric FILE ID (the version's files[].id) as well as a name, so you
can pick exactly one — the ids are shown by --dry-run and in the error you get
if a name is ambiguous. --all refuses to run when two selected files would
resolve to the same on-disk path (which would silently overwrite one), listing
the colliding files with their ids so you can --file <id> the one you want.

Authentication: most model files require a token to download — a gated file
requires authentication (it 401s without a token), but some public files
download with no token at all. Run 'civitai login' if a download 401s. Your
stored login token or CIVITAI_TOKEN is sent automatically. The read/search
commands (models, model-versions, articles, images, …) always work anonymously.

Folder routing: pass --layout <a1111|comfyui> (with an optional --root <dir>,
default ".") to write each file into the correct subfolder for that app, routed
by the file/model type — so --all fans a bundled VAE into the VAE folder
instead of polluting the checkpoint folder. --layout is mutually exclusive with
--out/--out-dir.

Compatibility: --for-base "<baseModel>" warns on stderr when the version's base
model is in a confidently different family than your target (e.g. an SD 1.5
embedding for an SDXL model). The version's base model is always shown.

Integrity: the streamed bytes are verified against the file's SHA256 by default
(--no-verify to skip; a file with no published SHA256 is downloaded with a
warning). A hash mismatch deletes the partial file and fails. SHA256 verifies
INTEGRITY (the bytes match what the API advertised), NOT authenticity — a
compromised source that advertises a matching hash for malicious bytes can't be
detected by the hash alone. Pickle/executable (.ckpt/.pt/.pth/.bin/.pickle/.pkl)
and archive (.zip/.tar/.tar.gz/.tgz/.rar/.7z) files can execute code when loaded;
the CLI notes this on stderr. Only download models from creators you trust.`,
		Example: `  civitai download 691639
  civitai download --version 128713                # force a version id (skips the ambiguous-id stop)
  civitai download --model 4384 --out ./dreamshaper.safetensors
  civitai download 290640 --file vae --out-dir ./models
  civitai download 691639 --file 1234567          # pick one of two same-named files by id
  civitai download 290640 --all --out-dir ./models
  civitai download 290640 --all --layout comfyui --root ~/ComfyUI
  civitai download 691639 --layout a1111 --for-base "SDXL 1.0"`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDownload(cmd, args, o)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.modelID, "model", "", "resolve+download a MODEL's default (first published) version instead of a version id")
	f.StringVar(&o.version, "version", "", "download this model-VERSION id explicitly (skips the ambiguous model-id safety stop the bare positional id triggers)")
	f.BoolVar(&o.yes, "yes", false, "proceed past the ambiguous-id safety stop (a bare id that is BOTH a model id and a version id): download the version as typed")
	f.StringVar(&o.file, "file", "", "select one file by numeric file id, or by name (exact, else a unique case-insensitive substring); use the id to pick one of two same-named files")
	f.BoolVar(&o.all, "all", false, "download every file in the version (refuses if two files would overwrite the same path — pick one with --file <id>)")
	f.StringVar(&o.out, "out", "", "target file path (single-file only; mutually exclusive with --all/--out-dir)")
	f.StringVar(&o.outDir, "out-dir", "", "directory to write server-named file(s) into (created if needed)")
	f.StringVar(&o.layout, "layout", "", "route each file into its type's subfolder for an app (a1111|comfyui); mutually exclusive with --out/--out-dir")
	f.StringVar(&o.root, "root", "", "base directory for --layout routing (default \".\"; only applies with --layout)")
	f.StringVar(&o.forBase, "for-base", "", "warn if the version's base model is a confidently different family than this target (e.g. \"SDXL 1.0\")")
	f.BoolVar(&o.noVerify, "no-verify", false, "skip SHA256 verification of the downloaded bytes")
	f.BoolVar(&o.force, "force", false, "re-download even if the target file already exists")
	f.BoolVar(&o.anon, "anon", false, "force an anonymous request (ignore any stored login token); NOTE: most downloads 401 without a token — --anon is meaningful for read commands, not downloads")
	f.BoolVar(&o.dryRun, "dry-run", false, "print the resolved download plan (files, sizes, hashes, targets) and exit without downloading anything")
	return cmd
}

func runDownload(cmd *cobra.Command, args []string, o *downloadOpts) error {
	// Resolve the target version id (exactly one of positional / --model) and
	// validate the flag combinations up front, before any network work.
	positional, err := resolveDownloadTarget(args, o)
	if err != nil {
		return err
	}
	if err := validateDownloadFlags(o); err != nil {
		return err
	}

	client, baseURL, err := newReader(&readOpts{anon: o.anon})
	if err != nil {
		return err
	}
	// Cancel the whole download on Ctrl-C (SIGINT): the signal-bound context is
	// threaded into every request + io.Copy, so an interrupted transfer unblocks
	// with a cancellation error and downloadOne's cleanup removes the partial
	// ".part" file rather than leaving it behind.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	out := cmd.OutOrStdout()
	errW := cmd.ErrOrStderr()

	// Resolve the version detail to download, handling the three bare-positional
	// shapes (version-only / model-id auto-resolve / ambiguous stop). `note` is a
	// user-facing stderr line describing any automatic resolution.
	v, note, err := resolveDownloadVersion(ctx, client, positional, o)
	if err != nil {
		return err
	}
	if note != "" {
		fmt.Fprintln(errW, note)
	}
	// Parent model type (Checkpoint, LORA, …) drives --layout routing of the
	// weights file; the version detail embeds it under `model`.
	if v.Model != nil {
		o.modelType = v.Model.Type
	}

	selected, err := selectFiles(v.Files, o)
	if err != nil {
		return err
	}

	// Guard against a silent same-target overwrite BEFORE any transfer (and before
	// the --dry-run plan): when multiple files are selected, two resolving to the
	// same on-disk path would clobber each other (data loss). Fail-safe here so no
	// bytes ever move for an unsafe plan.
	if len(selected) > 1 {
		if err := checkTargetCollisions(selected, o); err != nil {
			return err
		}
	}

	// Always surface the version's base model, and (with --for-base) warn on a
	// confident cross-family mismatch before any bytes move.
	reportBaseModel(out, errW, v.BaseModel, o.forBase)

	// --dry-run: print the resolved plan and exit 0, transferring nothing and
	// creating no file (not even a ".part").
	if o.dryRun {
		return printDownloadPlan(out, selected, o, baseURL)
	}

	// Without --layout, warn when `--all` would mis-file differing types into one
	// directory (the footgun --layout exists to fix).
	warnMixedTypes(errW, selected, o)

	downloaded, err := downloadSelected(ctx, client, out, errW, selected, o)
	if err != nil {
		return err
	}
	if downloaded == 0 && o.all {
		// Everything was already present on disk.
		fmt.Fprintln(errW, ui.For(errW).Warn("no files were downloaded (all were already present)"))
	}
	return nil
}

// resolveDownloadTarget returns the positional version id from args after
// enforcing that EXACTLY one of the positional id / --model / --version is
// supplied. Its errors are invocation mistakes → tagged as usage errors (exit 2).
func resolveDownloadTarget(args []string, o *downloadOpts) (string, error) {
	var positional string
	if len(args) == 1 {
		positional = strings.TrimSpace(args[0])
	}
	n := 0
	for _, set := range []bool{positional != "", o.modelID != "", o.version != ""} {
		if set {
			n++
		}
	}
	if n > 1 {
		return "", asUsageError(fmt.Errorf("provide exactly ONE of a positional version id, --model <model-id>, or --version <version-id> — not both/several"))
	}
	if n == 0 {
		return "", asUsageError(fmt.Errorf("provide a model-version id (civitai download <version-id>), --model <model-id>, or --version <version-id>"))
	}
	return positional, nil
}

// validateDownloadFlags rejects incompatible flag combinations before any
// network work: --all/--file, --out/--out-dir, --out/--all, and the --layout
// exclusions (a valid layout, no --out, no --out-dir; --root only with --layout).
// Every failure here is a local invocation mistake → tagged as a usage error
// (exit 2), matching `model-versions get`.
func validateDownloadFlags(o *downloadOpts) error {
	if o.all && o.file != "" {
		return asUsageError(fmt.Errorf("--all and --file are mutually exclusive"))
	}
	if o.out != "" && o.outDir != "" {
		return asUsageError(fmt.Errorf("--out (a file path) and --out-dir (a directory) are mutually exclusive"))
	}
	if o.out != "" && o.all {
		return asUsageError(fmt.Errorf("--out sets a single file path and can't be combined with --all — use --out-dir"))
	}
	if o.layout != "" {
		if !validLayout(o.layout) {
			return asUsageError(fmt.Errorf("--layout must be one of %s, got %q", strings.Join(knownLayouts, "|"), o.layout))
		}
		if o.out != "" {
			return asUsageError(fmt.Errorf("--layout routes files into type folders and can't be combined with --out (an explicit single path)"))
		}
		if o.outDir != "" {
			return asUsageError(fmt.Errorf("--layout routes files under --root and can't be combined with --out-dir"))
		}
	} else if o.root != "" {
		return asUsageError(fmt.Errorf("--root only applies with --layout"))
	}
	return nil
}

// reportBaseModel prints the version's base model and, with --for-base set,
// warns on stderr about a confident cross-family mismatch.
func reportBaseModel(out, errW io.Writer, baseModel, forBase string) {
	fmt.Fprintf(out, "base model: %s\n", dashIfEmpty(safeTerm(baseModel)))
	if forBase != "" {
		if w := baseModelWarning(baseModel, forBase); w != "" {
			fmt.Fprintln(errW, ui.For(errW).Warn(safeTerm(w)))
		}
	}
}

// warnMixedTypes warns (stderr) when --all without --layout would mis-file
// differing file types into a single directory.
func warnMixedTypes(errW io.Writer, selected []civitai.ModelVersionFile, o *downloadOpts) {
	if o.all && o.layout == "" {
		if w := mixedTypeWarning(fileTypeInfos(selected, o.modelType)); w != "" {
			fmt.Fprintln(errW, ui.For(errW).Warn(safeTerm(w)))
		}
	}
}

// downloadSelected downloads each selected file — resolving its per-file target
// (surfacing any routing note on stderr) then transferring it — and returns how
// many files were actually transferred (an already-present, skipped file does
// not count). It stops at the first error.
func downloadSelected(ctx context.Context, dl civitai.Downloader, out, errW io.Writer, selected []civitai.ModelVersionFile, o *downloadOpts) (int, error) {
	downloaded := 0
	for i := range selected {
		f := selected[i]
		target, note, err := targetPath(f, o)
		if err != nil {
			return downloaded, err
		}
		if note != "" {
			fmt.Fprintln(errW, ui.For(errW).Warn(safeTerm(note)))
		}
		skipped, err := downloadOne(ctx, dl, out, errW, f, target, o)
		if err != nil {
			return downloaded, err
		}
		if !skipped {
			downloaded++
		}
	}
	return downloaded, nil
}

// resolveDownloadVersion loads the model-version detail to download from the
// resolved flags/positional, returning it plus an optional user-facing note
// (stderr) describing any automatic resolution.
//
//   - --version <id> / --model <id> resolve deterministically (a --model resolves
//     the model's default (first published) version) — no ambiguity handling.
//   - A bare positional id is classified by looking it up as BOTH a version and a
//     model, then handled by resolveBarePositionalVersion.
func resolveDownloadVersion(ctx context.Context, client civitai.Reader, positional string, o *downloadOpts) (*civitai.ModelVersionDetail, string, error) {
	if o.version != "" || o.modelID != "" {
		versionID, err := resolveExplicitVersionID(ctx, client, o)
		if err != nil {
			return nil, "", err
		}
		v, _, err := client.GetModelVersion(ctx, versionID)
		if err != nil {
			return nil, "", err
		}
		return v, "", nil
	}
	return resolveBarePositionalVersion(ctx, client, positional, o)
}

// resolveExplicitVersionID returns the version id for the explicit --version /
// --model paths. For --model, the API returns modelVersions default/latest-first,
// so the model's default version is modelVersions[0]; its primary file downloads
// regardless of file type (a "Workflows" model's Archive, training data, or plain
// weights all download the same way).
func resolveExplicitVersionID(ctx context.Context, client civitai.Reader, o *downloadOpts) (string, error) {
	if o.version != "" {
		if _, err := strconv.Atoi(o.version); err != nil {
			return "", asUsageError(fmt.Errorf("--version id must be an integer, got %q", o.version))
		}
		return o.version, nil
	}
	if _, err := strconv.Atoi(o.modelID); err != nil {
		return "", asUsageError(fmt.Errorf("--model id must be an integer, got %q", o.modelID))
	}
	m, _, err := client.GetModel(ctx, o.modelID)
	if err != nil {
		return "", err
	}
	if len(m.ModelVersions) == 0 {
		return "", fmt.Errorf("model %s has no published versions to download", o.modelID)
	}
	return strconv.Itoa(m.ModelVersions[0].ID), nil
}

// resolveBarePositionalVersion resolves a bare positional id into the version to
// download. `civitai models search` lists MODEL ids, so a user's natural
// `civitai download <id>` frequently pastes a MODEL id into the version-id
// positional. Three shapes are distinguished by looking the id up as BOTH a
// version and a model:
//
//   - version-only (valid version, NOT a model) → download that version, as today.
//   - model-only (valid model, NOT a version) → auto-resolve + download the model's
//     default version, with a note. This turns the common search→download flow into
//     a success instead of a hint-wall.
//   - ambiguous (BOTH a model AND a version) → STOP for disambiguation (exit 2), so
//     the CLI never silently downloads an UNRELATED model's version and launders it
//     with a reassuring "SHA256 verified". --yes proceeds (download the version as
//     typed) but ECHOES exactly which interpretation it chose.
//
// A MODEL id is only "confirmable" when the model lookup echoes back the queried
// id (the real REST API always echoes it — a mismatch means the lookup didn't
// actually resolve that id).
func resolveBarePositionalVersion(ctx context.Context, client civitai.Reader, positional string, o *downloadOpts) (*civitai.ModelVersionDetail, string, error) {
	n, err := strconv.Atoi(positional)
	if err != nil {
		return nil, "", asUsageError(fmt.Errorf("model-version id must be an integer, got %q — pass a numeric model-version id, or find one with `civitai models search`", positional))
	}

	v, _, verErr := client.GetModelVersion(ctx, positional)
	if verErr != nil {
		// Not a valid version. If it IS a confirmable MODEL id, auto-resolve its
		// default version instead of dead-ending on the version 404 — the common
		// `models search` → `download <model-id>` flow. Only a genuine 404 warrants
		// the second lookup; any other failure (auth/network/rate-limit) is the real
		// problem and passes through untouched.
		if errors.Is(verErr, civitai.ErrNotFound) {
			if dv, note, handled, aerr := autoResolveModelDefault(ctx, client, positional, n); handled {
				return dv, note, aerr
			}
		}
		return nil, "", verErr
	}

	// The id IS a valid version. Is it ALSO a confirmable model id? If so it's the
	// ambiguity footgun.
	m := lookupConfirmedModel(ctx, client, positional, n)
	if m == nil {
		return v, "", nil // version-only — unambiguous, download as today.
	}
	if o.yes {
		// --yes bypasses the stop; echo EXACTLY which interpretation it chose so a
		// reflexive --yes can't silently grab an unrelated model.
		return v, ambiguousYesNote(positional, v, m), nil
	}
	return nil, "", ambiguousStopError(positional, v, m)
}

// lookupConfirmedModel returns the model for id only when it resolves as a model
// whose returned id ECHOES the queried id (a mismatch means the lookup didn't
// actually resolve that id). Any lookup failure or id mismatch returns nil ("not
// a confirmable model id").
func lookupConfirmedModel(ctx context.Context, client civitai.Reader, id string, n int) *civitai.ModelDetail {
	m, _, err := client.GetModel(ctx, id)
	if err != nil || m == nil || m.ID != n {
		return nil
	}
	return m
}

// autoResolveModelDefault handles a bare positional that is a valid MODEL id but
// NOT a valid version id: it resolves the model's default (first published)
// version and returns its detail plus a one-line note. handled=false means the id
// is not a confirmable model id → the caller keeps the original version error.
func autoResolveModelDefault(ctx context.Context, client civitai.Reader, id string, n int) (v *civitai.ModelVersionDetail, note string, handled bool, err error) {
	m := lookupConfirmedModel(ctx, client, id, n)
	if m == nil {
		return nil, "", false, nil
	}
	if len(m.ModelVersions) == 0 {
		return nil, "", true, fmt.Errorf("model %s has no published versions to download", id)
	}
	defVerID := strconv.Itoa(m.ModelVersions[0].ID)
	dv, _, gerr := client.GetModelVersion(ctx, defVerID)
	if gerr != nil {
		return nil, "", true, gerr
	}
	return dv, fmt.Sprintf("note: %s is a model id — downloading its default version %s", id, defVerID), true, nil
}

// ambiguousYesNote is the --yes echo for an ambiguous id: it spells out EXACTLY
// which interpretation was chosen (the VERSION, of its own parent model) and how
// to get the OTHER one (the model whose id was pasted), so a reflexive --yes can't
// silently grab an unrelated model.
func ambiguousYesNote(id string, v *civitai.ModelVersionDetail, m *civitai.ModelDetail) string {
	parent := ""
	if v != nil && v.Model != nil {
		parent = v.Model.Name
	}
	return fmt.Sprintf(
		"--yes: downloading VERSION %s %q (use --model %s for the model %q instead)",
		id, safeTerm(dashIfEmpty(parent)), id, safeTerm(dashIfEmpty(m.Name)))
}

// ambiguousStopError is the core footgun guard's usage error (exit 2): a bare
// positional id that is BOTH a valid model id and a valid version id STOPs and
// spells out both interpretations and how to pick one.
func ambiguousStopError(id string, v *civitai.ModelVersionDetail, m *civitai.ModelDetail) error {
	parent := ""
	if v != nil && v.Model != nil {
		parent = v.Model.Name
	}
	return asUsageError(fmt.Errorf(
		"%s is ambiguous — it's both model %q and version %s (of model %q).\n"+
			"To download the model's default version:  civitai download --model %s\n"+
			"To download version %s as-is:             re-run with --version %s (or --yes)",
		id, safeTerm(m.Name), id, safeTerm(dashIfEmpty(parent)), id, id, id))
}

// printDownloadPlan renders the --dry-run plan: for each selected file its name,
// size, SHA256, resolved target path, and whether authentication will be
// required. It writes NOTHING to disk.
func printDownloadPlan(out io.Writer, files []civitai.ModelVersionFile, o *downloadOpts, baseURL string) error {
	fmt.Fprintf(out, "Dry run — planning %d file(s); nothing will be downloaded.\n", len(files))
	// Surface the mis-file warning in the plan too when it applies.
	if o.all && o.layout == "" {
		if w := mixedTypeWarning(fileTypeInfos(files, o.modelType)); w != "" {
			fmt.Fprintf(out, "  note:   %s\n", safeTerm(w))
		}
	}
	for i := range files {
		f := files[i]
		fmt.Fprintf(out, "\n%s\n", safeTerm(f.Name))
		fmt.Fprintf(out, "  size:   %s\n", humanBytes(int64(f.SizeKB*1024)))
		sha := strings.TrimSpace(f.Hashes.SHA256)
		if sha == "" {
			sha = "(none published)"
		}
		fmt.Fprintf(out, "  sha256: %s\n", safeTerm(sha))
		if target, note, terr := targetPath(f, o); terr != nil {
			fmt.Fprintf(out, "  target: (unresolved) — %v\n", terr)
		} else {
			fmt.Fprintf(out, "  target: %s\n", safeTerm(target))
			if note != "" {
				fmt.Fprintf(out, "  note:   %s\n", safeTerm(note))
			}
		}
		if civitai.DownloadNeedsAuth(f.DownloadURL, baseURL) {
			fmt.Fprintf(out, "  auth:   required\n")
		} else {
			fmt.Fprintf(out, "  auth:   not required\n")
		}
		fmt.Fprintf(out, "  status: would download\n")
	}
	return nil
}

// selectFiles resolves which file(s) of a version to download from the flags:
// --all → every file; --file → one file by exact name or a unique
// case-insensitive substring; default → the primary file (or the first file when
// none is flagged primary).
func selectFiles(files []civitai.ModelVersionFile, o *downloadOpts) ([]civitai.ModelVersionFile, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("this version has no downloadable files")
	}
	if o.all {
		return files, nil
	}
	if o.file != "" {
		// A bad --file value is an invocation mistake → usage error (exit 2).
		sel, err := selectOneFile(files, o.file)
		return sel, asUsageError(err)
	}
	primary := civitai.PrimaryFile(files)
	return []civitai.ModelVersionFile{*primary}, nil
}

// selectOneFile picks a single file for --file. Resolution order:
//
//  1. Numeric file id — an all-digits `want` that equals a file's `id` selects
//     exactly that file. This is the unambiguous way to pick ONE of two files
//     that share a name (a version can ship, e.g., an fp16 and an fp8 both named
//     flux_dev.safetensors): copy the numeric id shown in the collision error or
//     the --dry-run plan. Tried first so an id always wins; a real filename is
//     practically never bare digits.
//  2. Exact (case-insensitive) name — one match wins. TWO+ files can legitimately
//     share a name, so that's ambiguous: error and tell the user to pick by id.
//  3. A UNIQUE case-insensitive substring match.
//
// Ambiguous or no match is an error that lists the candidate files with their ids.
func selectOneFile(files []civitai.ModelVersionFile, want string) ([]civitai.ModelVersionFile, error) {
	// (1) Numeric file-id selection.
	if id, err := strconv.Atoi(strings.TrimSpace(want)); err == nil {
		for i := range files {
			if files[i].ID == id {
				return []civitai.ModelVersionFile{files[i]}, nil
			}
		}
	}
	// (2) Exact (case-insensitive) name match — collect ALL, since a name can be
	// shared by more than one file.
	var exact []civitai.ModelVersionFile
	for i := range files {
		if strings.EqualFold(files[i].Name, want) {
			exact = append(exact, files[i])
		}
	}
	if len(exact) == 1 {
		return exact[:1], nil
	}
	if len(exact) > 1 {
		return nil, fmt.Errorf("%q matches %d files that share this name — select one by its numeric file id with --file <id>:\n%s", want, len(exact), formatFileList(exact))
	}
	// (3) Unique case-insensitive substring match.
	var matches []civitai.ModelVersionFile
	lw := strings.ToLower(want)
	for i := range files {
		if strings.Contains(strings.ToLower(files[i].Name), lw) {
			matches = append(matches, files[i])
		}
	}
	switch len(matches) {
	case 1:
		return matches[:1], nil
	case 0:
		return nil, fmt.Errorf("no file matches %q. Available files:\n%s", want, formatFileList(files))
	default:
		return nil, fmt.Errorf("%q is ambiguous — it matches %d files. Be more specific (or use --file <id>):\n%s", want, len(matches), formatFileList(matches))
	}
}

// formatFileList renders a compact "  - [id N] name (type, size)" list for
// errors. The id is included so the user can copy it into --file <id> to
// disambiguate same-named files.
//
// The placeholder nests dashIfEmpty OUTSIDE safeTerm — civitai/cli#572. The
// other order asks "is the RAW type empty", which a type of one zero-width rune
// answers "no"; safeTerm then empties it and the line renders "(, 2.9 MiB)"
// with no placeholder at all. reportBaseModel already had this order; these two
// sites did not.
func formatFileList(files []civitai.ModelVersionFile) string {
	var b strings.Builder
	for i := range files {
		f := files[i]
		fmt.Fprintf(&b, "  - [id %d] %s (%s, %s)\n", f.ID, safeTerm(f.Name), dashIfEmpty(safeTerm(f.Type)), humanBytes(int64(f.SizeKB*1024)))
	}
	return strings.TrimRight(b.String(), "\n")
}

// checkTargetCollisions guards against a silent same-target overwrite: when more
// than one file is selected (--all, or --layout/--out-dir routing several files),
// two files that resolve to the SAME on-disk path would clobber each other —
// the second download overwrites the first with no warning and one file is lost.
// This bit Flux Dev version 691639, which ships an fp16 (22.2 GB) and an fp8
// (15.9 GB) file BOTH named flux_dev.safetensors → --all planned both to
// ./flux_dev.safetensors.
//
// It returns a fail-safe error listing every colliding group (with each file's
// numeric id + size to distinguish them) BEFORE anything is transferred, so no
// data is ever lost to a silent overwrite. Per-file targetPath errors are left
// for the normal per-file path to surface.
//
// 🔴 EVERY SERVER-ORIGIN FIELD IN THE LISTING IS safeTerm'd, AND IT WAS NOT
// UNTIL civitai/cli#566. The group line's `target` is mixed-origin (targetPath's
// doc comment states that, and names sanitising its print sites as the reason),
// and the per-file line is the LITERAL TWIN of formatFileList's — same fields,
// same shape — and that one was gated while this one was not. That is the defect
// shape #564's nonModelFileMarker fix closed: one renderer sanitising a field
// while its sibling does not. The stakes are higher here than on the ambiguity
// list, because this error is the ONLY thing standing between the user and a
// silent overwrite — a cursor escape in a file name can erase the sibling row
// the user is being asked to choose between.
//
// ⚠ RESIDUAL, STATED RATHER THAN IMPLIED: safeTerm removes terminal ESCAPES and
// the invisible class, NOT every forgery primitive. saferune deliberately keeps
// `\n` and `\t` (saferune.go: "Cc, minus \n and \t"), so a files[].name
// containing a newline still forges whole lines inside this refusal — the row
// the user is choosing between can be followed by attacker-written text at
// column zero. That class is repo-wide rather than this renderer's, it is
// strictly better than the raw state this replaced, and the single-line
// treatment for it is safeTermSingle; applying it here is civitai/cli#552's
// work, not this function's.
func checkTargetCollisions(files []civitai.ModelVersionFile, o *downloadOpts) error {
	byTarget := make(map[string][]civitai.ModelVersionFile)
	var order []string
	for i := range files {
		target, _, err := targetPath(files[i], o)
		if err != nil {
			// An unusable target for this file is reported later per-file; don't
			// mask it with a collision error here.
			continue
		}
		if _, seen := byTarget[target]; !seen {
			order = append(order, target)
		}
		byTarget[target] = append(byTarget[target], files[i])
	}

	var b strings.Builder
	groups := 0
	for _, target := range order {
		grp := byTarget[target]
		if len(grp) < 2 {
			continue
		}
		groups++
		fmt.Fprintf(&b, "  %s  ← %d files:\n", safeTerm(target), len(grp))
		for i := range grp {
			f := grp[i]
			// dashIfEmpty OUTSIDE safeTerm — see formatFileList (civitai/cli#572).
			fmt.Fprintf(&b, "      - [id %d] %s (%s, %s)\n", f.ID, safeTerm(f.Name), dashIfEmpty(safeTerm(f.Type)), humanBytes(int64(f.SizeKB*1024)))
		}
	}
	if groups == 0 {
		return nil
	}
	return fmt.Errorf("refusing to download: %d set(s) of files would be written to the same path and silently overwrite each other:\n%s\nPick ONE with --file <id> (the numeric file id shown above), or download them separately to distinct paths (e.g. one at a time with --out <path>)", groups, strings.TrimRight(b.String(), "\n"))
}

// targetPath computes the on-disk destination for a file given the flags:
// --out (verbatim path), else --layout routes under --root/<type-folder>, else
// --out-dir/<server-name>, else ./<server-name>. It also returns an optional
// stderr note (non-empty only under --layout when the file's type is unmapped
// and falls back to --root).
//
// For every server-named case (default, --out-dir, --layout) the API-supplied
// f.Name is reduced to its bare basename with filepath.Base BEFORE joining, so a
// hostile name like "/home/user/.bashrc" or "../../etc/foo" can never write
// outside the intended directory (path-traversal / arbitrary write). --out is
// the user's OWN explicit target and is used verbatim (they may legitimately
// pass a relative or absolute path). A basename that degenerates to ".", "/", or
// ".." (empty or all-slashes server name) is unusable and errors rather than
// writing to a junk path.
// 🔴 THE RETURNED PATH IS MIXED-ORIGIN, WHICH IS WHY ITS PRINT SITES SANITISE
// IT EVEN THOUGH #393 SAYS USER INPUT IS ECHOED EXACTLY. With `--out` it is the
// user's own bytes, verbatim; without it, it is built from
// `filepath.Base(f.Name)` — the SERVER's file name, which safeTerm's own
// docstring names as a forgery vector. One value, two origins, and the callers
// cannot tell them apart, so the safe treatment wins: printing it raw would let
// a hostile uploader forge the `Saved … (SHA256 verified)` line for a
// papercut's worth of fidelity. The cost, stated rather than hidden: an `--out`
// path containing an invisible rune is echoed without it, so a user copying
// that line gets a path that does not exist. Separating the two would mean
// threading the origin out of here; it is not worth it on this path, and the
// README says what the CLI actually does.
//
// 🔴 ITS REFUSAL IS A TERMINAL SURFACE TOO, AND `%q` IS NOT A SANITISER —
// civitai/cli#572. The unusable-filename error below echoes the RAW `f.Name`,
// and it is reachable with an arbitrary payload: `filepath.Base` of
// "<anything>/.." is "..", so a name can carry any prefix it likes into the
// message. It reaches a terminal twice — on stderr via downloadSelected (main.go
// prints err.Error() unfiltered) and on STDOUT via printDownloadPlan's
// "target: (unresolved) — %v", one line below a correctly-sanitised name.
//
// An earlier round shipped this ungated, reasoning that "%q already escapes
// every Cc/Cf; only the blank-but-graphic pair survives, which cannot move a
// cursor". That is wrong in the direction that matters, and it is wrong by
// MEASUREMENT, not by argument: strconv's quoting escapes what is not
// `IsPrint`, and Go's `IsPrint` admits every non-spacing mark and symbol. So
// `%q` escapes the Cc/Cf half of saferune's class and passes the rest THROUGH,
// RAW — U+034F (Mn), U+2800 (So), U+FE00 and U+180B (Mn), U+17B4 (Mn), and the
// Hangul fillers U+3164/U+115F (Lo). That is precisely the half saferune's
// package doc records the `Cf`-based first cut of #393 missing, and why the
// class was redrawn on Default_Ignorable_Code_Point. It is not a cursor vector
// and was never claimed to be one; it is the PADDING/FORGERY vector this repo
// already demonstrated — hardSplitOverlong's comment records a U+2800 run
// rendering as a table row.
//
// `%q` is KEPT as well as safeTerm, for two things safeTerm does not do. It
// DELIMITS — this refusal fires exactly when the basename degenerated, so the
// name is often empty, blank or all-slashes, and unquoted it would render as
// nothing at all. And it escapes `\n` and `\t`, which saferune deliberately
// KEEPS (see safeTermSingle), so quoting is what stops this particular message
// forging a line. Order matters: safeTerm runs FIRST, so `%q` never sees the
// invisible class and cannot be read as the thing removing it.
func targetPath(f civitai.ModelVersionFile, o *downloadOpts) (string, string, error) {
	if o.out != "" {
		return o.out, "", nil
	}
	base := filepath.Base(f.Name)
	if base == "." || base == ".." || base == string(filepath.Separator) || base == "/" {
		return "", "", fmt.Errorf("server returned an unusable filename %q; pass --out to set the output path", safeTerm(f.Name))
	}
	if o.layout != "" {
		dir, note := routeDir(o.layout, o.root, f.Type, o.modelType, base)
		return filepath.Join(dir, base), note, nil
	}
	if o.outDir != "" {
		return filepath.Join(o.outDir, base), "", nil
	}
	return base, "", nil
}

// fileTypeInfos projects the selected files into the minimal view the mis-file
// warning needs (name + file type + the shared parent model type).
func fileTypeInfos(files []civitai.ModelVersionFile, modelType string) []fileTypeInfo {
	infos := make([]fileTypeInfo, len(files))
	for i := range files {
		infos[i] = fileTypeInfo{name: files[i].Name, fileType: files[i].Type, modelType: modelType, primary: files[i].Primary}
	}
	return infos
}

// downloadOne downloads a single file to target, streaming to "<target>.part"
// and renaming on success. Returns skipped=true when an already-present target
// satisfied the idempotency check. Verification (SHA256, default on) deletes the
// partial file and errors on mismatch.
//
// 🔴 THE ERROR STRINGS ARE A TERMINAL SURFACE TOO, AND THEY WERE NOT GATED
// UNTIL civitai/cli#566. `cmd/civitai/main.go` prints err.Error() to stderr
// with no filter of its own, so every %s here reaches a terminal exactly like a
// rendered line does — which makes an unsanitised `f.Name` or `target` in one
// of them the same forgery vector as an unsanitised table cell, with a worse
// payload: the SHA256-mismatch string is the CLI ASSERTING AN INTEGRITY
// FAILURE, so a cursor escape in the uploader's file name can overwrite the
// word "mismatch" before the user reads it. `target` is mixed-origin for the
// reason targetPath's doc comment gives; it was already safeTerm'd on the
// `Saved …` line below and raw two returns above it.
//
// ⚠ RESIDUAL, STATED RATHER THAN IMPLIED: what these gates remove is terminal
// ESCAPES and the invisible class — not `\n`, which saferune deliberately keeps
// (saferune.go: "Cc, minus \n and \t"). A newline in files[].name therefore
// still forges whole lines inside the SHA256-mismatch and status errors. That is
// a repo-wide class with a repo-wide answer (safeTermSingle); closing it here is
// civitai/cli#552's work, and every one of these surfaces was fully raw before
// #566, so the gates below are a strict improvement on it rather than a claim
// to have closed it.
func downloadOne(ctx context.Context, dl civitai.Downloader, out, errW io.Writer, f civitai.ModelVersionFile, target string, o *downloadOpts) (skipped bool, err error) {
	sha := strings.TrimSpace(f.Hashes.SHA256)
	verify := !o.noVerify && sha != ""
	emitPreDownloadNotes(errW, f.Name, o.modelType, o.noVerify, sha)

	// Idempotency: skip an already-present target unless --force.
	if presentTargetSatisfies(out, target, sha, verify, o.force) {
		return true, nil
	}

	// Deliberately NOT gated, unlike every other error below, because `dir` holds
	// only bytes the USER typed and saferune's package doc is explicit that the
	// strip is not applied to those. That rests on THREE premises, all true today
	// and all verified in civitai/cli#572:
	//
	//  1. targetPath puts the server's name at the LEAF (`filepath.Base(f.Name)`),
	//     which `filepath.Dir` drops;
	//  2. its bare-`base` return yields Dir == "." and is skipped by the condition
	//     below;
	//  3. under --layout, `routeDir` returns --root, or --root joined with a
	//     compile-time `layoutFolders` constant — the server's file/model type
	//     only picks the map key, it never reaches `dir`.
	//
	// MkdirAll's *fs.PathError carries a prefix of `dir`, so the cause is
	// user-typed too, and gating it would be the #393 defect (rewriting the user's
	// own path back at them).
	//
	// 🔴 SO THIS IS A CONDITIONAL, NOT A PROHIBITION, AND NOTHING MECHANICAL HOLDS
	// IT. The ledger's GREW check only fires for a function that ALREADY calls
	// safeTerm, and the `bareIdentArgs["dir"]` row went away with the old gate — so
	// if a later change breaks any of the three premises (the obvious one: a
	// --layout variant routing by a server-supplied type STRING rather than by a
	// map key), `dir` silently becomes server text printed raw, with nothing red
	// and this comment reading as permission. RE-DERIVE ALL THREE before deciding;
	// if any has stopped holding, a gate here is correct and this comment is what
	// is stale.
	if dir := filepath.Dir(target); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return false, fmt.Errorf("create output directory %s: %w", dir, err)
		}
	}

	resp, err := dl.DownloadFile(ctx, f.DownloadURL)
	if err != nil {
		return false, fmt.Errorf("download %s: %w", safeTerm(f.Name), safeTermErr(err))
	}
	defer resp.Body.Close()

	if err := downloadStatusError(resp.StatusCode, f.Name); err != nil {
		return false, err
	}

	total := resp.ContentLength
	if total <= 0 && f.SizeKB > 0 {
		total = int64(f.SizeKB * 1024)
	}

	partPath := target + ".part"
	written, got, err := writePart(resp.Body, partPath, errW, f.Name, total, verify)
	if err != nil {
		return false, err
	}

	if verify && !strings.EqualFold(got, sha) {
		// Delete the corrupt partial so the failure message is honest about the
		// state on disk.
		_ = os.Remove(partPath)
		return false, fmt.Errorf("SHA256 mismatch for %s — expected %s, got %s (deleted the partial download)", safeTerm(f.Name), strings.ToLower(sha), got)
	}

	if err := os.Rename(partPath, target); err != nil {
		// The .part is finished but couldn't be installed; don't leave it behind.
		_ = os.Remove(partPath)
		return false, fmt.Errorf("install %s: %w", safeTerm(target), safeTermErr(err))
	}

	note := ""
	if verify {
		note = "  (SHA256 verified)"
	}
	fmt.Fprintf(out, "Saved %s (%s)%s\n", safeTerm(target), humanBytes(written), note)
	return false, nil
}

// emitPreDownloadNotes prints the once-per-file stderr notes for a download: the
// missing-hash verification-skip warning, the pickle/archive code-execution
// warning, and the ControlNet preprocessor-dependency hint. Notes go to stderr
// only, so --json / pipes on stdout are unaffected.
func emitPreDownloadNotes(errW io.Writer, name, modelType string, noVerify bool, sha string) {
	if !noVerify && sha == "" {
		fmt.Fprintln(errW, ui.For(errW).Warn(fmt.Sprintf("%s: no SHA256 published — skipping integrity verification", safeTerm(name))))
	}
	// Surface the code-execution risk of pickle/archive formats (a .ckpt, .pt, or
	// .zip lands in a folder ComfyUI/A1111 will auto-load).
	if note := pickleArchiveNote(name); note != "" {
		fmt.Fprintln(errW, note)
	}
	// Point ControlNet downloads at the preprocessor/annotator dependency the CLI
	// can't fetch (a separate ComfyUI custom node, not hosted on Civitai).
	if note := controlnetPreprocessorNote(modelType); note != "" {
		fmt.Fprintln(errW, note)
	}
}

// presentTargetSatisfies reports whether an already-present target lets the
// download be skipped (idempotency), printing the "already present" line it
// decides on. With --force it never skips. Otherwise a non-directory file at
// target is trusted when not verifying (or no hash is known); when verifying it
// is skipped only on a confirmed SHA256 match. A present-but-wrong/unverifiable
// file returns false → re-download.
func presentTargetSatisfies(out io.Writer, target, sha string, verify, force bool) bool {
	if force {
		return false
	}
	info, statErr := os.Stat(target)
	if statErr != nil || info.IsDir() {
		return false
	}
	if !verify {
		fmt.Fprintf(out, "already present: %s\n", safeTerm(target))
		return true
	}
	if match, verr := fileSHA256Matches(target, sha); verr == nil && match {
		fmt.Fprintf(out, "already present (SHA256 verified): %s\n", safeTerm(target))
		return true
	}
	return false
}

// writePart streams body into "<target>.part" while updating the progress
// display and (when verify) a SHA256 hasher. Any stale ".part" from a previous
// aborted run is removed first (there is no resume). On ANY failure it closes
// and removes the partial file before returning, so an interrupted or failed
// transfer never leaves a truncated ".part" behind. On success it returns the
// byte count written and, when verify is set, the lowercase-hex SHA256 of the
// streamed bytes; the caller then owns the finished (closed) ".part" file and is
// responsible for verifying / renaming / removing it.
//
// 🔴 ITS THREE ERROR STRINGS ARE GATED FOR THE REASON downloadOne's ARE
// (civitai/cli#566): main.go prints err.Error() raw to stderr. `name` is the
// SERVER's files[].name; `partPath` is the mixed-origin target plus ".part".
// Leaving these raw while downloadOne's are gated would rebuild the exact
// sibling-renderer split #566 was filed about, one call frame down.
func writePart(body io.Reader, partPath string, errW io.Writer, name string, total int64, verify bool) (written int64, gotHash string, err error) {
	// Self-heal a stale ".part": remove it before a fresh transfer (os.Create
	// truncates too, but be explicit).
	if _, statErr := os.Stat(partPath); statErr == nil {
		_ = os.Remove(partPath)
	}
	partFile, err := os.Create(partPath)
	if err != nil {
		return 0, "", fmt.Errorf("create %s: %w", safeTerm(partPath), safeTermErr(err))
	}
	// Best-effort cleanup of the partial file on any failure before we hand a
	// finished file back to the caller.
	cleanup := true
	defer func() {
		if cleanup {
			_ = partFile.Close()
			_ = os.Remove(partPath)
		}
	}()

	pw := newProgressWriter(errW, name, total)
	hasher := sha256.New()
	writers := []io.Writer{partFile, pw}
	if verify {
		writers = append(writers, hasher)
	}
	if _, err := io.Copy(io.MultiWriter(writers...), body); err != nil {
		return 0, "", fmt.Errorf("streaming %s: %w", safeTerm(name), safeTermErr(err))
	}
	pw.done()
	if err := partFile.Close(); err != nil {
		return 0, "", fmt.Errorf("finalize %s: %w", safeTerm(partPath), safeTermErr(err))
	}
	cleanup = false

	if verify {
		gotHash = hex.EncodeToString(hasher.Sum(nil))
	}
	return pw.written, gotHash, nil
}

// pickleExts / archiveExts name the downloaded-file extensions that warrant a
// trust note: pickle/executable model formats (which can run arbitrary code when
// deserialized by ComfyUI/A1111/torch.load) and archive formats (which unpack
// into those same auto-loaded folders). Safetensors + images are safe and get no
// note.
var (
	pickleExts  = map[string]bool{".ckpt": true, ".pt": true, ".pth": true, ".bin": true, ".pickle": true, ".pkl": true}
	archiveExts = map[string]bool{".zip": true, ".tar": true, ".gz": true, ".tgz": true, ".rar": true, ".7z": true}
)

// pickleArchiveNote returns a one-line stderr note when name has a pickle/
// executable or archive extension, warning that the file can execute code when
// loaded and that SHA256 proves integrity (byte-match), not authenticity (a
// trusted source). It returns "" for safetensors, images, and other inert types.
// The returned string is safeTerm-sanitized (the file name is server-supplied).
func pickleArchiveNote(name string) string {
	base := strings.ToLower(filepath.Base(name))
	ext := filepath.Ext(base)
	// ".tar.gz"/".tgz" report ext ".gz"/".tgz"; both are archives.
	if !pickleExts[ext] && !archiveExts[ext] {
		return ""
	}
	return safeTerm(fmt.Sprintf("note: %s is a pickle/archive-format file — it can execute code when loaded; only load models from creators you trust.", filepath.Base(name)))
}

// controlnetPreprocessorNote returns a one-line stderr note when the parent model
// is a ControlNet, pointing at the preprocessor/annotator dependency the CLI can't
// fetch: a ControlNet model needs a matching preprocessor (e.g. the ComfyUI
// `comfyui_controlnet_aux` custom node — OpenPose/Canny/Depth) to derive the
// control image, and that lives outside Civitai. The match is case-insensitive on
// the model type; it returns "" for every other type. The note is fully static
// (no server-derived substring), so it needs no safeTerm sanitization.
func controlnetPreprocessorNote(modelType string) string {
	if !strings.EqualFold(strings.TrimSpace(modelType), "Controlnet") {
		return ""
	}
	return "note: ControlNet models need a matching preprocessor/annotator to generate the control image (e.g. the ComfyUI 'comfyui_controlnet_aux' custom node — OpenPose/Canny/Depth). That preprocessor is a separate install, not available on Civitai."
}

// downloadStatusError maps a non-2xx download response to an actionable error.
func downloadStatusError(status int, name string) (err error) {
	// Classify the returned error by status (401/403→auth, 404→not-found, …)
	// without changing its message, so the process exit code reflects the kind.
	defer func() { err = civitai.TagStatus(status, err) }()
	// 🔴 THE NAME IS THE SERVER'S files[].name AND main.go PRINTS err.Error()
	// RAW — civitai/cli#566. Gated ONCE, here, rather than at each of the four
	// returns below: they are four spellings of one rule, and #566 exists because
	// two spellings of one rule drifted apart. The 401 arm is the most exposed of
	// them — an anonymous download of a gated file reaches it on the FIRST run,
	// before the user has seen that file name on any other surface, and the
	// message it would forge is an instruction to run `civitai login`.
	name = safeTerm(name)
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusUnauthorized:
		// Civitai requires a token to download ANY model file — even public ones —
		// so an anonymous download 401s. Point the user straight at login.
		return fmt.Errorf("downloading %s requires authentication (401) — run `civitai login` (or set CIVITAI_TOKEN); this file needs a token (most model files do; some public files don't)", name)
	case status == http.StatusForbidden:
		// Authenticated but refused: the file is gated, not a login problem.
		return fmt.Errorf("downloading %s was forbidden (403) — your token is valid but this file is gated (early-access / subscriber-only / not shared with your account), so logging in again won't help", name)
	case status == http.StatusNotFound:
		return fmt.Errorf("download URL for %s returned 404 — the file may have been removed", name)
	default:
		return fmt.Errorf("download of %s failed (HTTP %d)", name, status)
	}
}

// fileSHA256Matches streams path and reports whether its SHA256 equals want
// (case-insensitive).
func fileSHA256Matches(path, want string) (bool, error) {
	fh, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer fh.Close()
	h := sha256.New()
	if _, err := io.Copy(h, fh); err != nil {
		return false, err
	}
	return strings.EqualFold(hex.EncodeToString(h.Sum(nil)), want), nil
}

// ── progress ─────────────────────────────────────────────────────────────────

// progressWriter counts streamed bytes and renders progress to a writer
// (stderr). On a TTY it draws a throttled, carriage-return-updated line; off a
// TTY it prints an occasional plain line so long downloads still show life
// without flooding a log.
type progressWriter struct {
	w         io.Writer
	name      string
	total     int64 // expected size; 0 = unknown
	written   int64
	tty       bool
	start     time.Time
	lastPrint time.Time
}

func newProgressWriter(w io.Writer, name string, total int64) *progressWriter {
	return &progressWriter{w: w, name: name, total: total, tty: ui.IsTTY(w), start: time.Now()}
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n := len(b)
	p.written += int64(n)
	now := time.Now()
	if p.tty {
		if now.Sub(p.lastPrint) >= 100*time.Millisecond {
			p.lastPrint = now
			fmt.Fprint(p.w, "\r"+p.line())
		}
	} else if now.Sub(p.lastPrint) >= 5*time.Second {
		p.lastPrint = now
		fmt.Fprintln(p.w, p.line())
	}
	return n, nil
}

// done finishes the progress display (clears/terminates the TTY line).
func (p *progressWriter) done() {
	if p.tty {
		fmt.Fprint(p.w, "\r"+p.line()+"\n")
	}
}

// line renders the progress line. p.name is the SERVER's files[].name — it is
// threaded here from downloadOne via writePart's `name` parameter — so it is
// gated by safeTerm like every other server-origin string this CLI prints.
//
// 🔴 THIS IS THE \r-REWRITTEN SURFACE, WHERE A CURSOR ESCAPE IS WORTH MOST, AND
// IT WAS RAW UNTIL civitai/cli#566. Both of line's callers rewrite the line in
// place — Write's TTY branch prints "\r"+line() ten times a second, done()
// prints "\r"+line()+"\n" — so the terminal is already being told to move the
// cursor by the CLI itself, and an `\x1b[1A\x1b[2K` in the file name simply
// extends that reach upward into lines the CLI wrote earlier (the pickle/archive
// EXECUTION WARNING and the no-SHA256 warning, both emitted by
// emitPreDownloadNotes immediately above this progress line). #564's ledger
// makes exactly this argument for (*ttyPollReporter).tick.
//
// It is stripped ONCE per call rather than once per interpolation so the two
// branches cannot drift apart — the defect shape #566 found in
// checkTargetCollisions.
//
// ⚠ RESIDUAL, AND IT IS WORTH MOST ON THIS SURFACE OF THE FOUR: saferune keeps
// `\n` on purpose, so a newline in files[].name still forges whole lines here,
// and Write re-emits "\r"+line() at 10 Hz — so a forged `Saved … (SHA256
// verified)` can scroll past before the transfer has finished. safeTerm is the
// escape gate, not a one-line guarantee; safeTermSingle is the guarantee, and
// applying it across the renderers is civitai/cli#552. This line was fully raw
// before #566, so the strip above narrows the vector rather than closing it.
func (p *progressWriter) line() string {
	name := safeTerm(p.name)
	if p.total > 0 {
		pct := float64(p.written) / float64(p.total) * 100
		return fmt.Sprintf("  %s  %s / %s (%.0f%%)", name, humanBytes(p.written), humanBytes(p.total), pct)
	}
	return fmt.Sprintf("  %s  %s", name, humanBytes(p.written))
}

// humanBytes renders a byte count in IEC binary units (KiB/MiB/GiB…).
//
// 🔴 THE LABEL AND THE DIVISOR HAVE TO AGREE. This divided by 1024 and printed
// the SI suffixes (KB/MB/GB) "for brevity", which is not an abbreviation — it is
// a different quantity. 2,097,152 bytes rendered as "2.0 MB", 4.9% below the
// value it described. That was cosmetic while the string only reached download
// progress; #275 put it in `--help`, where `set-icon` / `set-cover` interpolate
// listingSourceRule(kind) and so advertised "at most 2.0 MB" / "at most 4.0 MB"
// for caps the README documents — correctly — as 2 MiB and 4 MiB.
//
// It is fixed by RELABELLING, not by re-basing the arithmetic: every caller is
// already a binary quantity. maxIconBytes and its siblings are literal
// 1024-multiples mirroring server constants that are themselves 1024-multiples;
// download progress counts raw bytes against a Content-Length; and the catalog's
// SizeKB is multiplied by 1024 at the call site. A 1000 divisor would restate all
// of them in a unit none of them is measured in and make `--help` quote "2.1 MB"
// for the 2 MiB cap the README and AGENTS.md item 25 both name. The numbers this
// returns are therefore unchanged from every release before this one; only the
// suffix moved. TestHumanBytesLabelsItsOwnArithmetic pins both halves.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
