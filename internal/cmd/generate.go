package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

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
	dryRun         bool
	jsonOut        bool
	assumeYes      bool
	maxCost        int
	maxCostSet     bool
	baseURL        string
}

func newGenerateCmd() *cobra.Command {
	var o generateOpts

	cmd := &cobra.Command{
		Use:   "generate [prompt]",
		Short: "Generate images from a text prompt (SPENDS BUZZ)",
		Long: `Generate images from a text prompt on Civitai's generator.

🔴 THIS SPENDS REAL BUZZ AND CANNOT BE UNDONE. A submitted generation is charged;
there is no cancel-for-refund and no "undo". Preview the price with --dry-run
first — it calls the server's cost estimator and spends nothing.

CREDENTIAL: generation needs a full-scope PERSONAL API KEY with the AI Services
scopes (create one at https://civitai.com/user/account, then
` + "`civitai login --token <key>`" + `). An OAuth browser login (` + "`civitai login`" + `) does
NOT carry those scopes and is refused. Check yours with ` + "`civitai whoami`" + `.

--max-cost IS AN ESTIMATE CHECK, NOT A SPENDING CAP. The cost this command shows
is an estimate, not a quote: the server's estimator returns no quote id, no
signed price and no expiry — there is nothing to hand back at submit time, and
no server-side ceiling is reachable from an API key at all. The realized charge
can exceed the estimate, and it is not refunded. --max-cost compares the
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

This release submits the job and prints its workflow id; it does not yet wait
for completion or download the results.`,
		Example: `  # Preview the price — spends nothing
  civitai generate "a cat wearing sunglasses" --dry-run

  # The same estimate as JSON, for scripts
  civitai generate "a cat wearing sunglasses" --dry-run --json

  # Generate 4 images, refusing if the estimate exceeds 50 Buzz
  civitai generate "a cat wearing sunglasses" --quantity 4 --max-cost 50

  # A specific checkpoint plus a LoRA at 0.8 strength
  civitai generate "a cat" --checkpoint 128713 --lora 250712:0.8

  # Non-interactive (CI): --yes is required, or the run is refused
  civitai generate "a cat" --yes --max-cost 20`,
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
			if cfg.Token() == "" {
				return civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf(
					"no token configured — generation needs a personal API key: run `civitai login --token <key>` (or set CIVITAI_TOKEN)"))
			}
			o.baseURL = cfg.BaseURL()

			src := auth.New(cfg)
			gen := genapi.NewWithSource(cfg.BaseURL(), src)
			buzz := appapi.NewWithSource(cfg.BaseURL(), src, "")
			deps := generateDeps{
				whatIf:         gen.WhatIfFromGraph,
				submit:         gen.GenerateFromGraph,
				resolveVersion: gen.ResolveModelVersion,
				buzzBalance: func(ctx context.Context) (int64, error) {
					acct, err := buzz.GetBuzzAccount(ctx)
					if err != nil {
						return 0, err
					}
					return acct.Total(), nil
				},
			}
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

	cmd.Flags().BoolVar(&o.dryRun, "dry-run", false, "print the cost estimate and exit without submitting (spends nothing)")
	cmd.Flags().BoolVar(&o.jsonOut, "json", false, "emit the raw server payload on stdout (scriptable)")
	cmd.Flags().BoolVarP(&o.assumeYes, "yes", "y", false, "skip the confirmation and submit (required in a non-interactive shell)")
	// NOTE: no back-quotes in this usage string — pflag's UnquoteUsage treats the
	// first back-quoted span as the flag's VALUE NAME.
	cmd.Flags().IntVar(&o.maxCost, "max-cost", 0,
		"refuse to submit if the ESTIMATE exceeds this many Buzz. This is an estimate check, NOT a spending cap: "+
			"the estimate is not binding, the server enforces no ceiling, and the realized charge can be higher with no refund")
	return cmd
}

// validateGenerateOpts rejects impossible invocations BEFORE any network call,
// the way validateDownloadFlags does — every failure here is a local mistake, so
// it is a usage error (exit 2).
func validateGenerateOpts(o *generateOpts) error {
	if strings.TrimSpace(o.prompt) == "" {
		return asUsageError(errors.New(
			"a prompt is required — quote it as one argument: civitai generate \"a cat wearing sunglasses\""))
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
	for _, raw := range o.loras {
		if _, err := parseLoraFlag(raw); err != nil {
			return err
		}
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
	loras      []string
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
		Workflow:       generateWorkflow,
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
	return out, nil
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

	built, err := buildGenerateGraph(ctx, deps, o)
	if err != nil {
		return err
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

	if o.dryRun {
		if o.jsonOut {
			// Raw passthrough: a script sees every field, including ones this CLI
			// does not model.
			return writeRawJSON(out, rawQuote)
		}
		printGenerateQuote(out, errw, built, o, quote)
		return nil
	}

	if !quote.Ready {
		// `ready:false` means some job in the workflow has no available support —
		// the resources are not currently generatable. Submitting anyway spends on
		// a job the server has already said it cannot serve.
		return civitai.Tag(civitai.ErrBadRequest, fmt.Errorf(
			"the server reports this job is not currently generatable (ready: false) — the selected resources may be unavailable or unsupported by this workflow; inspect the estimate with --dry-run --json"))
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

	result, externalID, rawSubmit, err := deps.submit(ctx, built.graph, genapi.SubmitOptions{})
	if err != nil {
		return classifyGenerateError(err)
	}

	if o.jsonOut {
		return writeRawJSON(out, rawSubmit)
	}
	printSubmitResult(out, errw, result, externalID, o.baseURL)
	return nil
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
func confirmGenerate(cmd *cobra.Command, o generateOpts, built *resolvedGraph, cost float64, balance int64, balanceKnown bool) error {
	if o.assumeYes {
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
	fmt.Fprintf(errw, "  Prompt:     %s\n", safeTerm(o.prompt))
	if o.negativePrompt != "" {
		fmt.Fprintf(errw, "  Negative:   %s\n", safeTerm(o.negativePrompt))
	}
	if o.quantitySet {
		fmt.Fprintf(errw, "  Quantity:   %d\n", o.quantity)
	}
	if built.checkpoint != "" {
		fmt.Fprintf(errw, "  Checkpoint: %s\n", built.checkpoint)
	}
	for _, l := range built.loras {
		fmt.Fprintf(errw, "  LoRA:       %s\n", l)
	}
	if balanceKnown {
		fmt.Fprintf(errw, "Cost: %s Buzz (balance %d).\n", buzzAmount(cost), balance)
	} else {
		fmt.Fprintf(errw, "Cost: %s Buzz (balance unknown).\n", buzzAmount(cost))
	}
	fmt.Fprintln(errw, st.Warn("This SPENDS REAL BUZZ. It cannot be undone and is not refunded."))
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

// printGenerateQuote renders the --dry-run breakdown. Factor and fixed keys are
// printed VERBATIM — they are server-owned, and inventing friendlier labels is
// how a vendored mapping starts.
func printGenerateQuote(out, errw io.Writer, built *resolvedGraph, o generateOpts, q *genapi.WhatIfResult) {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Workflow:\t%s\n", generateWorkflow)
	fmt.Fprintf(tw, "Prompt:\t%s\n", safeTerm(o.prompt))
	if o.negativePrompt != "" {
		fmt.Fprintf(tw, "Negative prompt:\t%s\n", safeTerm(o.negativePrompt))
	}
	if o.quantitySet {
		fmt.Fprintf(tw, "Quantity:\t%d\n", o.quantity)
	}
	if o.aspectRatio != "" {
		fmt.Fprintf(tw, "Aspect ratio:\t%s\n", safeTerm(o.aspectRatio))
	}
	if built.checkpoint != "" {
		fmt.Fprintf(tw, "Checkpoint:\t%s\n", built.checkpoint)
	}
	for _, l := range built.loras {
		fmt.Fprintf(tw, "LoRA:\t%s\n", l)
	}
	fmt.Fprintf(tw, "Generatable:\t%t\n", q.Ready)
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
		fmt.Fprintln(errw, ui.For(errw).Warn(
			"The server reports this job is NOT currently generatable (ready: false) — a real submit would be refused."))
	}
	// The caveat belongs on stderr next to the number, every time. A user who
	// reads only the total will otherwise treat it as a quote.
	fmt.Fprintln(errw, ui.For(errw).Dim(
		"Estimate only — nothing was submitted and nothing was charged. The server returns no binding quote, enforces no spending ceiling, and the realized charge can exceed this figure with no refund."))
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
func printSubmitResult(out, errw io.Writer, r *genapi.SubmitResult, externalID, baseURL string) {
	if r == nil || r.ID == "" {
		fmt.Fprintln(errw, ui.For(errw).Warn(
			"The generation was submitted but the server's reply carried no workflow id. It has been CHARGED — do not resubmit."))
		fmt.Fprintf(out, "External ID: %s\n", externalID)
		fmt.Fprintf(errw, "Find the job at %s/generate\n", strings.TrimRight(baseURL, "/"))
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
	fmt.Fprintln(errw, ui.For(errw).Dim(fmt.Sprintf(
		"Not waiting for the result — this release submits only. Watch it at %s/generate", strings.TrimRight(baseURL, "/"))))
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
