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
		// A missing/unreadable path is the user's mistake, not the server's.
		return nil, asUsageError(fmt.Errorf("read --input %s: %w", path, err))
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
// 🔴 The wording is a statement about THIS CLI'S OWN KNOWLEDGE, not about the
// key's validity, and it must stay one. The CLI has no authority to say whether
// the server accepts a key — it does not vendor the node registry, and the
// platform's ~51 ecosystem graphs declare far more nodes than the five flags
// model. What it CAN say, and what the user needs, is: this key is being sent
// verbatim, nothing here checked it, and the server's failure mode for a key it
// does not declare is to DROP IT SILENTLY at HTTP 200 — measured: `foobar:123`
// returns 200, prices identically, and the parameter simply vanishes. So a typo
// costs money and produces a job that ran without it, with no error anywhere.
func unknownKeyWarning(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"--input carries %s that this CLI does not recognise and therefore cannot verify: %s. %s sent to the server as-is. "+
			"This is NOT a claim that %s invalid — the server declares many more graph nodes than this CLI models. "+
			"But the server SILENTLY IGNORES keys it does not declare: an unrecognised key returns HTTP 200, prices the same, and simply has no effect, so a typo here costs Buzz and produces a job that ran without it. Check the spelling against the graph you copied it from",
		plural(len(keys), "a key", "keys"), joinQuoted(keys),
		plural(len(keys), "It is", "They are"), plural(len(keys), "it is", "they are"))
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
