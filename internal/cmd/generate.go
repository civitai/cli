package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/genapi"
	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// generateWorkflow is the only generation mode this release builds. img2img and
// arbitrary graphs come later; pinning it here keeps the graph honest about what
// the five flags can actually express.
const generateWorkflow = "txt2img"

// serverQuantityClamp is the server's CURRENT upper bound on --quantity. It is
// used for a WARNING ONLY and must never gate the request: the server silently
// clamps out-of-range values (measured: 10000 -> 10, -5 -> 1) with no error, so
// a user who asks for 40 images pays for 10 and gets no signal at all. Warning
// fails soft — if the platform raises the bound, a stale number here produces a
// needless note, never a blocked valid request. Do not promote it to validation
// (this repo's anti-mirror rule).
const serverQuantityClamp = 10

// buzzLedgerUnknownNote is THE ONE THING this CLI is allowed to say about the
// fate of a charge it cannot observe, and it is a shared constant rather than
// three hand-written sentences ON PURPOSE (#278, AGENTS.md item 28).
//
// 🔴 A BANNED-PHRASE LIST IS NOT A GUARD HERE, AND THAT WAS MEASURED. The first
// version of this fix policed the copy with a substring ledger of directional
// claims ("not refunded", "has been refunded", …). An audit mutant rewrote the
// terminal-status error to "your Buzz returns to your balance automatically;
// confirm with `civitai buzz`" — a paraphrase that asserts a refund, matches no
// banned substring, and left ALL EIGHTEEN PACKAGES GREEN. Any list of phrases
// loses to the next paraphrase; the property "this sentence decides the refund
// question" is not computable from text.
//
// So the guard is structural instead: every surface that speaks to the FATE of a
// charge must render THIS constant verbatim, and the set of call sites is an
// asserted ledger (see TestBuzzLedgerNoteIsTheOnlyRefundWording).
//
// 🔴 THE SCOPE IS "FATE", NOT "MONEY", and the distinction is load-bearing
// because the comment used to overstate it. Saying a charge HAPPENED is a
// different claim from saying what became of it, it is not in doubt, and
// softening it would understate what the user paid for — so
// `reportOutputCountMismatch` ("the difference was charged for either way") and
// the succeeded-but-empty error ("it was charged") deliberately do NOT render
// this note and are deliberately NOT in the ledger.
//
// A paraphrase drops the constant, which goes red; a site that stops using it
// shrinks the ledger, which also goes red. 🔴 What NEITHER catches is a brand-new
// file that never mentions the constant at all — measured: an added
// `zz_new_refund_surface.go` printing "your Buzz has been refunded in full"
// survived with all 18 packages ok, because the ledger only records files where
// the count is non-zero. The golden-output guards below close that for the
// surfaces they cover; a fate-of-charge claim on some OTHER command's output is
// the standing residual, recorded in AGENTS.md item 28.
//
// The wording is deliberately about OBSERVABILITY, not about the rule: it says
// what this process can and cannot see, which is true regardless of what the
// orchestrator decides. It names `/user/transactions` because `civitai buzz`
// reports a BALANCE and not a history — a balance alone cannot settle "did THIS
// run come back" unless you happened to note it beforehand.
const buzzLedgerUnknownNote = "this CLI cannot see your Buzz ledger — `civitai buzz` reports a balance, not a history, so settle it against your Buzz transaction history (/user/transactions)"

// noFailureReasonNote is the caveat that keeps `civitai workflows get <id>` an
// HONEST pointer rather than a promised diagnosis (#331, dogfood finding F13).
//
// 🔴 THE COMMAND STAYS; ONLY THE PROMISE GOES. Both errors below used to name
// the command with no qualification, so a user reasonably read it as "run this
// and you will learn why". The blind dogfood run of 2026-08-07 did exactly that
// on a failed job and got back `Status: failed`, `Outputs (0 deliverable, 1
// excluded)` and NO reason — the orchestrator simply does not supply one — and
// the only strategy left was to re-submit hypotheses at 8 Buzz each. That is
// what an unqualified pointer costs.
//
// The fix is deliberately SUBTRACTIVE IN CLAIM AND ADDITIVE IN NOTHING ELSE:
// the workflow id and the re-attach command are the user's only handle on a run
// they have already paid for, so they must survive verbatim. This sentence
// lowers the expectation of what that handle will tell them.
//
// It says nothing about the charge in either direction, so it is outside
// AGENTS.md item 28's rule — but the surfaces it lands on are golden-pinned
// spend surfaces, so an edit here has to be re-approved with
// `go test ./internal/cmd -run TestGoldenSpendCopy -update` and the diff read.
const noFailureReasonNote = "the orchestrator often supplies no failure reason, so it may not say why"

// Classification sentinels for generation failures that the HTTP-status mapping
// alone gets wrong. They carry NO user-visible text: they are ATTACHED via
// civitai.Tag so errors.Is reports the KIND while the printed message is
// whatever classifyGenerateError composed.
//
// 🔴 Why they exist: civitai.TagStatus maps BOTH 401 and 403 to
// civitai.ErrUnauthorized -> exit 3, documented as "login required / credential
// lacks scope". A muted account and an incomplete onboarding both arrive as a
// bare 403, so without this a restricted user is told to re-run `civitai login`
// — forever. Changing statusKind was not an option: it would silently move the
// exit code of every other command.
//
// None of these is mapped in cmd/civitai's exitCode, so they all land on the
// GENERIC exit 1 — deliberately distinct from 3 (auth) and 2 (usage), neither of
// which describes them. They are still errors.Is-assertable, which is what pins
// the contract.
var (
	// ErrInsufficientBuzz marks a refusal for lack of spendable Buzz — whether
	// caught locally against the balance or reported by the server.
	ErrInsufficientBuzz = errors.New("insufficient Buzz")
	// ErrGenerationDisabled marks generation being switched off server-side (or
	// restricted to members). Not the caller's fault and not fixable by them.
	ErrGenerationDisabled = errors.New("generation disabled")
	// ErrAccountRestricted marks a muted account or an incomplete onboarding —
	// the two failures that read byte-identical to a missing scope.
	ErrAccountRestricted = errors.New("account restricted")
	// ErrPromptBlocked marks a prompt the content audit refused. 🔴 A caller must
	// NEVER retry this: repeated blocked prompts increment a 30-day counter that
	// auto-mutes the account.
	ErrPromptBlocked = errors.New("prompt blocked")
	// ErrModelSubstituted marks a run refused by --fail-on-substitution because
	// the server reported it would run a different checkpoint than the one asked
	// for. It is raised BEFORE the submit, so nothing has been spent.
	//
	// It is a distinct sentinel rather than a usage error because it is not a
	// mistake in the command line: the invocation was well-formed and the server
	// answered it — what failed is an expectation about WHICH MODEL runs, and a
	// script needs to tell that apart from a typo'd flag.
	ErrModelSubstituted = errors.New("model substituted")
)

// generateDeps are the network seams `generate` needs. Bundled as func fields so
// every branch — confirmation, cost ceiling, each error row — is exercisable
// without a live server, and so a test can PROVE the submit seam was not called.
type generateDeps struct {
	// whatIf estimates the cost. It spends nothing.
	whatIf func(ctx context.Context, g genapi.Graph) (*genapi.WhatIfResult, json.RawMessage, error)
	// submit SPENDS BUZZ. 🔴 A test must never point this at a real server.
	submit func(ctx context.Context, g genapi.Graph, opts genapi.SubmitOptions) (*genapi.SubmitResult, string, json.RawMessage, error)
	// resolveVersion turns a model-version id into its model TYPE + display name.
	resolveVersion func(ctx context.Context, id int) (*genapi.ResolvedVersion, error)
	// buzzBalance reads the spendable balance for the cost-vs-balance check.
	buzzBalance func(ctx context.Context) (int64, error)
	// getWorkflow reads a workflow back. It is the POLL seam, and it spends
	// nothing.
	getWorkflow getWorkflowFn
	// downloadBlob fetches one presigned output URL WITHOUT a credential (see
	// blobFetcher). It is also what reads a remote --image's header bytes, so
	// that fetch inherits the same SSRF/https posture and carries no token.
	downloadBlob blobFetcher
	// uploadImage stores one local --image and returns the blob URL to reference
	// in the graph. 🔴 Its second hop must never carry a credential — see
	// genapi.UploadImageBlob and AGENTS.md item 19(e). It spends no Buzz.
	uploadImage func(ctx context.Context, contentType string, body []byte) (string, error)
	// pendingDir is where the pre-submit crash-recovery record is written. A
	// test points it at a t.TempDir(); empty means the real config dir.
	pendingDir string
	// poll carries the poll cadence + the injected clock/sleep. Zero values fall
	// back to the documented defaults (see pollConfig.resolved).
	poll pollConfig
}

// generateOpts is the parsed invocation. The *Set fields record whether a flag
// was given at all: an unset flag must be ABSENT from the outgoing JSON, never a
// Go zero value, because the server accepts `quantity: 0` and silently clamps it
// rather than rejecting it.
type generateOpts struct {
	prompt         string
	negativePrompt string
	quantity       int
	quantitySet    bool
	aspectRatio    string
	checkpoint     int
	checkpointSet  bool
	loras          []string
	// images are the --image values (local paths and/or https URLs).
	images []string
	// ecosystem is the --ecosystem passthrough. It is REQUIRED with --image; see
	// validateGenerateOpts.
	ecosystem  string
	dryRun     bool
	jsonOut    bool
	assumeYes  bool
	maxCost    int
	maxCostSet bool
	baseURL    string
	// failOnSubstitution turns a reported checkpoint substitution from a warning
	// into a refusal. Opt-in: the DEFAULT is to warn and continue, because the
	// server preserves a substitution deliberately so that a script pinned to a
	// retired version keeps working. See the flag registration for the argument.
	failOnSubstitution bool

	// noWait prints the workflow id and exits instead of waiting.
	noWait bool
	// timeout bounds the WAIT, never the job and never the charge.
	timeout time.Duration
	// outDir is the directory outputs are written into.
	outDir string
	// outName is the --out-name template for each output's file name. Empty
	// means defaultOutNameTemplate. It is USER-supplied input on the path that
	// decides where bytes land, so it is validated (rendered against a probe and
	// containment-checked) in validateGenerateOpts, BEFORE anything is submitted.
	outName string
	// noDownload waits for the result but writes no files.
	noDownload bool
	// force overwrites existing output files.
	force bool
	// externalID overrides the minted idempotency key, which is how a lost
	// submit is re-attached rather than re-charged.
	externalID string

	// inputPath is the `--input` graph file ("-" reads stdin). When set, the
	// graph is a raw passthrough and every content flag above is REFUSED.
	inputPath string
	// printInput dumps the assembled graph and exits without submitting.
	printInput bool
}

func newGenerateCmd() *cobra.Command {
	var o generateOpts

	cmd := &cobra.Command{
		Use:   "generate [prompt]",
		Short: "Generate images from a text prompt (SPENDS BUZZ)",
		Long: `Generate images from a text prompt on Civitai's generator.

🔴 THIS SPENDS REAL BUZZ AND CANNOT BE UNDONE. A submitted generation is charged
the moment the orchestrator accepts it, and nothing local calls that back —
neither --timeout nor Ctrl-C stops the job, and ` + "`civitai workflows cancel`" + ` stops
the remaining work rather than reversing the charge. Preview the price with
--dry-run first; it calls the server's cost estimator and spends nothing.
What the LEDGER then does with that charge — if the run fails, expires, or you
cancel it — is decided server-side, and ` + buzzLedgerUnknownNote + `.

CREDENTIAL: generation needs the AI Services scopes. Two credentials carry them:
` + spendCredentialRoutes + `. A DEFAULT OAuth browser login
(` + "`civitai login`" + ` with no --scopes) does NOT carry them and is refused — and
re-running plain ` + "`civitai login`" + ` will not fix that. Check yours with
` + "`civitai whoami`" + `.
The ONE exception is --print-input: it assembles the graph and exits before the
estimator, the submit and the balance read, so it needs no credential. Two
caveats, and they are NOT the same caveat. With --image it does need one, because
it uploads each local file first and that upload is authenticated. With
--checkpoint or --lora it still needs none — the model-version lookup is a public
read — but it is not OFFLINE: that lookup is a real request and fails without a
network. Only a bare --print-input needs neither a credential nor a network.

--max-cost IS AN ESTIMATE CHECK, NOT A SPENDING CAP. The cost this command shows
is an estimate, not a quote: the server's estimator returns no quote id, no
signed price and no expiry — there is nothing to hand back at submit time, and
no server-side ceiling is reachable from an API key at all. The realized charge
can exceed the estimate, and --max-cost cannot claw the difference back — it
never reaches the server. --max-cost compares the
ESTIMATE against your number and refuses locally before submitting; it catches a
--quantity typo, and that is all it can do. Do not run an unattended loop
believing it caps spend.

CONFIRMATION: an interactive run prints the estimate and your balance and asks
before spending. A non-interactive shell (pipe/CI) without --yes is REFUSED
rather than charged silently.

WHAT THE SERVER DOES NOT TELL YOU: the generator is permissive, not a validator.
An out-of-range --quantity is clamped with no error, and a checkpoint id that
does not exist is accepted with the ecosystem default silently substituted and
billed. This command therefore resolves every --checkpoint / --lora id against
the public model-version API BEFORE submitting, so a bad id is a hard local
error instead of a wrong charge, and it echoes the resolved model NAME in the
confirmation so you approve a name rather than an integer.

🔴 --dry-run's "Resources ready" line is NOT A PROMISE OF OUTPUT. It echoes the
server's ` + "`ready`" + ` flag, which reports only that the resources this job needs are
currently available — nothing about moderation, and nothing about whether the
job will actually produce an image. A run that reports resources ready can still
be charged and return nothing. Treat ` + "`ready: false`" + ` as "do not submit"; do not
read ` + "`ready: true`" + ` as a green light.

WAITING AND DOWNLOADING: by default the command waits for the job to finish and
writes every deliverable output into --out-dir as <workflow-id>-<n>.<ext>.
--out-name <template> names them instead: {workflow}, {n} (1-based) and {ext}
(with its leading dot) expand, everything else is literal. The rendered value
must be a plain file name inside --out-dir — a path separator or ".." is REFUSED,
not stripped — and the template is checked before anything is submitted, so a bad
one costs nothing. A template that would give two outputs the same name is
refused before any byte is downloaded rather than overwriting your own results,
so include {n} for a batch. Pass --no-wait to print the workflow id and exit
immediately, and pick the results up later with
` + "`civitai workflows get <workflow-id>`" + `. Output URLs are PRESIGNED AND EXPIRE, so
download promptly; re-read the workflow for fresh links.

🔴 --timeout STOPS WAITING. IT DOES NOT STOP THE JOB. The generation keeps
running server-side after the CLI gives up, and finishes and bills exactly as if
you had stayed. The same is true of Ctrl-C. Both print the workflow id and the
exact command to re-attach; ` + "`civitai workflows cancel`" + ` is the only thing that
stops the remaining work.

CRASH SAFETY: the idempotency key is written to a local file BEFORE the request
is sent, because the money moves server-side even if this process dies mid-POST.
If a submit's reply never arrives, re-run with --external-id <the recorded key>:
the orchestrator dedupes on it and returns the PRE-EXISTING workflow instead of
charging a second time.

IMAGE-TO-IMAGE: --image <file-or-url> attaches a reference image (repeatable).
A local png/jpeg is uploaded to Civitai first and the stored blob is referenced;
an https URL is passed through as-is, but must be publicly reachable, because the
generator downloads it server-side too. Either way the CLI reads the image's
width and height from its header and sends them — the server requires both and
rejects an entry without them.

🔴 --image REQUIRES --ecosystem, and the reason is money. The server turns a
text-to-image job into image-to-image only when the request names an ecosystem;
without one it ignores the images, generates from the prompt alone, and charges
you the full amount with no error. Worse, only SOME ecosystems accept reference
images at all (Qwen, Flux1Kontext, NanoBanana, Seedream, OpenAI, Grok and a few
more do; the SD family and the default do not) — and the cost estimate cannot
tell you which case you are in, because several edit-capable ecosystems price
identically with and without images. Name an ecosystem you know supports editing.

🔴 The server SILENTLY TRUNCATES too many reference images. Per-ecosystem limits
run from 1 to 7 and are not knowable from here; over the limit the extras are
dropped with no error and the truncated job is billed. The CLI refuses more than
7 (no ecosystem accepts more) and warns for anything above 1.

RAW GRAPHS: --input <file> (or --input -) sends a generation-graph JSON document
exactly as written, instead of building one from the flags above. It is how you
reach graph parameters this CLI has no flag for. Get a valid starting point with
--print-input, which assembles the graph, prints it, and exits without
submitting or even pricing anything.

--input is txt2img only in this release. It cannot be combined with the content
flags (--negative-prompt, --quantity, --aspect-ratio, --checkpoint, --lora) or
with a prompt argument; the execution flags all still apply. Keys that belong to
the request ENVELOPE rather than the graph — civitaiTip, creatorTip, buzzType,
tags, externalId — are REFUSED in an input file: they are this CLI's to set, and
a tip in particular is real Buzz that --dry-run structurally cannot see. Keys
this CLI does not recognise are passed through with a warning, because the
server silently drops what it does not declare rather than reporting an error.

🔴 --input DOES NOT get the model-id safety net. --checkpoint and --lora are
resolved against the public API before submitting, so a bad id fails locally
instead of being billed with a substituted model; a raw graph is not
interpreted, so nothing in it is checked before you pay for it.`,
		Example: `  # Preview the price — spends nothing
  civitai generate "a cat wearing sunglasses" --dry-run

  # The same estimate as JSON, for scripts
  civitai generate "a cat wearing sunglasses" --dry-run --json

  # Generate 4 images, refusing if the estimate exceeds 50 Buzz
  civitai generate "a cat wearing sunglasses" --quantity 4 --max-cost 50

  # A specific checkpoint plus a LoRA at 0.8 strength
  civitai generate "a cat" --checkpoint 128713 --lora 250712:0.8

  # Image-to-image from a local file — --ecosystem is required
  civitai generate "make it winter" --ecosystem Flux1Kontext --image ./cat.png --dry-run

  # …or from a public URL, with two reference images
  civitai generate "combine these" --ecosystem Seedream \
    --image https://example.com/a.jpg --image ./b.png --yes

  # Wait, and write the images into ./out
  civitai generate "a cat" --yes --out-dir ./out

  # …naming the files yourself — {n} keeps a batch from colliding
  civitai generate "a cat" --yes --quantity 4 --out-dir ./out --out-name 'cat-{n}{ext}'

  # Fire and forget; collect the results later
  civitai generate "a cat" --yes --no-wait
  civitai workflows get <workflow-id>

  # Non-interactive (CI): --yes is required, or the run is refused
  civitai generate "a cat" --yes --max-cost 20

  # Graduate from flags to a raw graph: print, edit, send back
  civitai generate "a cat" --quantity 2 --print-input > graph.json
  civitai generate --input graph.json --dry-run
  civitai generate --input graph.json --yes

  # …or pipe it straight through
  jq '.prompt = "a dog"' graph.json | civitai generate --input - --dry-run`,
		// MaximumNArgs(1), not ExactArgs(1): a later release adds a prompt-less
		// mode (--input). Today a missing prompt is a usage error, raised in
		// validateGenerateOpts. 🔴 This command deliberately has NO subcommands —
		// root.go's unknown-subcommand guard only covers non-runnable parents, so
		// a typo'd subcommand on a runnable one would be swallowed as the PROMPT
		// and billed.
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				o.prompt = args[0]
			}
			o.quantitySet = cmd.Flags().Changed("quantity")
			o.checkpointSet = cmd.Flags().Changed("checkpoint")
			o.maxCostSet = cmd.Flags().Changed("max-cost")

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			// 🔴 ONE credential gate, and it stays here — before every seam is
			// built, not pushed down into the call sites. `--print-input` is the
			// single exception, and only when it reaches no AUTHED request: it
			// assembles the graph and exits before the estimator, the submit and
			// the balance read, so with no `--image` it sends nothing that
			// carries a credential, and refusing it contradicted the invariant
			// stated at the --print-input short-circuit below. Issue #257.
			//
			// 🔴 "NO AUTHED REQUEST" IS NOT "NO REQUEST", AND THE TWO MUST NOT
			// BE COLLAPSED — an earlier revision of this comment said "makes NO
			// request of any kind", which is the wording that belongs at the
			// short-circuit (where it is guarded by "with no
			// --checkpoint/--lora") and is false here. `--checkpoint`/`--lora`
			// still perform the public model-version READ, so they need no
			// credential but DO need a network. Measured against a dead
			// endpoint, no credential: bare `--print-input` returns nil and
			// prints the graph; `--print-input --checkpoint <id>` and
			// `--print-input --lora <id>` both fail as transport errors (exit 5)
			// after the read's 4 attempts. Only a bare `--print-input` needs
			// neither.
			//
			// 🔴 The condition is `--image`, NOT `--checkpoint`/`--lora`, and
			// the difference is which hop needs a credential. `--image` with a
			// local file UPLOADS before printing (item 19(f)) and hop 1 of that
			// upload — `getConsumerBlobUploadUrl` — is AUTHED (item 19(e)), so
			// the flag genuinely needs a token even on this path. An https
			// `--image` reaches only the credential-free fetch, but a token is
			// still required for the whole flag rather than per value: the
			// narrower rule buys nothing, and "some --image values need a
			// credential" is a worse contract than "--image does".
			// `--checkpoint`/`--lora` resolve through the PUBLIC
			// `GET /api/v1/model-versions/{id}` (item 13, "free,
			// unauthenticated-capable"), so gating on them would keep refusing
			// a case that demonstrably works with no credential.
			needsCredential := !o.printInput || len(o.images) > 0
			if needsCredential && cfg.Token() == "" {
				return civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf(
					"no token configured — generation needs a credential with the AI Services scopes: "+
						spendCredentialRoutes+". Or set CIVITAI_TOKEN"))
			}
			o.baseURL = cfg.BaseURL()

			src := auth.New(cfg)
			gen := genapi.NewWithSource(cfg.BaseURL(), src)
			buzz := appapi.NewWithSource(cfg.BaseURL(), src, "")
			// 🔴 The blob fetcher is DownloadPresigned, never DownloadFile:
			// output URLs are presigned and are served from a *.civitai.com
			// host, which the download layer's trusted-host predicate would
			// match — so DownloadFile would hand a full-scope personal API key
			// to a request that needs no credential at all.
			reader := civitai.NewWithSource(cfg.BaseURL(), src)
			deps := generateDeps{
				whatIf:         gen.WhatIfFromGraph,
				submit:         gen.GenerateFromGraph,
				resolveVersion: gen.ResolveModelVersion,
				getWorkflow:    gen.GetWorkflow,
				downloadBlob:   reader.DownloadPresigned,
				// 🔴 The presigned UPLOAD is credential-free for the same
				// reason the download is (AGENTS.md items 17 and 19(e)): the upload
				// URL is server-supplied and lives on a *.civitai.com host that
				// isTrustedDownloadHost matches, so a token-carrying client
				// would hand a full-scope personal API key to a request its own
				// signature already authorizes. `reader` is passed as a
				// civitai.PresignedUploader, whose only method takes no token.
				uploadImage: func(ctx context.Context, contentType string, body []byte) (string, error) {
					return gen.UploadImageBlob(ctx, reader, contentType, body)
				},
				buzzBalance: func(ctx context.Context) (int64, error) {
					acct, err := buzz.GetBuzzAccount(ctx)
					if err != nil {
						return 0, err
					}
					return acct.Total(), nil
				},
			}
			// Bind SIGINT for the whole run: the poll loop, the sleep between
			// polls and every blob transfer all take this context, so Ctrl-C
			// unblocks promptly, writePart's defer removes any partial file,
			// and runGenerate's recovery path prints the re-attach command for
			// the job that was already paid for.
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			cmd.SetContext(ctx)
			return runGenerate(cmd, deps, o)
		},
	}

	// EXACTLY five content flags. Everything else the graph accepts is either a
	// silently-degrading money lever (--steps / --cfg-scale / --sampler: a zero is
	// ACCEPTED and prices a cheaper, broken job; an unknown sampler is ignored) or
	// a price multiplier best not exposed before there is a way to echo what the
	// server actually used. --model is deliberately absent: `civitai download`
	// already uses --model for a MODEL id, and this takes a VERSION id.
	cmd.Flags().StringVar(&o.negativePrompt, "negative-prompt", "", "negative prompt")
	cmd.Flags().IntVar(&o.quantity, "quantity", 0, "number of images to generate (server default when unset; no -n shorthand, it reads as \"no\")")
	cmd.Flags().StringVar(&o.aspectRatio, "aspect-ratio", "", "aspect ratio bucket, e.g. 1:1 (width/height derive from it)")
	cmd.Flags().IntVar(&o.checkpoint, "checkpoint", 0, "checkpoint model-VERSION id (not a model id) — resolved before submitting")
	cmd.Flags().StringArrayVar(&o.loras, "lora", nil, "LoRA model-version id, optionally :strength (e.g. 250712:0.8). Repeatable")
	// NOTE: no back-quotes in these usage strings — see the pflag UnquoteUsage
	// note further down.
	cmd.Flags().StringArrayVar(&o.images, "image", nil,
		"reference image for image-to-image: a local file (png or jpeg, uploaded) or an https URL (passed through). "+
			"Repeatable. Requires --ecosystem, and only some ecosystems accept reference images at all")
	cmd.Flags().StringVar(&o.ecosystem, "ecosystem", "",
		"model family to generate with, e.g. Qwen or Flux1Kontext. Sent to the server verbatim and NOT checked locally; "+
			"required with --image because the server only promotes a job to image-to-image when the ecosystem is stated")

	cmd.Flags().BoolVar(&o.dryRun, "dry-run", false, "print the cost estimate and exit without submitting (spends nothing)")
	cmd.Flags().BoolVar(&o.jsonOut, "json", false, "emit the raw server payload on stdout (scriptable)")
	cmd.Flags().BoolVarP(&o.assumeYes, "yes", "y", false, "skip the confirmation and submit (required in a non-interactive shell)")
	// NOTE: no back-quotes in this usage string — pflag's UnquoteUsage treats the
	// first back-quoted span as the flag's VALUE NAME.
	cmd.Flags().IntVar(&o.maxCost, "max-cost", 0,
		"refuse to submit if the ESTIMATE exceeds this many Buzz. This is an estimate check, NOT a spending cap: "+
			"the estimate is not binding, the server enforces no ceiling, and the realized charge can be higher — this flag cannot claw that back")
	// 🔴 OPT-IN, AND IT HAS TO BE. The server substitutes rather than rejects ON
	// PURPOSE: a script pinned to a checkpoint version that was later retired
	// keeps producing images instead of breaking. Making that a hard CLI failure
	// by default would unilaterally override a deliberate server-side degradation
	// and break working automation on upgrade. The default is therefore to WARN —
	// loudly, before the confirmation prompt, where a human can still say no —
	// and this flag is for callers who would rather fail than get a different
	// model. It mirrors --max-cost: an opt-in pre-flight refusal that spends
	// nothing. See AGENTS.md item 21(c).
	//
	// NOTE: no back-quotes in this usage string — see the note above.
	// 🔴 THE USAGE STRING MUST STATE THE SERVER-VERSION CONDITION. This flag can
	// only refuse what the server REPORTS, and a server predating the report omits
	// the field entirely — so against an older deployment the flag is silently
	// inert: exit 0, submitted, charged. Someone adopting it as a spend guard
	// deserves to read that here rather than discover it from a bill.
	cmd.Flags().BoolVar(&o.failOnSubstitution, "fail-on-substitution", false,
		"refuse to submit if the server REPORTS it substituted a different checkpoint for the one you asked for. "+
			"Checked against the ESTIMATE, so nothing is spent when it refuses. Off by default: the server substitutes "+
			"deliberately so that a script pinned to a retired version keeps working. "+
			"NOT A GUARANTEE: a server that does not report substitutions makes this flag silently inert, so it "+
			"cannot be relied on as a spend guard against an older deployment")

	// NOTE: no back-quotes in ANY usage string here — pflag's UnquoteUsage
	// treats the first back-quoted span as the flag's VALUE NAME. Quoting a
	// command in this bool flag's usage rendered it in --help as a flag that
	// takes a value called "civitai workflows get <id>" (measured, then fixed).
	// Use single quotes. Pinned by TestGenerateFlagUsageHasNoBackquotes.
	cmd.Flags().BoolVar(&o.noWait, "no-wait", false,
		"submit, print the workflow id and exit without waiting; collect the results later with 'civitai workflows get <id>'")
	// 🔴 The wording here is load-bearing. --timeout bounds the CLI's WAIT. The
	// job keeps running and the charge stands, so any phrasing that hints at
	// "stop the generation" or "cap the spend" is a lie the user pays for.
	cmd.Flags().DurationVar(&o.timeout, "timeout", defaultWaitTimeout,
		"how long to WAIT for the generation to finish (e.g. 5m, 0 waits indefinitely). This stops the CLI waiting; "+
			"it does NOT stop the generation and does NOT stop the charge — the job continues server-side to completion")
	cmd.Flags().StringVar(&o.outDir, "out-dir", ".",
		"directory to write the generated files into (created if needed); named <workflow-id>-<n>.<ext> unless --out-name says otherwise")
	// NOTE: no back-quotes in this usage string either — see the note above.
	cmd.Flags().StringVar(&o.outName, "out-name", "",
		"template for each output's file name inside --out-dir, e.g. 'img-{n}{ext}'. Placeholders: {workflow} (the workflow id), "+
			"{n} (1-based output number), {ext} (the extension, WITH its leading dot); everything else is literal. "+
			"Default '{workflow}-{n}{ext}'. It names a plain file: a path separator or '..' is REFUSED, not stripped, and the "+
			"template is checked before anything is submitted. A template that would name two outputs the same is refused before "+
			"anything is downloaded, so include {n} for a batch")
	cmd.Flags().BoolVar(&o.noDownload, "no-download", false,
		"wait for the result and print the output URLs, but write no files")
	cmd.Flags().BoolVar(&o.force, "force", false, "overwrite existing output files instead of refusing")
	cmd.Flags().StringVar(&o.externalID, "external-id", "",
		"re-attach to an earlier submit by reusing its idempotency key (the orchestrator dedupes on it and returns the "+
			"PRE-EXISTING workflow rather than charging again). Use the key recorded before the lost submit")

	// NOTE: no back-quotes in these usage strings either — see the note above.
	cmd.Flags().StringVar(&o.inputPath, "input", "",
		"read the generation graph from a JSON file ('-' for stdin) and send it as-is, instead of building one from flags. "+
			"txt2img only. Cannot be combined with the content flags above")
	cmd.Flags().BoolVar(&o.printInput, "print-input", false,
		"print the exact generation graph that would be sent and exit without submitting. Redirect it to a file, edit it, "+
			"and feed it back with --input")
	return cmd
}

// graphInputFlags are the content flags --input replaces. They are listed here
// so the mutual-exclusion error can name exactly which one the user passed,
// following validateDownloadFlags: reject the combination rather than invent a
// merge rule. (What would --lora mean against a file that already declares
// `resources`? Append, or replace? There is no answer the user can predict, and
// guessing wrong costs them a generation.)
func graphInputFlags(o generateOpts) []string {
	var used []string
	if strings.TrimSpace(o.prompt) != "" {
		used = append(used, "a prompt argument")
	}
	if o.negativePrompt != "" {
		used = append(used, "--negative-prompt")
	}
	if o.quantitySet {
		used = append(used, "--quantity")
	}
	if o.aspectRatio != "" {
		used = append(used, "--aspect-ratio")
	}
	if o.checkpointSet {
		used = append(used, "--checkpoint")
	}
	if len(o.loras) > 0 {
		used = append(used, "--lora")
	}
	if len(o.images) > 0 {
		used = append(used, "--image")
	}
	if o.ecosystem != "" {
		used = append(used, "--ecosystem")
	}
	return used
}

// The stand-in workflow id and blob URL --out-name is rendered against before a
// run is submitted. They are only ever used to prove the TEMPLATE renders to a
// contained plain file name; nothing is written and no request is made.
const (
	outNameProbeWorkflowID = "wf_probe"
	outNameProbeURL        = "https://example.invalid/probe.jpeg"
)

// validateGenerateOpts rejects impossible invocations BEFORE any network call,
// the way validateDownloadFlags does — every failure here is a local mistake, so
// it is a usage error (exit 2).
func validateGenerateOpts(o *generateOpts) error {
	usingInput := strings.TrimSpace(o.inputPath) != ""
	if usingInput {
		// Mutual exclusion, not a merge. Execution flags (--dry-run, --yes,
		// --max-cost, --out-dir, --out-name, --timeout, --no-wait, --json, --force,
		// --external-id) stay valid: they govern HOW the request is made and
		// what happens to the result, and none of them writes to the graph.
		if used := graphInputFlags(*o); len(used) > 0 {
			return asUsageError(fmt.Errorf(
				"--input cannot be combined with %s — --input sends the graph in the file exactly as written, so a flag that would also set graph content has no defined meaning against it. "+
					"Either drop %s, or edit the file (start from `civitai generate \"a cat\" --print-input`)",
				strings.Join(used, ", "), plural(len(used), "it", "them")))
		}
	} else if strings.TrimSpace(o.prompt) == "" {
		return asUsageError(errors.New(
			"a prompt is required — quote it as one argument: civitai generate \"a cat wearing sunglasses\" (or pass a graph with --input <file>)"))
	}
	if o.quantitySet && o.quantity < 1 {
		// The server would CLAMP this to 1 without saying so. Refuse instead: a
		// user who typed 0 did not mean 1, and they would be charged for it.
		return asUsageError(fmt.Errorf("--quantity must be at least 1, got %d", o.quantity))
	}
	if o.checkpointSet && o.checkpoint < 1 {
		return asUsageError(fmt.Errorf(
			"--checkpoint must be a positive model-VERSION id, got %d — find it in a model's URL (…/models/<model-id>?modelVersionId=<version-id>)", o.checkpoint))
	}
	if o.maxCostSet && o.maxCost < 0 {
		return asUsageError(fmt.Errorf("--max-cost must not be negative, got %d", o.maxCost))
	}
	if o.jsonOut && !o.dryRun && !o.assumeYes {
		// --json is a scripting path; pairing it with an interactive money prompt
		// invites a script that hangs or, worse, one whose operator clicks through.
		// Say so up front rather than at the prompt.
		return asUsageError(errors.New(
			"--json without --dry-run submits and spends Buzz — pass --yes to confirm non-interactively, or --dry-run to only estimate"))
	}
	if o.timeout < 0 {
		return asUsageError(fmt.Errorf("--timeout must not be negative, got %s (use 0 to wait indefinitely)", o.timeout))
	}
	if o.noWait && o.noDownload {
		// --no-wait already downloads nothing; accepting both would imply the
		// two do different things.
		return asUsageError(errors.New("--no-wait already exits before any result exists — drop --no-download"))
	}
	if strings.TrimSpace(o.outDir) == "" {
		// The flag default is ".", so an empty value can only come from a
		// programmatic construction (or `--out-dir ""`). Resolve it to the
		// documented default rather than erroring: an empty path would silently
		// become a relative write anyway.
		o.outDir = "."
	}
	// 🔴 The --out-name template is checked HERE, before the estimator, the
	// submit and any charge. A template that cannot render is a local mistake,
	// and discovering it after the generation has been billed leaves the user
	// holding presigned URLs and no files.
	//
	// Probe values stand in for the real ones because every placeholder is
	// bounded: {workflow} is a sanitised basename, {n} is a decimal integer and
	// {ext} is blobExtension's output, so nothing a real run substitutes can turn
	// a template that renders safely here into one that escapes there. It goes
	// through planOutputTarget — the same function the download loop uses — so
	// the pre-spend answer and the on-disk answer cannot drift apart.
	if o.outName != "" {
		if _, err := planOutputTarget(o.outName, o.outDir, outNameProbeWorkflowID, 1, outNameProbeURL); err != nil {
			return asUsageError(err)
		}
	}
	for _, raw := range o.loras {
		if _, err := parseLoraFlag(raw); err != nil {
			return err
		}
	}
	if err := validateImageOpts(o); err != nil {
		return err
	}
	return nil
}

// validateImageOpts holds the two --image rules that must fire BEFORE any
// network call — before an upload, and long before a submit.
//
// 🔴 Rule 1: --image REQUIRES --ecosystem. This is a refusal, not a warning,
// because without it the flag is a guaranteed no-op that still charges. The
// server promotes `txt2img` + images to `img2img:edit` in
// `normalizeImageWorkflow`, which reads `ecosystem` off the RAW request body
// BEFORE the graph applies its own ecosystem default — so an absent ecosystem
// skips the promotion, the default ecosystem's graph has no `images` node, and
// the graph engine DROPS the unknown key with zero diagnostics. Measured against
// civitai.com: `{workflow:txt2img, images:[<real image>]}` with no ecosystem
// priced 8 with factors {base,pixels,steps,quantity} — byte-identical to the
// same graph carrying no images at all, HTTP 200 throughout.
//
// This is not the vendored validation item 13 forbids: it checks a FLAG
// COMBINATION this CLI owns, and asserts nothing about which ecosystem values
// exist or what any of them allows. `--ecosystem` itself is passed through
// unexamined.
//
// 🔴 Rule 2: refuse above maxReferenceImages. See that constant for why the
// bound is a single global ceiling rather than a per-ecosystem table, and why
// exceeding it cannot be left to the server (it truncates silently and bills the
// truncated job).
func validateImageOpts(o *generateOpts) error {
	if len(o.images) == 0 {
		return nil
	}
	if strings.TrimSpace(o.ecosystem) == "" {
		return asUsageError(errors.New(
			"--image requires --ecosystem — the server only turns a job into image-to-image when the request names an ecosystem. " +
				"Without one it silently ignores the images, generates from the prompt alone and charges you for it. " +
				"Pass an ecosystem that supports image editing, e.g. --ecosystem Flux1Kontext or --ecosystem NanoBanana"))
	}
	if len(o.images) > maxReferenceImages {
		return asUsageError(fmt.Errorf(
			"--image was given %d times, above the %d-image maximum any ecosystem accepts — nothing was uploaded and nothing was submitted. "+
				"The server does not reject an over-limit list: it DROPS the extras silently and charges for the truncated job. Pass at most %d",
			len(o.images), maxReferenceImages, maxReferenceImages))
	}
	return nil
}

// loraSpec is one parsed --lora value.
type loraSpec struct {
	versionID int
	// strength is nil when the user gave none, so the field is ABSENT from the
	// payload and the server applies its own default.
	strength *float64
}

// parseLoraFlag parses `<version-id>` or `<version-id>:<strength>`. Anything else
// is a usage error — a silently-mis-parsed strength is a money mistake, since the
// server accepts whatever it is handed.
func parseLoraFlag(raw string) (loraSpec, error) {
	v := strings.TrimSpace(raw)
	bad := func(why string) error {
		return asUsageError(fmt.Errorf("invalid --lora %q — %s; expected <version-id> or <version-id>:<strength>, e.g. 250712 or 250712:0.8", raw, why))
	}
	if v == "" {
		return loraSpec{}, bad("it is empty")
	}
	idPart, strengthPart, hasStrength := strings.Cut(v, ":")
	id, err := strconv.Atoi(strings.TrimSpace(idPart))
	if err != nil {
		return loraSpec{}, bad("the id is not an integer")
	}
	if id < 1 {
		return loraSpec{}, bad("the id must be positive")
	}
	if !hasStrength {
		return loraSpec{versionID: id}, nil
	}
	s := strings.TrimSpace(strengthPart)
	if s == "" {
		return loraSpec{}, bad("the strength is missing after ':'")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return loraSpec{}, bad("the strength is not a number")
	}
	return loraSpec{versionID: id, strength: &f}, nil
}

// resolvedGraph is the built payload plus the human labels the confirmation
// needs, so the user approves NAMES rather than integers.
type resolvedGraph struct {
	graph      genapi.Graph
	checkpoint string
	// checkpointNote annotates the checkpoint label when the server has already
	// said it will not use it. It is filled in AFTER the estimate (the only
	// place the substitution record exists) from ONE call site, so the confirm
	// prompt and the --dry-run quote cannot disagree about it — see
	// substitutionCheckpointNote.
	checkpointNote string
	loras          []string
	// images renders the resolved reference images (dimensions + final URL) for
	// the confirmation, so the user approves what will actually be sent rather
	// than the paths they typed — a local file's URL is a stored blob by then.
	images []string
	// inputPath is set when the graph came from --input, so the confirmation can
	// name the FILE the user is about to be charged for rather than a prompt it
	// deliberately did not parse out of it.
	inputPath string
}

// buildInputGraph loads, validates and wraps a `--input` graph.
//
// It performs NO network calls: the graph is passed through verbatim, so there
// are no model-version ids for the CLI to resolve — and resolving them would
// require parsing a structure the CLI has deliberately declined to interpret.
//
// 🔴 The cost of that: the --checkpoint/--lora path's protection against a
// nonexistent id (accepted with HTTP 200, ecosystem default silently
// substituted, billed) does NOT extend to --input. That is inherent to a
// passthrough, not an oversight, and the warning below says so out loud rather
// than letting the flag look as safe as the flags it replaces.
func buildInputGraph(cmd *cobra.Command, o generateOpts) (*resolvedGraph, error) {
	data, err := readGraphInput(cmd.InOrStdin(), o.inputPath)
	if err != nil {
		return nil, err
	}
	parsed, err := parseGraphInput(data)
	if err != nil {
		return nil, err
	}
	errw := cmd.ErrOrStderr()
	if w := unknownKeyWarning(parsed.unknownKeys); w != "" {
		fmt.Fprintln(errw, ui.For(errw).Warn(w))
	}
	return &resolvedGraph{
		graph:     genapi.Graph{Raw: parsed.raw},
		inputPath: o.inputPath,
	}, nil
}

// buildGenerateGraph resolves every model-version id and assembles the graph.
//
// 🔴 The resolution is not a nicety. Graph resources REQUIRE the model type,
// which is not derivable from a version id, and a nonexistent checkpoint id is
// accepted by the generator with HTTP 200 and the ecosystem default substituted
// and BILLED. Resolving first turns that into a hard local 404 — before any
// spend, and before the cost estimate the user is shown.
func buildGenerateGraph(ctx context.Context, deps generateDeps, o generateOpts) (*resolvedGraph, error) {
	out := &resolvedGraph{graph: genapi.Graph{
		// 🔴 Always "txt2img", even with --image. The server promotes it to
		// img2img:edit itself; sending an img2img workflow value is a DIFFERENT
		// and worse request — see AGENTS.md item 19(a).
		Workflow:       generateWorkflow,
		Ecosystem:      strings.TrimSpace(o.ecosystem),
		Prompt:         o.prompt,
		NegativePrompt: o.negativePrompt,
		AspectRatio:    o.aspectRatio,
	}}
	if o.quantitySet {
		out.graph.Quantity = genapi.Ptr(o.quantity)
	}

	if o.checkpointSet {
		rv, err := deps.resolveVersion(ctx, o.checkpoint)
		if err != nil {
			return nil, fmt.Errorf("--checkpoint %d: %w", o.checkpoint, err)
		}
		out.graph.Model = genapi.Ptr(rv.Resource(nil))
		out.checkpoint = describeVersion(rv, nil)
	}
	for _, raw := range o.loras {
		spec, err := parseLoraFlag(raw)
		if err != nil {
			return nil, err
		}
		rv, err := deps.resolveVersion(ctx, spec.versionID)
		if err != nil {
			return nil, fmt.Errorf("--lora %s: %w", raw, err)
		}
		out.graph.Resources = append(out.graph.Resources, rv.Resource(spec.strength))
		out.loras = append(out.loras, describeVersion(rv, spec.strength))
	}
	// Reference images last: it is the only step that can UPLOAD, so a bad
	// --checkpoint / --lora id still fails without having stored anything.
	imgs, err := resolveImages(ctx, deps, o.images)
	if err != nil {
		return nil, err
	}
	out.graph.Images = imgs
	// Label each image with the SOURCE the user typed, not just the blob URL it
	// became: after upload every entry is an opaque blob URL on a *.civitai.com
	// host (observed: orchestration-new.civitai.com), so a confirmation listing
	// only those cannot be matched back to the files on disk — which is the whole
	// point of showing them before a spend.
	// resolveImages preserves order, so imgs[i] is o.images[i].
	for i, img := range imgs {
		src := ""
		if i < len(o.images) {
			src = safeTerm(o.images[i]) + " "
		}
		out.images = append(out.images, fmt.Sprintf("%s(%dx%d) → %s", src, img.Width, img.Height, safeTerm(img.URL)))
	}
	return out, nil
}

// buildGraphForRun picks the graph source: a `--input` passthrough, or the five
// content flags. validateGenerateOpts has already proved the two were not
// combined.
func buildGraphForRun(ctx context.Context, cmd *cobra.Command, deps generateDeps, o generateOpts) (*resolvedGraph, error) {
	if strings.TrimSpace(o.inputPath) != "" {
		return buildInputGraph(cmd, o)
	}
	return buildGenerateGraph(ctx, deps, o)
}

// printAssembledGraph writes the graph that WOULD be sent, indented, to stdout.
//
// It prints the graph and nothing else — not the tRPC envelope. The envelope's
// siblings (externalId, tips, tags) are CLI-owned and are refused on the way
// back in (see envelopeOnlyKeys), so printing them would emit a document that
// `--input` then rejects. What comes out of --print-input must be a valid
// --input; that round-trip is the feature.
func printAssembledGraph(out io.Writer, g genapi.Graph) error {
	raw, err := json.Marshal(g)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return err
	}
	buf.WriteByte('\n')
	_, err = out.Write(buf.Bytes())
	return err
}

// describeVersion renders a resolved version for human output. The name is
// server-origin text, so it goes through safeTerm.
func describeVersion(rv *genapi.ResolvedVersion, strength *float64) string {
	s := fmt.Sprintf("%s (%s, id %d)", safeTerm(rv.DisplayName()), safeTerm(rv.ModelType), rv.VersionID)
	if strength != nil {
		s += fmt.Sprintf(", strength %s", strconv.FormatFloat(*strength, 'g', -1, 64))
	}
	return s
}

// runGenerate is the testable core: validate, resolve, estimate, gate, submit.
//
// The order matters and is a spend-safety property: nothing that could charge
// the user runs until the ids resolve, the estimate is in hand, --max-cost has
// been honoured, the balance has been compared, and the user (or --yes) has
// agreed.
func runGenerate(cmd *cobra.Command, deps generateDeps, o generateOpts) error {
	if err := validateGenerateOpts(&o); err != nil {
		return err
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	out, errw := cmd.OutOrStdout(), cmd.ErrOrStderr()

	if o.quantitySet && o.quantity > serverQuantityClamp {
		// Warn, never block: the clamp is a server fact that can change, and the
		// silent version of this is a user paying for 10 images after asking for 40.
		fmt.Fprintln(errw, ui.For(errw).Warn(fmt.Sprintf(
			"--quantity %d is above the server's current limit (%d). The server CLAMPS silently — you will get %d, and the estimate below already prices that.",
			o.quantity, serverQuantityClamp, serverQuantityClamp)))
	}

	if note := imageCountNote(len(o.images)); note != "" {
		// Printed BEFORE the uploads, so the caveat is visible even if a later
		// step fails — and because it describes what the estimate below cannot
		// tell you.
		fmt.Fprintln(errw, ui.For(errw).Warn(note))
	}

	built, err := buildGraphForRun(ctx, cmd, deps, o)
	if err != nil {
		return err
	}

	if o.printInput {
		// 🔴 Exit here, BEFORE the estimator and long before the submit. This is
		// the documented way to graduate from flags to a file
		// (`--print-input > g.json` → edit → `--input g.json`), and it replaces
		// the `--set` path-expression surface that was cut: a wrong type in a
		// hand-written --set expression is accepted silently by the server and
		// billed, whereas editing a printed file is inspectable before it is
		// sent.
		//
		// Second honest caveat: with --image it also UPLOADS each local file
		// first, because the graph it prints must reference a real stored blob
		// to be a valid --input. That spends no Buzz (an upload is not a
		// charge), but it is a network write, so --print-input is not purely
		// local when --image is present.
		//
		// One honest caveat on "no network call": it reaches NO money seam —
		// not the submit, not the estimator, not the balance read. But the graph
		// is printed AFTER buildGraphForRun, so `--checkpoint`/`--lora` still
		// perform their public model-version READ (free, unauthenticated-capable,
		// spends nothing). That is deliberate: the resolution supplies
		// `model.type`, which graph `resources[]` REQUIRE — a bare `{id}` is
		// rejected 400 "expected object, received undefined". Skipping it would
		// make --print-input emit a document that --input then cannot submit,
		// destroying the round-trip that is the whole feature. With no
		// --checkpoint/--lora there is no request of any kind.
		//
		// That last sentence is what the credential gate in RunE now honours:
		// `--print-input` without `--image` needs no token, because nothing on
		// this path sends one. Issue #257 — the command used to refuse offline.
		return printAssembledGraph(out, built.graph)
	}

	quote, rawQuote, err := deps.whatIf(ctx, built.graph)
	if err != nil {
		return classifyGenerateError(err)
	}
	if quote == nil || quote.Cost == nil {
		// A cost-less estimate must never render as "free". Refuse rather than
		// invent a number the user would approve.
		return fmt.Errorf("the server returned no cost estimate — refusing to submit a generation whose price is unknown")
	}

	// 🔴 BEFORE THE MONEY, AND BEFORE EVERY EARLY RETURN BELOW. This is the only
	// moment in the whole command where the user still has the choice: the
	// estimate has answered, nothing has been submitted, and the confirmation
	// prompt has not yet been shown. Reporting here covers --dry-run, --dry-run
	// --json and the full submit path from ONE call site, because the estimate
	// runs unconditionally on all three.
	//
	// It writes to stderr, so a --json stdout stays machine-clean.
	reportModelSubstitutions(errw, quote.ModelSubstitutions, substitutionAtEstimate)
	substitutionFlagHint(errw, o, quote.ModelSubstitutions)

	// 🔴 AND THE SUMMARY BELOW MUST NOT PRESENT THE SUPERSEDED CHECKPOINT AS THE
	// ONE THAT WILL RUN. The warning above scrolls; the summary is what the user
	// reads at `Generate? [y/N]`, and its last model line was the name the server
	// has already discarded. Computed HERE, from one call site, so the confirm
	// prompt and the --dry-run quote annotate identically — the two-renderers
	// mistake generate_charge_test.go exists to remember.
	built.checkpointNote = substitutionCheckpointNote(built.checkpoint, o, quote.ModelSubstitutions, substitutionAtEstimate)

	if o.dryRun {
		if o.jsonOut {
			// Raw passthrough: a script sees every field, including ones this CLI
			// does not model.
			if err := writeRawJSON(out, rawQuote); err != nil {
				return err
			}
		} else {
			printGenerateQuote(out, errw, built, o, quote)
		}
		// The refusal comes AFTER the output, not instead of it: a --dry-run is a
		// pre-flight, and a caller that asked for the estimate should still get
		// the estimate it asked for before being told the answer is unacceptable.
		return substitutionRefusal(o, quote.ModelSubstitutions)
	}

	if err := substitutionRefusal(o, quote.ModelSubstitutions); err != nil {
		return err
	}

	if !quote.Ready {
		// `ready:false` means some job in the workflow has no available support —
		// the resources are not currently available. Submitting anyway spends on
		// a job the server has already said it cannot serve.
		return civitai.Tag(civitai.ErrBadRequest, fmt.Errorf(
			"the server reports the resources for this job are not currently available (ready: false) — they may be unavailable or unsupported by this workflow; inspect the estimate with --dry-run --json"))
	}

	cost := quote.Cost.Total
	if o.maxCostSet && cost > float64(o.maxCost) {
		return asUsageError(fmt.Errorf(
			"estimated cost %s Buzz exceeds --max-cost %d — nothing was submitted. Raise --max-cost or lower --quantity (remember --max-cost only checks this ESTIMATE; it is not a cap the server enforces)",
			buzzAmount(cost), o.maxCost))
	}

	balance, balanceKnown := int64(0), false
	if b, berr := deps.buzzBalance(ctx); berr != nil {
		// Advisory: a balance we cannot read must not block a user who can
		// generate — but it must never be shown as a number either, or an
		// unreadable balance reads as "you have 0".
		fmt.Fprintln(errw, ui.For(errw).Warn(fmt.Sprintf(
			"could not read your Buzz balance (%v) — continuing without the balance check; verify with `civitai buzz`", berr)))
	} else {
		balance, balanceKnown = b, true
	}
	if balanceKnown && cost > float64(balance) {
		return civitai.Tag(ErrInsufficientBuzz, fmt.Errorf(
			"estimated cost %s Buzz exceeds your balance of %d — nothing was submitted; buy Buzz at %s/purchase/buzz",
			buzzAmount(cost), balance, strings.TrimRight(o.baseURL, "/")))
	}

	if err := confirmGenerate(cmd, o, built, cost, balance, balanceKnown); err != nil {
		return err
	}

	// 🔴 The idempotency key is minted HERE, not inside the submit, so it can be
	// RECORDED BEFORE the request leaves. The charge happens server-side the
	// moment the orchestrator accepts the workflow; a process that dies during
	// the POST has already spent the money and, without this record, has no
	// handle to what it bought.
	externalID := strings.TrimSpace(o.externalID)
	if externalID == "" {
		var mintErr error
		if externalID, mintErr = genapi.NewExternalID(); mintErr != nil {
			return mintErr
		}
	}
	statePath, stateErr := writeSubmitRecord(deps, o, built.graph, externalID)
	if stateErr != nil {
		// Never fatal — a user who can generate must not be blocked by a config
		// directory problem. But say it BEFORE the spend, because the crash-
		// recovery net is what is missing, not a cosmetic feature.
		fmt.Fprintln(errw, ui.For(errw).Warn(fmt.Sprintf(
			"could not write the crash-recovery record (%v) — if this run is interrupted mid-submit you will have to find the job at %s/generate. Your idempotency key is %s",
			stateErr, strings.TrimRight(o.baseURL, "/"), externalID)))
	}

	result, externalID, rawSubmit, err := deps.submit(ctx, built.graph, genapi.SubmitOptions{ExternalID: externalID})
	if err != nil {
		if genapi.StatusOf(err) == 0 {
			// No HTTP status means no interpretable reply: the request may well
			// have reached the orchestrator and been charged. Never let this
			// read as "nothing happened".
			//
			// 🔴 THIS WARNING MUST NEVER BE SUPPRESSED, and a cancelled context is
			// NOT grounds to suppress it. An earlier revision returned early on
			// ctx.Err() != nil, reasoning that a cancel means the request never
			// left the process. That is true only for the microseconds before the
			// POST; Ctrl-C during the round trip cancels the SAME context, and
			// that window is up to submitTimeout (see genapi.Client.submitClient)
			// — a slow orchestrator hop being exactly why a user reaches for
			// Ctrl-C. So the guard traded a harmless false warning in a
			// microsecond window for SILENCE in a two-minute window where the
			// charge is real, and silence here strands the user: this is the only
			// surface that hands back the externalId, and nothing reads the
			// crash-recovery record. Compare the wait loop's cancel path, which
			// routes to printReattach (this file, called from waitAndCollect) and
			// says the job "has already been charged" — the precedent points the
			// other way.
			//
			// We cannot observe whether the bytes were written, so the message
			// carries the uncertainty instead of the control flow resolving it.
			lead := "the submit got no answer from the server"
			if ctxErr := ctx.Err(); ctxErr != nil {
				lead = "the submit was interrupted before any answer arrived"
			}
			fmt.Fprintln(errw, ui.For(errw).Warn(fmt.Sprintf(
				"%s — it MAY still have been accepted and charged. Do NOT simply re-run it; re-attach with:\n    civitai generate %s --external-id %s",
				lead, reattachInvocation(o), externalID)))
		}
		return classifyGenerateError(err)
	}

	// 🔴 REPORTED AGAIN, FROM THE SUBMIT'S OWN RECORD, AND THIS IS NOT A DUPLICATE
	// OF THE ESTIMATE WARNING ABOVE. That one said "nothing has been charged yet";
	// this one is the receipt, and it is authoritative for what was actually
	// billed — the server builds it from the validation it performed on THIS
	// request rather than the estimate's.
	//
	// 🔴 IT SITS HERE, AT THE REPLY, NOT IN A RENDERER. There are TWO submit
	// renderers — printSubmitResult on the --no-wait path and printSubmitted on
	// the waiting path — and putting a money-relevant line in only one of them is
	// a mistake this repo has already made and written a whole test file about
	// (generate_charge_test.go: `Charged:` had one call site the waiting path
	// never reached). One call site covers --no-wait, the waiting path, and
	// --json alike, and emitSubmitHandle keeps it that way.
	workflowID := ""
	if result != nil {
		workflowID = result.ID
	}
	if statePath != "" && workflowID != "" {
		// Best-effort: the id is about to be printed anyway.
		_ = recordPendingWorkflowID(statePath, workflowID)
	}

	// 🔴 THE HANDLE GOES OUT BEFORE THE ADVISORY, NOT AFTER IT. By this point the
	// job is CHARGED and the workflow id is the user's only way back to what they
	// paid for; the substitution report explains a charge that has already
	// happened. Nothing advisory may sit between the reply and the handle — an
	// enrichment added to the report later (resolving the substituted ids to
	// NAMES through ResolveModelVersion, say) would inherit getWithRetry's 4
	// attempts on a 30s-timeout client and could hold the id for minutes, for a
	// cosmetic gain, on a job whose money is already gone. It is the same
	// judgement substitutionAdvice already makes on the decoding side — an
	// unfamiliar reason still reports the ids rather than failing the record —
	// applied to time instead of parsing. Ordering is pinned by
	// TestGenerate_SubmitHandleReachesTheUserBeforeTheSubstitutionAdvisory.
	terminal, handleErr := emitSubmitHandle(out, errw, o, result, workflowID, externalID, statePath, rawSubmit)
	reportModelSubstitutions(errw, result.Substitutions(), substitutionAfterSubmit)
	if handleErr != nil || terminal {
		return handleErr
	}
	return waitAndCollect(ctx, cmd, deps, o, workflowID, externalID, statePath)
}

// emitSubmitHandle prints the handle on a job that has ALREADY BEEN CHARGED, and
// reports whether the command is finished (nothing left to wait for).
//
// It is one function rather than two call sites for the reason the comment above
// gives: every branch here — --no-wait, --json, a reply with no workflow id, and
// the waiting path — has to emit the handle BEFORE any advisory output, and a
// second call site is how one of those branches quietly stops doing that.
//
// The --json branch returns the write error rather than swallowing it, and the
// caller still emits the substitution advisory afterwards: a broken stdout is no
// reason to withhold the record of what was billed.
func emitSubmitHandle(out, errw io.Writer, o generateOpts, result *genapi.SubmitResult, workflowID, externalID, statePath string, rawSubmit json.RawMessage) (bool, error) {
	if o.noWait || workflowID == "" {
		// Nothing more can be done without a handle to poll, so the record has
		// served its purpose the moment the id reaches the user.
		if workflowID != "" {
			clearSubmitRecord(statePath)
		}
		if o.jsonOut {
			return true, writeRawJSON(out, rawSubmit)
		}
		printSubmitResult(out, errw, result, externalID, o.baseURL, o.noWait)
		return true, nil
	}

	var submitCost *genapi.WorkflowCost
	if result != nil {
		submitCost = result.Cost
	}
	printSubmitted(errw, workflowID, externalID, o.baseURL, submitCost)
	return false, nil
}

// writeSubmitRecord writes the pre-submit crash-recovery record and returns its
// path. It resolves the directory from deps (a test points it at a t.TempDir()).
func writeSubmitRecord(deps generateDeps, o generateOpts, g genapi.Graph, externalID string) (string, error) {
	dir := deps.pendingDir
	if dir == "" {
		var err error
		if dir, err = pendingDir(); err != nil {
			return "", err
		}
	}
	return writePendingGeneration(dir, pendingGeneration{
		ExternalID:  externalID,
		SubmittedAt: nowRFC3339(),
		PayloadHash: graphPayloadHash(g),
		BaseURL:     o.baseURL,
	})
}

// clearSubmitRecord removes a record whose workflow id has been reported to the
// user — from that point the workflow id IS the handle, and keeping the file
// would just accumulate clutter that looks like unfinished work.
func clearSubmitRecord(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}

// waitAndCollect polls the submitted workflow to a terminal status, then reports
// and (unless --no-download) downloads its deliverable outputs.
//
// Every early exit here — timeout, Ctrl-C, a failed workflow — is an ERROR
// return, never a success: the user paid, and a zero exit code would tell a
// script the images are on disk.
func waitAndCollect(ctx context.Context, cmd *cobra.Command, deps generateDeps, o generateOpts, workflowID, externalID, statePath string) error {
	out, errw := cmd.OutOrStdout(), cmd.ErrOrStderr()

	cfg := deps.poll
	cfg.timeout = o.timeout
	rep := newPollReporter(errw, cfg.now, cfg.heartbeat)

	wf, rawWF, err := pollWorkflow(ctx, deps.getWorkflow, workflowID, cfg, rep)
	if err != nil {
		switch {
		case errors.Is(err, errWaitTimeout):
			printReattach(errw, o, workflowID, externalID, statusOrUnknown(wf),
				fmt.Sprintf("Stopped waiting after %s.", o.timeout))
			return fmt.Errorf("%w after %s — the generation is still running and has already been charged; it was not cancelled", errWaitTimeout, o.timeout)
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			printReattach(errw, o, workflowID, externalID, statusOrUnknown(wf),
				"Interrupted.")
			return fmt.Errorf("interrupted while waiting — the generation is still running and has already been charged; it was not cancelled")
		}
		return classifyGenerateError(err)
	}

	// The workflow id is now the durable handle and has been printed; the
	// pre-submit record is no longer load-bearing.
	clearSubmitRecord(statePath)

	if !strings.EqualFold(wf.Status, genapi.StatusSucceeded) {
		// failed / expired / canceled.
		//
		// 🔴 THIS USED TO READ "it was charged and produced no usable result",
		// i.e. it asserted the charge STANDS. That claim is contradicted by the
		// platform's own first-party copy for exactly this status set:
		// `civitai/civitai → src/components/ImageGeneration/QueueItem.tsx`
		// comments "the orchestrator auto-refunds spent buzz on these" over
		// `failed || expired || canceled` and renders "Your Buzz has been
		// refunded." It is also contradicted by measurement — across 29 submits
		// (21 succeeded, 8 failed) two balance reads 25 minutes apart moved by
		// 11x8 and 21x8, the SUCCESS count both times, not the submit count.
		//
		// 🔴 AND IT STILL DOES NOT SAY "IT WAS REFUNDED", though the reason has
		// CHANGED and the old reason is retracted. It used to read that the
		// orchestrator is "not part of the civitai monorepo" so "nothing
		// readable states whether it is full, pro-rated, or conditional" —
		// true of the monorepo, and false as a claim about the world: the
		// orchestrator is its own repo (`civitai/civitai-orchestration`) and
		// #307 read it. Failed/cancelled jobs are re-priced by the share of
		// their outputs that never landed and the difference is refunded
		// (traced in runWorkflowsCancel).
		//
		// What has NOT changed is what this sentence may promise. The rule is
		// now known; the AMOUNT is not, because it depends on how many blobs
		// landed before the run died — and this CLI still never sees the Buzz
		// ledger, so it cannot confirm any figure it might quote. It therefore
		// states the outcome and names the command that can answer the money
		// question. See AGENTS.md item 28.
		return fmt.Errorf("the generation finished with status %q and produced no usable result; %s. Inspect the run with `civitai workflows get %s` (%s)",
			safeTerm(wf.Status), buzzLedgerUnknownNote, safeTerm(workflowID), noFailureReasonNote)
	}

	kept, excluded := genapi.PartitionOutputs(wf)
	reportExcludedOutputs(errw, excluded)
	requested := 0
	if o.quantitySet {
		requested = o.quantity
		if requested > serverQuantityClamp {
			// The server clamps silently, so the honest expectation is the
			// clamped number, not what was typed.
			requested = serverQuantityClamp
		}
	}
	reportOutputCountMismatch(errw, requested, len(kept))

	if len(kept) == 0 {
		return fmt.Errorf("the generation succeeded but produced no deliverable outputs — it was charged; inspect it with `civitai workflows get %s` (%s)",
			safeTerm(workflowID), noFailureReasonNote)
	}

	if o.noDownload {
		printOutputURLs(out, errw, kept)
	} else {
		// With --json the machine-readable payload owns stdout, so the "Saved …"
		// lines move to stderr rather than corrupting it.
		saveW := out
		if o.jsonOut {
			saveW = errw
		}
		paths, derr := downloadOutputs(ctx, deps.downloadBlob, saveW, errw, workflowID, kept, o.outDir, o.outName, o.force)
		if derr != nil {
			if len(paths) > 0 {
				fmt.Fprintln(errw, ui.For(errw).Warn(fmt.Sprintf("%d of %d output(s) were saved before this failed", len(paths), len(kept))))
			}
			fmt.Fprintln(errw, ui.For(errw).Dim(fmt.Sprintf(
				"Output URLs expire — re-read the workflow for fresh links: civitai workflows get %s", workflowID)))
			return derr
		}
	}

	if o.jsonOut {
		return writeRawJSON(out, rawWF)
	}
	return nil
}

// printOutputURLs renders the --no-download listing.
func printOutputURLs(out, errw io.Writer, kept []genapi.Output) {
	for i, o := range kept {
		if o.URL != nil {
			fmt.Fprintf(out, "%d\t%s\n", i+1, safeTerm(*o.URL))
		}
	}
	fmt.Fprintln(errw, ui.For(errw).Dim(
		"These URLs are presigned and expire — fetch them promptly, or re-read the workflow for fresh links."))
}

// printSubmitted announces a submitted job on the DEFAULT (waiting) path. It
// writes to stderr so a --json stdout stays machine-clean.
//
// 🔴 It must print the REALIZED charge whenever the server reported one. The
// user approved an ESTIMATE, and this command's whole spend story is that the
// realized cost can exceed it and nothing local can claw that back (AGENTS.md
// items 12-17, and item 28 on why we do not assert what the ledger does) — so the
// one number that settles what actually happened cannot be visible only on the
// --no-wait branch. `cost` is nil when the server sent none; say nothing then
// rather than printing a fabricated 0.
func printSubmitted(errw io.Writer, workflowID, externalID, baseURL string, cost *genapi.WorkflowCost) {
	st := ui.For(errw)
	fmt.Fprintln(errw, st.Success(fmt.Sprintf("Generation submitted — workflow %s", safeTerm(workflowID))))
	if cost != nil {
		fmt.Fprintln(errw, st.Info(fmt.Sprintf("Charged %s Buzz", buzzAmount(cost.Total))))
	}
	fmt.Fprintln(errw, st.Dim(fmt.Sprintf(
		"External ID %s · watch it at %s/generate · re-attach any time with `civitai workflows get %s`",
		externalID, strings.TrimRight(baseURL, "/"), safeTerm(workflowID))))
}

// printReattach is the recovery block for a wait that ended without a result.
//
// 🔴 It must never suggest the job stopped. The CLI gave up WAITING; the
// orchestrator kept generating and the Buzz is gone either way. Everything here
// exists so the user can get to what they already paid for.
func printReattach(errw io.Writer, o generateOpts, workflowID, externalID, status, lead string) {
	st := ui.For(errw)
	fmt.Fprintln(errw, st.Warn(fmt.Sprintf(
		"%s The generation was NOT cancelled — it is still running server-side and has already been charged.", lead)))
	fmt.Fprintf(errw, "  Workflow ID: %s\n", safeTerm(workflowID))
	fmt.Fprintf(errw, "  External ID: %s\n", externalID)
	fmt.Fprintf(errw, "  Last status: %s\n", safeTerm(status))
	fmt.Fprintf(errw, "  Re-attach:   civitai workflows get %s\n", safeTerm(workflowID))
	fmt.Fprintln(errw, st.Dim(fmt.Sprintf("Or watch it at %s/generate", strings.TrimRight(o.baseURL, "/"))))
}

// confirmGenerate gates the spend. It mirrors confirmSubmit, with a stronger
// case: `app submit` gates a REVERSIBLE action and still prompts, while this
// charges Buzz that cannot be refunded.
//
//   - --yes/-y            → proceed without prompting.
//   - non-TTY without --yes → REFUSE (never hang, never charge silently).
//   - interactive TTY     → show cost + balance, prompt, proceed only on "y".
//
// Everything it prints goes to STDERR so a `--json` stdout stays machine-clean.
// printImageDisclosure states what the cost estimate structurally cannot.
//
// 🔴 It must reach EVERY surface that precedes a spend decision — the TTY
// prompt, `--yes`, and `--dry-run` alike. An ecosystem with no images node
// drops the array silently and bills a plain txt2img, and nothing in the whatIf
// reply distinguishes that from a real edit job (measured: Flux1Kontext,
// NanoBanana and Seedream all price identically with and without images, so a
// price comparison cannot discriminate). An earlier revision printed this only
// on the interactive path, which is exactly backwards: `--yes` is MANDATORY
// non-interactively, and `--dry-run` is the documented price-check-first
// workflow, so the two surfaces that most need the caveat were the two that
// never showed it.
func printImageDisclosure(errw io.Writer, o generateOpts, built *resolvedGraph) {
	if o.ecosystem != "" {
		fmt.Fprintf(errw, "  Ecosystem:  %s\n", safeTerm(o.ecosystem))
	}
	for _, img := range built.images {
		fmt.Fprintf(errw, "  Image:      %s\n", img)
	}
	if len(built.images) > 0 {
		fmt.Fprintln(errw, ui.For(errw).Dim(
			"If this ecosystem does not support image editing, the server IGNORES the images above, generates from the prompt alone and still charges — the estimate cannot show the difference."))
	}
}

func confirmGenerate(cmd *cobra.Command, o generateOpts, built *resolvedGraph, cost float64, balance int64, balanceKnown bool) error {
	if o.assumeYes {
		// --yes skips the PROMPT, never the disclosure: a CI run is precisely
		// where nobody is watching for a silently-dropped image.
		printImageDisclosure(cmd.ErrOrStderr(), o, built)
		return nil
	}
	if !stdinIsTTY() {
		return fmt.Errorf("refusing to spend Buzz without --yes in a non-interactive shell — this would charge %s Buzz with nobody to confirm it. "+
			"Pass --yes to confirm, --max-cost <buzz> to refuse above an estimate, or --dry-run to price it without submitting",
			buzzAmount(cost))
	}

	errw := cmd.ErrOrStderr()
	st := ui.For(errw)
	fmt.Fprintf(errw, "About to generate with %s at %s.\n", generateWorkflow, strings.TrimRight(o.baseURL, "/"))
	if built.inputPath != "" {
		// With --input the CLI has deliberately not interpreted the graph, so it
		// names the file rather than echoing fields it did not parse. Printing a
		// partial summary would imply the CLI had checked the rest.
		fmt.Fprintf(errw, "  Graph:      %s (sent as-is; this CLI did not interpret it)\n", safeTerm(built.inputPath))
	} else {
		fmt.Fprintf(errw, "  Prompt:     %s\n", safeTerm(o.prompt))
	}
	if o.negativePrompt != "" {
		fmt.Fprintf(errw, "  Negative:   %s\n", safeTerm(o.negativePrompt))
	}
	if o.quantitySet {
		fmt.Fprintf(errw, "  Quantity:   %d\n", o.quantity)
	}
	if built.checkpoint != "" {
		fmt.Fprintf(errw, "  Checkpoint: %s%s\n", built.checkpoint, built.checkpointNote)
	}
	for _, l := range built.loras {
		fmt.Fprintf(errw, "  LoRA:       %s\n", l)
	}
	printImageDisclosure(errw, o, built)
	if balanceKnown {
		fmt.Fprintf(errw, "Cost: %s Buzz (balance %d).\n", buzzAmount(cost), balance)
	} else {
		fmt.Fprintf(errw, "Cost: %s Buzz (balance unknown).\n", buzzAmount(cost))
	}
	// "and is not refunded" was dropped here (#278): it is a claim about EVERY
	// outcome, including the failed one the orchestrator does refund. The
	// deterrent is that submitting is irreversible and the money moves — which
	// is true regardless of what the ledger does afterwards.
	fmt.Fprintln(errw, st.Warn("This SPENDS REAL BUZZ and cannot be undone once submitted."))
	fmt.Fprintln(errw, st.Dim("The cost is an estimate, not a quote — the server enforces no ceiling and the realized charge can be higher."))
	fmt.Fprint(errw, "Generate? [y/N]: ")

	r := bufio.NewReader(cmd.InOrStdin())
	line, _ := r.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	default:
		return errors.New("generation cancelled")
	}
}

// promptNotChecked replaces the prompt text on the --dry-run quote. It is a
// LABEL, not a value: nothing downstream parses it, and it exists so the line
// reports what the estimate did rather than what the user typed.
const promptNotChecked = "<not checked by --dry-run>"

// printGenerateQuote renders the --dry-run breakdown. Factor and fixed keys are
// printed VERBATIM — they are server-owned, and inventing friendlier labels is
// how a vendored mapping starts.
func printGenerateQuote(out, errw io.Writer, built *resolvedGraph, o generateOpts, q *genapi.WhatIfResult) {
	// The img2img disclosure goes to STDERR so a `--dry-run --json` stdout stays
	// machine-clean, but it must still appear: --dry-run is the documented
	// price-check-first workflow, and it is where a silently-dropped image is
	// cheapest to catch.
	printImageDisclosure(errw, o, built)

	// 🔴 The second thing this estimate structurally cannot tell you, and the
	// reason the Prompt line below prints a label instead of the text (#281).
	// `whatIfGraph` STRIPS prompt/negativePrompt before the whatIf call — from a
	// typed graph and from a raw `--input` one alike — because they do not affect
	// cost and the server substitutes its own defaults. That strip is correct and
	// must not change. What was wrong was echoing the prompt back afterwards: the
	// server never saw it, so a `--dry-run` that printed it read as "checked and
	// priced". Measured: a real prompt was refused at submit for its content
	// after a clean `--dry-run`. There is no local repair — a moderation verdict
	// is server-side and the estimate carries no text to judge — so the honest
	// answer is to say what was not checked.
	fmt.Fprintln(errw, ui.For(errw).Dim(
		"The prompt is not sent with the estimate, so --dry-run cannot tell you whether it passes moderation — a submit can still be refused for prompt content."))

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Workflow:\t%s\n", generateWorkflow)
	if built.inputPath != "" {
		fmt.Fprintf(tw, "Graph:\t%s (sent as-is)\n", safeTerm(built.inputPath))
	} else {
		// NOT safeTerm(o.prompt) — see the disclosure above. The interactive
		// confirmation in confirmGenerate DOES echo the real prompt and must keep
		// doing so: that screen precedes an irreversible spend on a graph that
		// really does carry the text. This one describes an estimate that did not.
		fmt.Fprintf(tw, "Prompt:\t%s\n", promptNotChecked)
	}
	if o.negativePrompt != "" {
		// Same strip, same label. Echoing the negative prompt beside a "not
		// checked" prompt would say the negative one HAD been evaluated — it is
		// deleted by the same two lines of `whatIfGraph`. The line still appears
		// when the flag was set, because dropping it silently would leave the user
		// wondering whether --negative-prompt registered at all.
		fmt.Fprintf(tw, "Negative prompt:\t%s\n", promptNotChecked)
	}
	if o.quantitySet {
		fmt.Fprintf(tw, "Quantity:\t%d\n", o.quantity)
	}
	if o.aspectRatio != "" {
		fmt.Fprintf(tw, "Aspect ratio:\t%s\n", safeTerm(o.aspectRatio))
	}
	if built.checkpoint != "" {
		fmt.Fprintf(tw, "Checkpoint:\t%s%s\n", built.checkpoint, built.checkpointNote)
	}
	for _, l := range built.loras {
		fmt.Fprintf(tw, "LoRA:\t%s\n", l)
	}
	// 🔴 NOT "Generatable". The server's `ready` is a RESOURCE-AVAILABILITY flag,
	// not a prediction that the job will produce anything: it is computed as
	// "every job's queuePosition.support === 'available'", and a job carrying no
	// queuePosition at all is SKIPPED, leaving ready true (measured in
	// civitai/civitai → src/server/services/orchestrator/
	// orchestration-new.service.ts, the whatIf reply builder). Labelling it
	// "Generatable" promised a success predicate the flag cannot carry — 8
	// submits across 3 green-lit checkpoints produced 0 outputs (#279). The
	// FALSE direction is still sound and is warned about below; the label is
	// what stops the TRUE direction reading as a guarantee. See AGENTS.md
	// item 28.
	fmt.Fprintf(tw, "Resources ready:\t%t\n", q.Ready)
	_ = tw.Flush()

	fmt.Fprintf(out, "\n%s\n", ui.For(out).Bold("Estimated cost"))
	tw = tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  base\t%s\n", buzzAmount(q.Cost.Base))
	printCostMap(tw, "factors", q.Cost.Factors)
	printCostMap(tw, "fixed", q.Cost.Fixed)
	printCostMap(tw, "tips", q.Cost.Tips)
	fmt.Fprintf(tw, "  total\t%s\n", buzzAmount(q.Cost.Total))
	_ = tw.Flush()

	if !q.Ready {
		// 🔴 Two words are deliberately NOT here, and both were in the first
		// draft. "generatable", because printing it one line under
		// `Resources ready: false` re-teaches the exact equation the rename
		// exists to break (#279). And "a real submit would be refused", because
		// the only refusal this repo can evidence is OUR OWN: `runGenerate`
		// reads the flag and stops. `support !== 'available'` appears once in
		// the server's whatIf reply builder and nowhere on the submit path, and
		// #279's failing checkpoints came back as HTTP 400s rather than as
		// ready:false. Caveating `true` as unobservable while promoting `false`
		// on the same unread evidence is the same mistake twice.
		fmt.Fprintln(errw, ui.For(errw).Warn(
			"The server reports the resources for this job are NOT currently available (ready: false) — this CLI refuses to submit in that state."))
	}
	// The caveat belongs on stderr next to the number, every time. A user who
	// reads only the total will otherwise treat it as a quote.
	fmt.Fprintln(errw, ui.For(errw).Dim(
		"Estimate only — nothing was submitted and nothing was charged. The server returns no binding quote, enforces no spending ceiling, and the realized charge can exceed this figure."))
}

// printCostMap renders one server-owned cost map with its keys sorted for a
// stable rendering (Go map order is randomised).
func printCostMap(w io.Writer, label string, m map[string]float64) {
	if len(m) == 0 {
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(w, "  %s\t\n", label)
	for _, k := range keys {
		fmt.Fprintf(w, "    %s\t%s\n", safeTerm(k), buzzAmount(m[k]))
	}
}

// printSubmitResult reports a completed submit. The workflow id is the ONLY
// handle to a job that has already been paid for, so if the server's reply does
// not carry one, say so loudly and print the idempotency key instead of leaving
// the user with nothing.
func printSubmitResult(out, errw io.Writer, r *genapi.SubmitResult, externalID, baseURL string, noWait bool) {
	if r == nil || r.ID == "" {
		fmt.Fprintln(errw, ui.For(errw).Warn(
			"The generation was submitted but the server's reply carried no workflow id. It has been CHARGED — do not resubmit."))
		fmt.Fprintf(out, "External ID: %s\n", externalID)
		fmt.Fprintln(errw, ui.For(errw).Dim(fmt.Sprintf(
			"Find the job at %s/generate. To re-attach without paying again, re-run with --external-id %s — the orchestrator dedupes on it and returns the SAME workflow.",
			strings.TrimRight(baseURL, "/"), externalID)))
		return
	}
	fmt.Fprintln(out, ui.For(out).Success("Generation submitted"))
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  Workflow ID:\t%s\n", safeTerm(r.ID))
	if r.Status != "" {
		fmt.Fprintf(tw, "  Status:\t%s\n", safeTerm(r.Status))
	}
	if r.Cost != nil {
		fmt.Fprintf(tw, "  Charged:\t%s Buzz\n", buzzAmount(r.Cost.Total))
	}
	fmt.Fprintf(tw, "  External ID:\t%s\n", externalID)
	_ = tw.Flush()
	if noWait {
		fmt.Fprintln(errw, ui.For(errw).Dim(fmt.Sprintf(
			"Not waiting (--no-wait). Collect the results with `civitai workflows get %s`, or watch it at %s/generate",
			safeTerm(r.ID), strings.TrimRight(baseURL, "/"))))
	}
}

// reattachInvocation renders the argument the user must repeat to re-attach to a
// submit whose reply was lost. It has to reflect how the graph was BUILT: a
// --input run has no prompt to quote, and printing an empty one would hand the
// user a command that re-submits a different (and rejected) job.
func reattachInvocation(o generateOpts) string {
	if p := strings.TrimSpace(o.inputPath); p != "" {
		return fmt.Sprintf("--input %s", p)
	}
	return fmt.Sprintf("%q", o.prompt)
}

// buzzAmount renders a Buzz figure: whole numbers without a decimal tail, and a
// fractional one with enough digits to stay honest.
func buzzAmount(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// classifyGenerateError re-classifies a generation failure whose HTTP status
// alone maps to the wrong exit code.
//
// 🔴 It reads genapi.APIError.ServerMessage — the SERVER's own text — never
// err.Error(). The formatted 403 message this CLI builds itself enumerates every
// cause a 403 can have, so matching against it would classify every 403 as
// whichever pattern is checked first.
//
// It REPLACES the error rather than wrapping it, because civitai.TagStatus has
// already attached ErrUnauthorized to a 403 and errors.Is would keep matching it
// through any wrapper — the user-visible text is rebuilt here (server message
// included, sanitised) so nothing is lost.
//
// Message matching is the only discriminator available: tRPC answers all of
// these with the same code, and the only difference is the text. It fails SOFT —
// an unrecognised message keeps today's status-derived classification, so a
// server-side rewording degrades to the current behaviour, never to a wrong one.
func classifyGenerateError(err error) error {
	var apiErr *genapi.APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	msg := strings.ToLower(apiErr.ServerMessage)
	shown := safeTerm(strings.TrimSpace(apiErr.ServerMessage))
	has := func(needles ...string) bool {
		for _, n := range needles {
			if strings.Contains(msg, n) {
				return true
			}
		}
		return false
	}

	switch {
	// Insufficient Buzz. Server-side this is an orchestrator 403 re-thrown as
	// tRPC BAD_REQUEST, so it reaches us as a 400 and would otherwise exit 2
	// ("bad flags") — which is wrong, and the design doc's claim that it exits 3
	// is wrong too. Matched status-agnostically on purpose: the promise is that
	// running out of Buzz NEVER reads as "log in again", whichever status it
	// arrives on. The default server text is "…don't have enough funds…"; a
	// pass-through orchestrator detail is matched by the other needles.
	case has("enough funds", "insufficient funds", "insufficient buzz", "not enough buzz"):
		return civitai.Tag(ErrInsufficientBuzz, fmt.Errorf(
			"not enough Buzz to run this generation: %s — check your balance with `civitai buzz`", shown))

	// Prompt refused by the content audit. 🔴 Never retry: repeated blocked
	// prompts increment a 30-day counter that auto-mutes the account, so a retry
	// loop turns one refusal into a permanent generation ban.
	case has("your prompt was flagged", "prompt was flagged"):
		return civitai.Tag(ErrPromptBlocked, fmt.Errorf(
			"the prompt was refused by content moderation: %s — 🔴 do NOT retry this prompt: repeated blocked attempts get the account muted. Edit the prompt instead", shown))

	// Generation switched off (or member-only) server-side. Arrives as a 400,
	// which would read as a usage error; it is not the caller's invocation.
	// A custom server-configured status message falls through to the 400 default
	// — fail-soft, never a false classification.
	case has("generation is currently disabled", "generation is disabled"):
		return civitai.Tag(ErrGenerationDisabled, fmt.Errorf(
			"generation is currently unavailable server-side: %s — this is not a problem with your credential or your invocation; try again later", shown))

	// Muted account / incomplete onboarding. THESE are the real 403 victims: a
	// bare FORBIDDEN that is byte-identical to a missing scope, which would send
	// a restricted user round the `civitai login` loop forever.
	case has("account has been restricted", "complete the onboarding"):
		return civitai.Tag(ErrAccountRestricted, fmt.Errorf(
			"your account cannot generate right now: %s — re-running `civitai login` will NOT help; resolve it on the website", shown))

	// Unknown ecosystem is thrown server-side as a bare Error, so it arrives as a
	// 500 and would exit 1 as an unclassified server fault. It is a client-side
	// mistake in everything but the status code, so map it to usage (exit 2).
	case apiErr.Status >= 500 && has("unknown ecosystem"):
		return asUsageError(fmt.Errorf("unknown ecosystem: %s", shown))
	}

	// Everything else keeps its status-derived kind. Notably a 400 "<resource> is
	// not enabled for generation" stays civitai.ErrBadRequest -> exit 2 rather
	// than becoming ErrNotFound -> exit 4 (which the design doc floated with a
	// question mark). Reason: exit 4 on this command already means "no such
	// model-version id", produced locally and deterministically by the
	// --checkpoint/--lora resolution before anything is submitted. A resource
	// that resolved fine but is not currently generatable is a different
	// condition — the ids exist, the COMBINATION is not runnable — and folding
	// the two together would make exit 4 unactionable: a script could no longer
	// tell "fix the id" from "pick a different resource".
	return err
}
