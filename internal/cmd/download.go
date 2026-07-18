package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/ui"
	"github.com/spf13/cobra"
)

// downloadOpts holds the resolved flag values for `civitai download`.
type downloadOpts struct {
	modelID  string // --model (mutually exclusive with the positional version id)
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

  civitai download 128713

…or resolve a model's default (first published) version with --model:

  civitai download --model 4384

Use --dry-run to print the resolved plan (files, sizes, SHA256, target paths,
and whether auth is required) without transferring anything.

By default the version's PRIMARY file is downloaded into the current directory
under its server-provided name. Any file type downloads — model weights, but
also non-weights deliverables like a "Workflows" model's Archive, training data,
or other artifacts. Use --file to pick a specific file, or --all to download
every file. Downloads stream to "<target>.part" and are renamed into place only
on success, so an interrupted run never leaves a truncated final file.

Authentication: downloading ANY model file requires authentication — run
'civitai login' (Civitai requires a token even for public files; a 336 KB public
embedding 401s just like a gated checkpoint). Your stored login token or
CIVITAI_API_KEY is sent automatically. The read/search commands (models,
model-versions, articles, images, …) work anonymously; downloads do not.

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
		Example: `  civitai download 128713
  civitai download --model 4384 --out ./dreamshaper.safetensors
  civitai download 128713 --file vae --out-dir ./models
  civitai download 128713 --all --out-dir ./models
  civitai download 128713 --all --layout comfyui --root ~/ComfyUI
  civitai download 128713 --layout a1111 --for-base "SDXL 1.0"`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDownload(cmd, args, o)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.modelID, "model", "", "resolve+download a MODEL's default (first published) version instead of a version id")
	f.StringVar(&o.file, "file", "", "select a specific file by name (exact match, else a unique case-insensitive substring)")
	f.BoolVar(&o.all, "all", false, "download every file in the version")
	f.StringVar(&o.out, "out", "", "target file path (single-file only; mutually exclusive with --all/--out-dir)")
	f.StringVar(&o.outDir, "out-dir", "", "directory to write server-named file(s) into (created if needed)")
	f.StringVar(&o.layout, "layout", "", "route each file into its type's subfolder for an app (a1111|comfyui); mutually exclusive with --out/--out-dir")
	f.StringVar(&o.root, "root", "", "base directory for --layout routing (default \".\"; only applies with --layout)")
	f.StringVar(&o.forBase, "for-base", "", "warn if the version's base model is a confidently different family than this target (e.g. \"SDXL 1.0\")")
	f.BoolVar(&o.noVerify, "no-verify", false, "skip SHA256 verification of the downloaded bytes")
	f.BoolVar(&o.force, "force", false, "re-download even if the target file already exists")
	f.BoolVar(&o.anon, "anon", false, "force an anonymous request (ignore any stored login token); NOTE: downloads still 401 without a token — --anon is meaningful for read commands, not downloads")
	f.BoolVar(&o.dryRun, "dry-run", false, "print the resolved download plan (files, sizes, hashes, targets) and exit without downloading anything")
	return cmd
}

func runDownload(cmd *cobra.Command, args []string, o *downloadOpts) error {
	// ── resolve the target version id (exactly one of positional / --model) ──
	var positional string
	if len(args) == 1 {
		positional = strings.TrimSpace(args[0])
	}
	if positional != "" && o.modelID != "" {
		return fmt.Errorf("provide EITHER a version id or --model <model-id>, not both")
	}
	if positional == "" && o.modelID == "" {
		return fmt.Errorf("provide a model-version id (civitai download <version-id>) or --model <model-id>")
	}

	// ── validate flag combinations up front ──
	if o.all && o.file != "" {
		return fmt.Errorf("--all and --file are mutually exclusive")
	}
	if o.out != "" && o.outDir != "" {
		return fmt.Errorf("--out (a file path) and --out-dir (a directory) are mutually exclusive")
	}
	if o.out != "" && o.all {
		return fmt.Errorf("--out sets a single file path and can't be combined with --all — use --out-dir")
	}
	if o.layout != "" {
		if !validLayout(o.layout) {
			return fmt.Errorf("--layout must be one of %s, got %q", strings.Join(knownLayouts, "|"), o.layout)
		}
		if o.out != "" {
			return fmt.Errorf("--layout routes files into type folders and can't be combined with --out (an explicit single path)")
		}
		if o.outDir != "" {
			return fmt.Errorf("--layout routes files under --root and can't be combined with --out-dir")
		}
	} else if o.root != "" {
		return fmt.Errorf("--root only applies with --layout")
	}

	client, baseURL, err := newReader(&readOpts{anon: o.anon})
	if err != nil {
		return err
	}
	// Cancel the whole download on Ctrl-C (SIGINT): the signal-bound context is
	// threaded into every request + io.Copy, so an interrupted transfer unblocks
	// with a cancellation error and downloadOne's cleanup defer removes the
	// partial ".part" file rather than leaving it behind.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	out := cmd.OutOrStdout()
	errW := cmd.ErrOrStderr()

	versionID, err := resolveVersionID(ctx, client, positional, o.modelID)
	if err != nil {
		return err
	}

	v, _, err := client.GetModelVersion(ctx, versionID)
	if err != nil {
		return err
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

	// Always surface the version's base model, and (with --for-base) warn on a
	// confident cross-family mismatch before any bytes move.
	fmt.Fprintf(out, "base model: %s\n", dashIfEmpty(safeTerm(v.BaseModel)))
	if o.forBase != "" {
		if w := baseModelWarning(v.BaseModel, o.forBase); w != "" {
			fmt.Fprintln(errW, ui.For(errW).Warn(safeTerm(w)))
		}
	}

	// --dry-run: print the resolved plan and exit 0, transferring nothing and
	// creating no file (not even a ".part").
	if o.dryRun {
		return printDownloadPlan(out, selected, o, baseURL)
	}

	// Without --layout, warn when `--all` would mis-file differing types into one
	// directory (the footgun --layout exists to fix).
	if o.all && o.layout == "" {
		if w := mixedTypeWarning(fileTypeInfos(selected, o.modelType)); w != "" {
			fmt.Fprintln(errW, ui.For(errW).Warn(safeTerm(w)))
		}
	}

	downloaded := 0
	for i := range selected {
		f := selected[i]
		target, note, err := targetPath(f, o)
		if err != nil {
			return err
		}
		if note != "" {
			fmt.Fprintln(errW, ui.For(errW).Warn(safeTerm(note)))
		}
		skipped, err := downloadOne(ctx, client, out, errW, f, target, o)
		if err != nil {
			return err
		}
		if !skipped {
			downloaded++
		}
	}

	if downloaded == 0 && o.all {
		// Everything was already present on disk.
		fmt.Fprintln(errW, ui.For(errW).Warn("no files were downloaded (all were already present)"))
	}
	return nil
}

// resolveVersionID returns the version id to download: the positional id verbatim
// (validated numeric), or the default (first published) version of --model.
//
// For --model, the API returns modelVersions default/latest-first, so the model's
// default version is modelVersions[0]. Its primary file is downloaded regardless
// of file type — a "Workflows" model's Archive, training data, or plain weights
// all download the same way.
func resolveVersionID(ctx context.Context, client api.Reader, positional, modelID string) (string, error) {
	if positional != "" {
		if _, err := strconv.Atoi(positional); err != nil {
			return "", fmt.Errorf("model-version id must be an integer, got %q", positional)
		}
		return positional, nil
	}
	if _, err := strconv.Atoi(modelID); err != nil {
		return "", fmt.Errorf("--model id must be an integer, got %q", modelID)
	}
	m, _, err := client.GetModel(ctx, modelID)
	if err != nil {
		return "", err
	}
	if len(m.ModelVersions) == 0 {
		return "", fmt.Errorf("model %s has no published versions to download", modelID)
	}
	return strconv.Itoa(m.ModelVersions[0].ID), nil
}

// printDownloadPlan renders the --dry-run plan: for each selected file its name,
// size, SHA256, resolved target path, and whether authentication will be
// required. It writes NOTHING to disk.
func printDownloadPlan(out io.Writer, files []api.ModelVersionFile, o *downloadOpts, baseURL string) error {
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
		if api.DownloadNeedsAuth(f.DownloadURL, baseURL) {
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
func selectFiles(files []api.ModelVersionFile, o *downloadOpts) ([]api.ModelVersionFile, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("this version has no downloadable files")
	}
	if o.all {
		return files, nil
	}
	if o.file != "" {
		return selectOneFile(files, o.file)
	}
	primary := api.PrimaryFile(files)
	return []api.ModelVersionFile{*primary}, nil
}

// selectOneFile picks a single file by name: an exact (case-insensitive) name
// match wins; otherwise a UNIQUE case-insensitive substring match; ambiguous or
// no match is an error that lists the available files.
func selectOneFile(files []api.ModelVersionFile, want string) ([]api.ModelVersionFile, error) {
	// Exact (case-insensitive) name match first.
	for i := range files {
		if strings.EqualFold(files[i].Name, want) {
			return []api.ModelVersionFile{files[i]}, nil
		}
	}
	// Unique case-insensitive substring match.
	var matches []api.ModelVersionFile
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
		return nil, fmt.Errorf("%q is ambiguous — it matches %d files. Be more specific:\n%s", want, len(matches), formatFileList(matches))
	}
}

// formatFileList renders a compact "  - name (type, size)" list for errors.
func formatFileList(files []api.ModelVersionFile) string {
	var b strings.Builder
	for i := range files {
		f := files[i]
		fmt.Fprintf(&b, "  - %s (%s, %s)\n", safeTerm(f.Name), safeTerm(f.Type), humanBytes(int64(f.SizeKB*1024)))
	}
	return strings.TrimRight(b.String(), "\n")
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
func targetPath(f api.ModelVersionFile, o *downloadOpts) (string, string, error) {
	if o.out != "" {
		return o.out, "", nil
	}
	base := filepath.Base(f.Name)
	if base == "." || base == ".." || base == string(filepath.Separator) || base == "/" {
		return "", "", fmt.Errorf("server returned an unusable filename %q; pass --out to set the output path", f.Name)
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
func fileTypeInfos(files []api.ModelVersionFile, modelType string) []fileTypeInfo {
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
func downloadOne(ctx context.Context, dl api.Downloader, out, errW io.Writer, f api.ModelVersionFile, target string, o *downloadOpts) (skipped bool, err error) {
	sha := strings.TrimSpace(f.Hashes.SHA256)
	verify := !o.noVerify && sha != ""
	if !o.noVerify && sha == "" {
		fmt.Fprintln(errW, ui.For(errW).Warn(fmt.Sprintf("%s: no SHA256 published — skipping integrity verification", safeTerm(f.Name))))
	}
	// Surface the code-execution risk of pickle/archive formats (a .ckpt, .pt, or
	// .zip lands in a folder ComfyUI/A1111 will auto-load), once per file.
	if note := pickleArchiveNote(f.Name); note != "" {
		fmt.Fprintln(errW, note)
	}

	// Idempotency: skip an already-present target unless --force. When verifying
	// and a hash is known, only skip on a confirmed match; without a hash (or
	// --no-verify) a present file is trusted.
	if !o.force {
		if info, statErr := os.Stat(target); statErr == nil && !info.IsDir() {
			if !verify {
				fmt.Fprintf(out, "already present: %s\n", safeTerm(target))
				return true, nil
			}
			match, verr := fileSHA256Matches(target, sha)
			if verr == nil && match {
				fmt.Fprintf(out, "already present (SHA256 verified): %s\n", safeTerm(target))
				return true, nil
			}
			// Present but wrong/unverifiable → re-download.
		}
	}

	if dir := filepath.Dir(target); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return false, fmt.Errorf("create output directory %s: %w", dir, err)
		}
	}

	resp, err := dl.DownloadFile(ctx, f.DownloadURL)
	if err != nil {
		return false, fmt.Errorf("download %s: %w", f.Name, err)
	}
	defer resp.Body.Close()

	if err := downloadStatusError(resp.StatusCode, f.Name); err != nil {
		return false, err
	}

	partPath := target + ".part"
	// Self-heal a stale ".part" left by a previously aborted run: we don't resume,
	// so remove it before starting a fresh transfer (os.Create truncates too, but
	// be explicit).
	if _, statErr := os.Stat(partPath); statErr == nil {
		_ = os.Remove(partPath)
	}
	partFile, err := os.Create(partPath)
	if err != nil {
		return false, fmt.Errorf("create %s: %w", partPath, err)
	}
	// Best-effort cleanup of the partial file on any failure before rename.
	cleanup := true
	defer func() {
		if cleanup {
			_ = partFile.Close()
			_ = os.Remove(partPath)
		}
	}()

	total := resp.ContentLength
	if total <= 0 && f.SizeKB > 0 {
		total = int64(f.SizeKB * 1024)
	}
	pw := newProgressWriter(errW, f.Name, total)

	hasher := sha256.New()
	writers := []io.Writer{partFile, pw}
	if verify {
		writers = append(writers, hasher)
	}
	if _, err := io.Copy(io.MultiWriter(writers...), resp.Body); err != nil {
		return false, fmt.Errorf("streaming %s: %w", f.Name, err)
	}
	pw.done()
	if err := partFile.Close(); err != nil {
		return false, fmt.Errorf("finalize %s: %w", partPath, err)
	}

	if verify {
		got := hex.EncodeToString(hasher.Sum(nil))
		if !strings.EqualFold(got, sha) {
			// Delete the corrupt partial; the defer would also remove it, but be
			// explicit so the failure message is honest about the state on disk.
			_ = os.Remove(partPath)
			cleanup = false
			return false, fmt.Errorf("SHA256 mismatch for %s — expected %s, got %s (deleted the partial download)", f.Name, strings.ToLower(sha), got)
		}
	}

	if err := os.Rename(partPath, target); err != nil {
		return false, fmt.Errorf("install %s: %w", target, err)
	}
	cleanup = false

	note := ""
	if verify {
		note = "  (SHA256 verified)"
	}
	fmt.Fprintf(out, "Saved %s (%s)%s\n", safeTerm(target), humanBytes(pw.written), note)
	return false, nil
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

// downloadStatusError maps a non-2xx download response to an actionable error.
func downloadStatusError(status int, name string) error {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusUnauthorized:
		// Civitai requires a token to download ANY model file — even public ones —
		// so an anonymous download 401s. Point the user straight at login.
		return fmt.Errorf("downloading %s requires authentication (401) — run `civitai login` (or set CIVITAI_API_KEY); Civitai needs a token to download any model file, even public ones", name)
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

func (p *progressWriter) line() string {
	if p.total > 0 {
		pct := float64(p.written) / float64(p.total) * 100
		return fmt.Sprintf("  %s  %s / %s (%.0f%%)", p.name, humanBytes(p.written), humanBytes(p.total), pct)
	}
	return fmt.Sprintf("  %s  %s", p.name, humanBytes(p.written))
}

// humanBytes renders a byte count in binary units (KiB/MiB/GiB…), abbreviated
// as KB/MB/GB for brevity.
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
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
