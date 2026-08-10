package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/civitai/cli/internal/genapi"
)

// inputWorkflow is the ONLY workflow `--input` accepts in this release.
//
// 🔴 This gate is a CONTENT-AUDIT guard, not a feature-completeness placeholder.
// The server audits prompts at `'prompt' in data && typeof data.prompt ===
// 'string'` (civitai/civitai ->
// src/server/services/orchestrator/orchestration-new.service.ts:1460), and the
// `data` it tests is REBUILT FROM DECLARED GRAPH NODES ONLY. A graph that
// carries its prompt somewhere other than a top-level `prompt` string node
// therefore reaches the generator with its prompt never having been seen by the
// audit.
//
// 🔴 THAT IS NO LONGER A SUSPICION. An earlier revision of this comment called
// it an OPEN question upstream, and a later session went further and recorded it
// as "ruled out — no known bypass". Both are wrong, and the second is the
// dangerous one: verified against civitai@a7e0bcd668, TWO shipped ecosystems are
// exactly this shape.
//   - Hunyuan3D declares NO `prompt` node — only `hunyuanPrompt`
//     (hunyuan3d-graph.ts:71), prefixed because the bare names "collide with the
//     standard image Controllers in GenerationForm.tsx" (:12). So `data.prompt`
//     is absent, the audit never runs, and the handler maps the name back for
//     the generator: `prompt: hunyuanPrompt ? hunyuanPrompt : undefined`
//     (hunyuan3d-graph.handler.ts:58).
//   - PolyGen's `texturePrompt` (polygen-graph.ts:162) is audited by nothing,
//     reaches the orchestrator (polygen.schema.ts:143), and on `img2model3d` is
//     the only text in the request — that workflow's `prompt` node is gated
//     `when: workflow.startsWith('txt')` and is deleted from `data`.
//
// Neither was introduced deliberately: Hunyuan3D renamed its nodes to dodge a UI
// Controller collision and changed its AUDIT status as a side effect, because
// the two are keyed on the same strings with nothing tying them together.
// Escalated as civitai/civitai#3667.
//
// 🔴 AND THE HONEST FRAMING, because the overclaim is tempting: this CLI is NOT
// what holds the line. Both fields are reachable from the first-party generation
// form, and any personal API key can call the tRPC procedure directly — so the
// gate stops accidents, not adversaries. What it does buy is that we do not add
// a second client to a CONFIRMED unaudited path while it is open. Lift this when
// the server closes the coverage (#3667), not when the next workflow "looks like
// it would work", and not on the argument that the gap is reachable elsewhere
// anyway — that argument is true, and it is an argument for fixing the platform.
const inputWorkflow = generateWorkflow

// envelopeOnlyKeys are the keys the tRPC generate MUTATION destructures out of
// the wrapper AROUND the graph — they are siblings of `input`, never members of
// it (civitai/civitai -> src/server/routers/orchestrator.router.ts,
// `generateFromGraph`).
//
// 🔴 THIS IS A MONEY GUARD. `civitaiTip` and `creatorTip` are real Buzz, and the
// cost pre-flight structurally CANNOT see them: whatIf prices a strictly smaller
// body than a submit and is never sent tips at all, so `--dry-run` on a file
// carrying `civitaiTip: 5000` shows the untipped estimate and the submit charges
// the tip anyway. A `--max-cost` check compares against that same blind
// estimate.
//
// The whole set is listed, not just the two that cost money: `buzzType` selects
// WHICH balance is spent, `externalId` is the idempotency key this CLI mints and
// records itself (a file-supplied one would silently re-attach to — or collide
// with — an unrelated workflow), `tags`/`sourceMetadata`/`sourceMetadataMap`/
// `remixOfId` are provenance the CLI does not author, and a top-level `input`
// means the file is a saved ENVELOPE rather than a graph, which would nest the
// graph one level too deep and price the server's default job instead.
var envelopeOnlyKeys = []string{
	"input",
	"civitaiTip",
	"creatorTip",
	"tags",
	"buzzType",
	"externalId",
	"sourceMetadata",
	"sourceMetadataMap",
	"remixOfId",
}

// parsedGraphInput is a validated `--input` document.
type parsedGraphInput struct {
	// raw is the graph, byte-for-byte as supplied (compacted). It is passed
	// through UNCHANGED so keys this CLI does not model still reach the server —
	// which is the entire point of the flag.
	raw json.RawMessage
	// unknownKeys are top-level keys the CLI's own Graph does not model, sorted.
	// They are a WARNING, never a refusal.
	unknownKeys []string
}

// readGraphInput reads the `--input` document from a file, or from stdin when
// path is "-".
func readGraphInput(stdin io.Reader, path string) ([]byte, error) {
	if strings.TrimSpace(path) == "-" {
		if stdin == nil {
			return nil, asUsageError(fmt.Errorf("--input - reads the graph from stdin, but this command has no stdin attached"))
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read --input from stdin: %w", err)
		}
		return b, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		// 🔴 THE TWO FAILURES SPLIT, AND THE SPLIT IS THE PUBLISHED CONTRACT —
		// not a nicety. A path that is not there, or that is a directory, is a
		// mistake about the INVOCATION and exits 2. A file that IS there and
		// cannot be read (permissions, an I/O error) is a filesystem failure
		// and exits 1: exitCodeDocs says so in as many words under code 2, and
		// resolveLocalImage (generate_image.go) already honours it for
		// `--image`.
		//
		// This used to tag EVERY os.ReadFile failure `asUsageError` under the
		// comment "a missing/unreadable path is the user's mistake, not the
		// server's". Half right, and the wrong half was published: measured on
		// the binary at 592a8a9, `--input <mode-000 file>` exited 2 while
		// `--image <mode-000 png>` exited 1, so the CLI contradicted its own
		// README. The tell that it was an oversight rather than a decision is
		// twelve lines up — the stdin sibling returns its read failure UNTAGGED
		// and always exited 1, so one command answered the same question two
		// ways depending only on whether the graph arrived by file or by pipe.
		if os.IsNotExist(err) {
			return nil, asUsageError(fmt.Errorf(
				"--input %s: no such file — pass a path that exists, or `--input -` to read the graph from stdin; produce a starting point with `civitai generate \"a cat\" --print-input`", path))
		}
		// Only reached on the error path, so the happy path pays nothing for
		// it. os.ReadFile surfaces a directory as an unhelpful read error whose
		// errno is not portable, and a directory is the same invocation mistake
		// the image ledger already publishes as exit 2.
		if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
			return nil, asUsageError(fmt.Errorf(
				"--input %s is a directory, not a generation-graph JSON file", path))
		}
		return nil, fmt.Errorf("read --input %s: %w", path, err)
	}
	return b, nil
}

// parseGraphInput validates a raw `--input` document and returns the graph to
// send plus the keys the CLI could not recognise.
//
// The order of the checks is deliberate: REFUSALS first (malformed, envelope
// keys, non-txt2img), and only then the advisory unknown-key survey. A file that
// is going to be refused must not first be annotated with warnings about it.
func parseGraphInput(data []byte) (*parsedGraphInput, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, asUsageError(fmt.Errorf("--input is empty — expected a generation-graph JSON object, e.g. the output of `civitai generate \"a cat\" --print-input`"))
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, asUsageError(fmt.Errorf(
			"--input is not a JSON object: %w — expected a generation graph like {\"workflow\":\"%s\",\"prompt\":\"…\"}; produce a valid starting point with `civitai generate \"a cat\" --print-input`",
			err, inputWorkflow))
	}

	// 🔴 Envelope keys are REFUSED, never stripped. See the comment on
	// envelopeOnlyKeys for what they cost. A strip-and-warn would be a silent
	// override of an explicit instruction in the user's own file, delivered on a
	// stream (stderr) that an unattended run routinely discards — so the one
	// case that most needs to be seen is the one least likely to be. Refusing is
	// unmissable, costs the user one edit, and cannot be clicked through. It is
	// also this repo's house answer to an impossible flag combination
	// (validateDownloadFlags rejects six of them rather than defining subtle
	// merges).
	var found []string
	for _, k := range envelopeOnlyKeys {
		if _, ok := obj[k]; ok {
			found = append(found, k)
		}
	}
	if len(found) > 0 {
		return nil, asUsageError(fmt.Errorf(
			"--input contains %s, which %s NOT part of the generation graph: %s %s beside the graph in the request envelope, and this CLI owns %s. "+
				"Tips (civitaiTip/creatorTip) are real Buzz that the cost estimate cannot see, so a file setting one would charge more than --dry-run showed. "+
				"Remove %s from the file",
			joinQuoted(found),
			plural(len(found), "is", "are"),
			plural(len(found), "it", "they"),
			plural(len(found), "belongs", "belong"),
			plural(len(found), "it", "them"),
			plural(len(found), "it", "them")))
	}

	// 🔴 txt2img only. See inputWorkflow.
	rawWorkflow, ok := obj["workflow"]
	if !ok {
		return nil, asUsageError(fmt.Errorf(
			"--input has no \"workflow\" key — this CLI only submits %q graphs, and it will not let the server pick a default it cannot verify. Add \"workflow\": %q",
			inputWorkflow, inputWorkflow))
	}
	var workflow string
	if err := json.Unmarshal(rawWorkflow, &workflow); err != nil {
		return nil, asUsageError(fmt.Errorf(
			"--input has a non-string \"workflow\" value (%s) — expected %q", strings.TrimSpace(string(rawWorkflow)), inputWorkflow))
	}
	if workflow != inputWorkflow {
		return nil, asUsageError(fmt.Errorf(
			"--input declares workflow %q, but this CLI only submits %q. Other workflows (img2img, video, comfy, …) are not supported yet: their prompts do not always live in the top-level \"prompt\" node the server's content audit reads, and this CLI will not be the path that bypasses it",
			safeTerm(workflow), inputWorkflow))
	}

	// Advisory only. The CLI reports what IT does not recognise; it does not
	// vendor the server's node registry to decide what is valid (design §5's
	// anti-mirror rule — the platform owns ~51 ecosystem graphs, and a vendored
	// copy would go stale and start refusing valid new nodes).
	known := genapi.KnownGraphKeys()
	var unknown []string
	for k := range obj {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)

	// Compact so the recorded payload hash and the outgoing body do not depend
	// on the file's whitespace.
	compact, err := compactJSON(data)
	if err != nil {
		return nil, asUsageError(fmt.Errorf("--input is not valid JSON: %w", err))
	}
	return &parsedGraphInput{raw: compact, unknownKeys: unknown}, nil
}

// unknownKeyWarning renders the advisory for keys the CLI does not model, or ""
// when there are none.
//
// 🔴 THE WORDING IS A STATEMENT ABOUT THIS CLI'S OWN KNOWLEDGE, AND ABOUT
// NOTHING ELSE. It may not say the key is invalid — that would mirror a node
// registry this CLI deliberately does not vendor (item 13) — and, since #343, it
// may not say the server IGNORES it either. Both are verdicts on the server's
// behaviour, and this CLI has authority over neither.
//
// 🔴 THE SECOND CLAIM WAS DISPROVED ON ITS OWN SCREEN, WHICH IS WHY IT IS GONE.
// This warning used to close with: the server "SILENTLY IGNORES keys it does not
// declare: an unrecognised key returns HTTP 200, prices the same, and simply has
// no effect". Measured on a credentialed run (#343): a graph carrying
// `priority: high` drew exactly that sentence, and three lines below it the
// command's own cost breakdown showed a `fixed → priority 20` component and a
// total of 28 — against 8 for `normal` and 8 for `low`. The key was honoured, it
// more than tripled the price, and the job cleared in ~40s instead of the
// 60–90 minutes the low-priority queue was quoting. So the warning talked the
// user out of the one change that fixed their actual problem, and contradicted
// the CLI's own output doing it.
//
// The defect was a GENERALISATION, not a typo: ONE measurement (`foobar:123`
// priced identically and vanished) was written up as a rule about every
// undeclared key. That is the item-13 mistake in warning form — a vendored claim
// about the server's node registry, just phrased as prose instead of a table.
//
// What survives is only what this CLI can carry on its own authority: the key is
// not modelled HERE, it is sent exactly as written, and what it does — including
// what it costs — is the server's answer. Naming --dry-run is the actionable
// part and is structural rather than asserted: whatIfGraph strips only
// `prompt`/`negativePrompt` from a raw graph (item 15), so an unmodelled key IS
// part of what the estimator prices — which is precisely where #343's
// `priority 20` component appeared. That is not a promise the estimate is a
// quote (item 28(a) — it is not), only a pointer at the one surface where a
// price effect can show BEFORE the money moves.
func unknownKeyWarning(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"--input carries %s that this CLI does not model and therefore did not check: %s. %s sent to the server exactly as written. "+
			"This is NOT a claim that %s invalid, and NOT a claim that the server ignores %s: this CLI does not vendor the server's graph nodes, so \"unrecognised here\" is the whole of what it knows. "+
			"The server decides what %s, including what %s — --dry-run prices the graph with %s included, so the server's own estimate is where a price effect would show. "+
			"Check the spelling against the graph you copied %s from",
		plural(len(keys), "a key", "keys"), joinQuoted(keys),
		plural(len(keys), "It is", "They are"),
		plural(len(keys), "it is", "they are"),
		plural(len(keys), "it", "them"),
		plural(len(keys), "the key does", "the keys do"),
		plural(len(keys), "it costs", "they cost"),
		plural(len(keys), "it", "them"),
		plural(len(keys), "it", "them"))
}

// inputSubstitutionCoverageNote is the advisory for `--input` +
// `--fail-on-substitution`, or "" when they are not both present (#342).
//
// 🔴 IT IS UNCONDITIONAL ON THE FLAGS AND BLIND TO THE FILE. It reads two
// booleans and never looks at the graph. Deciding whether to warn by inspecting
// the file — "does it name a model at the top level, or under resources?" —
// would be exactly the per-key registry item 13 forbids, and it is the ONLY
// reason this note can be worded honestly at all: an advisory that fires
// always makes a claim about the FLAG's coverage, while one that fires
// selectively would be making a claim about the FILE's contents.
//
// # WHY A NOTE AND NOT A REFUSAL
//
// A refusal was drafted first and withdrawn. `substitutionRefusal` reads
// `quote.ModelSubstitutions` — the record returned by the ESTIMATE — so it is
// evaluated BEFORE the submit and refuses for free. That is a working pre-spend
// money guard, and it is live on `--input` today: it keys off the reply, not off
// `o.checkpoint`. Refusing the flag combination would have deleted a guard that
// works in order to close a gap somewhere else. #342's defect was never "the
// flag is broken" — it is that its coverage is PARTIAL AND UNDOCUMENTED, and the
// fix for undocumented is documentation, not amputation.
//
// # WHAT THIS MAY AND MAY NOT SAY — READ BEFORE REWORDING
//
// 🔴 It must NOT say "top-level model substitutions are covered, resources are
// not". That sentence is tempting because item 21(a) uses the words "TOP-LEVEL",
// but item 21(a) is about WHERE IN THE REPLY the record is carried (the reply's
// own top level, versus nested under `metadata`) — a fact about this CLI's
// transport. It says nothing about WHICH MODEL REFERENCE IN THE REQUEST GRAPH
// the server will substitute and report on. Reading the first as the second is a
// per-key claim about the server's graph registry inferred from one paid
// observation, which is #343's mistake wearing a different hat: that warning
// also generalised a single measurement (`foobar:123` vanished) into a rule.
//
// So this note states only: the flag is live, it fires on what the estimate
// reports, this CLI cannot relate that record to an uninterpreted file, and the
// one case that WAS measured produced no record. Item 21(b) supplies the last
// clause — absence is ambiguous, because a server predating the record is
// byte-identical to a server that substituted nothing.
func inputSubstitutionCoverageNote(o generateOpts) string {
	if strings.TrimSpace(o.inputPath) == "" || !o.failOnSubstitution {
		return ""
	}
	return "--fail-on-substitution is LIVE with --input, but this CLI cannot tell you how much of your graph it covers. " +
		"It refuses on the substitution record the ESTIMATE returns, before anything is submitted, so it is a real pre-spend guard. " +
		"What it cannot do here is relate that record to your file: a raw graph is not interpreted, so nothing local knows which model references the file contains. " +
		"Measured once: a checkpoint named under \"resources\" was charged and ran a different version with no record at all, so the flag did not fire. " +
		"Read a silent run as \"nothing was reported\", never as \"nothing was substituted\" — a server predating the record returns exactly the same silence. " +
		"--checkpoint is the path that resolves model ids before anything is submitted"
}

// compactJSON validates and whitespace-normalises a JSON document. json.Compact
// is used rather than a RawMessage round-trip: unmarshalling into a RawMessage
// copies the bytes VERBATIM (interior whitespace and all) and so normalises
// nothing, which would leave the recorded payload hash sensitive to how the file
// happened to be indented.
func compactJSON(data []byte) (json.RawMessage, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, data); err != nil {
		return nil, err
	}
	return json.RawMessage(buf.Bytes()), nil
}

// joinQuoted renders a key list as `"a", "b"`.
func joinQuoted(keys []string) string {
	q := make([]string, 0, len(keys))
	for _, k := range keys {
		q = append(q, fmt.Sprintf("%q", safeTerm(k)))
	}
	return strings.Join(q, ", ")
}

// plural picks the singular or plural form for n.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
