package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MCP server registration: the two Civitai servers, and the merge that puts them
// into an agent's own config file WITHOUT touching anything else in it.
//
// 🔴 MERGE, NEVER OVERWRITE, AND REFUSE RATHER THAN REPAIR. The file this writes
// belongs to the user and usually already holds servers they configured by hand.
// So: every unknown key survives, every existing server survives, and a file that
// does not parse is REFUSED by name instead of being replaced with a valid file
// that has lost their work. A "repair" here is indistinguishable from deletion.

// mcpServer is one server this command registers. Both are registered on every
// run, before login, because registering is cheap and reversible — but they do
// NOT both work without a credential. See Anonymous.
type mcpServer struct {
	// Name is the key the entry is written under, and the name the agent shows.
	Name string
	URL  string
	// What is one clause naming what the server is for, printed in the next-step
	// block and in the `--agent other` paste block.
	What string
	// Check is the `--check` row this server is reported under. Pinned rather
	// than derived: `prompt.md` reads these names out of `--check --json`.
	Check string
	// Anonymous is whether this server answers a credential-free request.
	//
	// 🔴 IT IS PER SERVER, AND THE TWO DISAGREE. This command shipped claiming
	// "both servers' read tools work anonymously" in seven places, and that claim
	// was the justification for the whole header-less design. It is false for one
	// of the two. Measured, no credential, POST `initialize`:
	//
	//	https://mcp.civitai.com/mcp            -> 200, serverInfo civitai-mcp-server
	//	https://orchestration.civitai.com/mcp  -> 401, empty body, NO WWW-Authenticate
	//
	// The missing `WWW-Authenticate` matters twice over: it is why no MCP client
	// can discover an auth flow from that 401 (every
	// `/.well-known/oauth-*` variant 404s as well), and it is why the failure
	// surfaces to a user as a server that simply does not answer.
	//
	// So a VS Code / Zed / `--agent other` / no-token user must be told that HALF
	// of what was registered needs a header. Read this field; never restate a
	// claim about "both servers".
	Anonymous bool
}

var civitaiMCPServers = []mcpServer{
	{
		Name:      "civitai",
		URL:       "https://mcp.civitai.com/mcp",
		What:      "the Civitai site — models, images, articles, your account",
		Check:     "mcp-site",
		Anonymous: true,
	},
	{
		Name:  "civitai-orchestration",
		URL:   "https://orchestration.civitai.com/mcp",
		What:  "the generation orchestrator — workflows and image generation",
		Check: "mcp-orch",
		// 🔴 401 WITHOUT A TOKEN. Not a read/write split: the anonymous
		// `initialize` handshake itself is refused, so nothing on this server is
		// reachable at all until an Authorization header is present.
		Anonymous: false,
	},
}

// anonymousServerNames and authRequiredServerNames split the table so a message
// naming one group cannot drift from the table. They are derived, never listed:
// a third server is described correctly the moment it is added.
func anonymousServerNames() []string    { return mcpServerNames(true) }
func authRequiredServerNames() []string { return mcpServerNames(false) }

func mcpServerNames(anonymous bool) []string {
	var out []string
	for _, srv := range civitaiMCPServers {
		if srv.Anonymous == anonymous {
			out = append(out, srv.Name)
		}
	}
	return out
}

// mcpAnonymityNote is the ONE sentence every header-less surface prints about
// what a credential-free config does and does not reach. Built from the table.
func mcpAnonymityNote() string {
	anon, auth := anonymousServerNames(), authRequiredServerNames()
	switch {
	case len(auth) == 0:
		return "every registered server answers without a credential"
	case len(anon) == 0:
		return "every registered server needs an Authorization header — none of them answers without one"
	default:
		return strings.Join(anon, ", ") + " answers without a credential (models, images, articles); " +
			strings.Join(auth, ", ") + " returns 401 until an Authorization header is present"
	}
}

// mcpAuthCoverage is which of the Civitai entries in the config a run LEAVES ON
// DISK carry a credential — whichever hand wrote it.
//
// 🔴 "THIS RUN WROTE NO HEADER" IS NOT "THE REGISTRATION HAS NO HEADER", AND THE
// OUTPUT USED TO SAY THE SECOND WHILE MEANING THE FIRST. Both mcpAuthReason and
// printAgentSetupAuthNote derived everything from `hasToken` and never looked at
// the merged entry. Measured: run with a token, then re-run in a shell without
// one (CI, a second machine, an expired login). The merge correctly PRESERVES
// `"Authorization": "Bearer ${env:CIVITAI_TOKEN}"` — that is finding 3 of item
// 35 working — and the run then printed "No token is configured, so no
// Authorization header was written" and "civitai-orchestration returns 401 until
// an Authorization header is present". The first sentence is true of the run and
// misleading about the file; the second is a claim about WHAT THE REGISTRATION
// REACHES, and it is simply wrong about the file just written.
//
// So the sentence is derived from the rendered bytes rather than from the gate.
type mcpAuthCoverage struct {
	// With and Without partition civitaiMCPServers by whether that entry, as it
	// will exist on disk, carries a credential. Both empty means the config was
	// never rendered (a refusal, or no file to write) and nothing is claimed.
	With    []string
	Without []string
}

// known reports whether the rendered config was inspected at all.
func (c mcpAuthCoverage) known() bool { return len(c.With)+len(c.Without) > 0 }

// mcpAuthCoverageOf reads the bytes this run will write and reports which of our
// entries end up carrying a credential.
//
// 🔴 IT READS THE RENDERED OUTPUT, NOT THE INPUT, because the output is what the
// user's agent will load — and it is the one artefact both `--dry-run` and the
// real run agree on by construction (renderMCPConfig performs no write).
//
// What counts as "carries a credential" is per format, and both forms are the
// ones this command or a user would actually produce:
//
//   - JSON: a non-empty `Authorization` under the entry's HeadersKey.
//   - TOML: an `EnvBearerKey` assignment, an inline `HeadersKey` line naming
//     Authorization, or an `Authorization` inside a `<table>.<HeadersKey>`
//     sub-table.
//
// 🔴 THE RESIDUAL: a shape neither branch recognises is reported as NO
// credential, which is the conservative direction — it falls back to exactly the
// message that shipped. It is conservative, not correct: a header this function
// cannot see still gets described as absent.
func mcpAuthCoverageOf(data []byte, t agentTarget) mcpAuthCoverage {
	if len(data) == 0 {
		return mcpAuthCoverage{}
	}
	var carries map[string]bool
	if t.Format == formatTOML {
		carries = tomlEntriesWithAuth(data, t)
	} else {
		carries = jsonEntriesWithAuth(data, t)
	}
	if carries == nil {
		return mcpAuthCoverage{}
	}
	var cov mcpAuthCoverage
	for _, srv := range civitaiMCPServers {
		if carries[srv.Name] {
			cov.With = append(cov.With, srv.Name)
			continue
		}
		cov.Without = append(cov.Without, srv.Name)
	}
	return cov
}

func jsonEntriesWithAuth(data []byte, t agentTarget) map[string]bool {
	if t.HeadersKey == "" {
		return map[string]bool{}
	}
	root := map[string]any{}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil
	}
	section, _ := root[t.ServersKey].(map[string]any)
	found := map[string]bool{}
	for _, srv := range civitaiMCPServers {
		entry, _ := section[srv.Name].(map[string]any)
		headers, _ := entry[t.HeadersKey].(map[string]any)
		if v, ok := headers["Authorization"].(string); ok && strings.TrimSpace(v) != "" {
			found[srv.Name] = true
		}
	}
	return found
}

func tomlEntriesWithAuth(data []byte, t agentTarget) map[string]bool {
	blocks, err := parseTOMLBlocks("", string(data))
	if err != nil {
		return nil
	}
	found := map[string]bool{}
	for _, srv := range civitaiMCPServers {
		table := tomlServerTable(t, srv)
		for _, b := range blocks {
			switch b.Name {
			case table:
				for _, line := range b.Lines[1:] {
					key := tomlKeyOnLine(line)
					if t.EnvBearerKey != "" && key == t.EnvBearerKey {
						found[srv.Name] = true
					}
					if t.HeadersKey != "" && key == t.HeadersKey &&
						strings.Contains(strings.ToLower(line), "authorization") {
						found[srv.Name] = true
					}
				}
			case table + "." + t.HeadersKey:
				if t.HeadersKey == "" {
					continue
				}
				for _, line := range b.Lines[1:] {
					if strings.EqualFold(tomlKeyOnLine(line), "Authorization") {
						found[srv.Name] = true
					}
				}
			}
		}
	}
	return found
}

// mcpPreservedAuthNote is the ONE sentence describing a credential the run did
// NOT write and did NOT touch, or "" when there is none to describe. Every
// header-less surface builds from it, so the terminal and `--json` cannot
// disagree about a file they are both describing.
func mcpPreservedAuthNote(cov mcpAuthCoverage) string {
	if !cov.known() || len(cov.With) == 0 {
		return ""
	}
	if len(cov.Without) == 0 {
		return "every Civitai entry already carries an Authorization header this run did not write " +
			"and did not remove, and that is what they authenticate with"
	}
	note := strings.Join(cov.With, ", ") + " already carries an Authorization header this run did not " +
		"write and did not remove; " + strings.Join(cov.Without, ", ") + " has none"
	var needing []string
	for _, name := range cov.Without {
		for _, srv := range civitaiMCPServers {
			if srv.Name == name && !srv.Anonymous {
				needing = append(needing, name)
			}
		}
	}
	if len(needing) > 0 {
		note += " and " + strings.Join(needing, ", ") + " returns 401 until one is present"
	}
	return note
}

// mcpAuthValue is the header VALUE an interpolating agent gets, or "" when this
// run must write no header at all. It is the ONE place the credential rule is
// decided, so the JSON writer, the TOML writer and the `--agent other` paste
// block cannot disagree about it.
//
// 🔴 NO LITERAL CREDENTIAL, EVER — see the block comment above agentTargets. The
// value returned here is a REFERENCE to CIVITAI_TOKEN in the vendor's own
// documented spelling; the caller's `token` argument is consulted only to decide
// WHETHER there is a credential to reference, and is never interpolated into the
// result. That is why the parameter is named `hasToken` rather than `token`: a
// signature that cannot receive the secret cannot leak it.
//
// 🔴 NO PLACEHOLDER, EITHER — the older rule, still live. `Bearer
// <your-token-here>` is a config that looks correct in every file the user can
// read and 401s at request time. A header-less entry is honest — but it is NOT
// fully working: see mcpServer.Anonymous, and mcpAnonymityNote for the sentence
// that says which half of the registration it reaches.
func mcpAuthValue(t agentTarget, hasToken bool) string {
	if !hasToken || t.HeadersKey == "" || t.EnvHeaderSyntax == "" {
		return ""
	}
	return "Bearer " + t.EnvHeaderSyntax
}

// mcpEntry renders ONE server entry for ONE agent.
func mcpEntry(t agentTarget, srv mcpServer, token string) map[string]any {
	m := map[string]any{t.URLKey: srv.URL}
	if t.TypeKey != "" {
		m[t.TypeKey] = t.TypeValue
	}
	if t.EnabledKey != "" {
		m[t.EnabledKey] = true
	}
	// 🔴 NO KEY AT ALL, NOT AN EMPTY ONE. `"headers": {}` reads to a human as a
	// header that was configured and left blank, and to an agent as a header
	// block to send; the absence is the whole signal.
	if v := mcpAuthValue(t, token != ""); v != "" {
		m[t.HeadersKey] = map[string]any{"Authorization": v}
	}
	return m
}

// mcpMalformed is the refusal for a config file that does not parse. It names
// the PATH, because "your MCP config is broken" is unactionable when the CLI
// knows six places it could be — and it deliberately does not offer to fix it.
func mcpMalformed(path string, err error) error {
	return fmt.Errorf("%s does not parse (%v) — fix or move that file and re-run `civitai agent-setup`; "+
		"this command will not rewrite a config it cannot read, because a repair here is indistinguishable "+
		"from deleting whatever you had in it", path, err)
}

// blankJSONComments replaces JSONC comments with spaces so `encoding/json` can
// decode the result.
//
// 🔴 IT BLANKS RATHER THAN DELETES, AND THE LENGTH IS PRESERVED DELIBERATELY —
// a newline inside a `/* */` run is kept as a newline, so every byte offset and
// every line number in the blanked text still points at the same place in the
// original. Nothing depends on that yet; a future splice that preserves comments
// on WRITE would, and a "simplification" to strings.Replace would remove the
// only property that makes it possible.
//
// 🔴 WHAT IT DOES NOT DO IS PRESERVE JSONC ON THE WAY OUT. The merge re-encodes
// through a Go map, so a JSONC file this command writes back loses its comments,
// its trailing commas (blankJSONTrailingCommas is the other pass) and its key
// order. That is a REAL loss and it is reported rather than hidden — see
// jsonMergeDropsFormatting and the change reason built from it.
// The alternative that was rejected: refusing every commented file, which is
// what shipped, and which made `--agent zed` fail for every real Zed install
// (Zed's settings.json opens with a four-line comment block) while taking
// AGENTS.md and CLAUDE.md down with it.
func blankJSONComments(src []byte) []byte {
	out := make([]byte, len(src))
	copy(out, src)
	inString, escaped := false, false
	for i := 0; i < len(out); i++ {
		c := out[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch {
		case c == '"':
			inString = true
		case c == '/' && i+1 < len(out) && out[i+1] == '/':
			for ; i < len(out) && out[i] != '\n'; i++ {
				out[i] = ' '
			}
		case c == '/' && i+1 < len(out) && out[i+1] == '*':
			out[i], out[i+1] = ' ', ' '
			i += 2
			for ; i < len(out); i++ {
				if out[i] == '*' && i+1 < len(out) && out[i+1] == '/' {
					out[i], out[i+1] = ' ', ' '
					i++
					break
				}
				if out[i] != '\n' {
					out[i] = ' '
				}
			}
		}
	}
	return out
}

// decodeAgentJSON decodes an agent's config, tolerating JSONC comments for the
// agents whose own parsers accept them.
//
// 🔴 A STRICT DECODE STAYS STRICT FOR EVERY OTHER AGENT. Tolerating comments in
// a file the agent itself would reject would let this command write a config
// that parses here and not there.
func decodeAgentJSON(path string, raw []byte, t agentTarget) (map[string]any, error) {
	body := raw
	if t.AllowsComments {
		body = blankJSONC(raw)
	}
	root := map[string]any{}
	if len(strings.TrimSpace(string(body))) == 0 {
		return root, nil
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, mcpMalformed(path, err)
	}
	return root, nil
}

// blankJSONTrailingCommas blanks a `,` that only whitespace separates from the
// `}` or `]` closing its container, so `encoding/json` can decode the result.
//
// 🔴 A TRAILING COMMA IS PART OF THE JSONC THESE AGENTS ACCEPT, AND REFUSING ONE
// WAS THIS COMMAND'S OWN STATED FAILURE MODE — refusing while blaming the user,
// on a file their editor authored and reads back happily. Measured on
// `{ "theme": "One Dark", }` as Zed's settings.json: rc 1, "does not parse … fix
// or move that file". Both parsers were read rather than remembered:
//
//   - **Zed** parses settings with `parse_json_with_comments`
//     (`crates/settings_json/src/settings_json.rs`), which is
//     `serde_json_lenient::Deserializer::from_str`; serde_json_lenient accepts
//     `//` and `/* */` comments AND trailing commas by default, with no feature
//     flag — that leniency is the crate's whole purpose.
//   - **VS Code** strips both in `src/vs/base/common/jsonc.ts` before
//     `JSON.parse`: the scanner's fifth capture group is `(,\s*[}\]])` and there
//     is a `.replace(/,\s*([}\]])/g, '$1')` fallback besides.
//
// 🔴 NEITHER EDITOR WAS RUN. The claim rests on those two sources, not on a
// live observation — and **opencode's parser was NOT established**, so its row
// rides on the same `AllowsComments` gate by inference. What bounds that: this
// command writes STRICT JSON back in every case, so tolerating an input form the
// agent would have rejected can never produce a file the agent cannot read. That
// is the asymmetry AllowsComments' own comment worries about, and it does not
// apply in this direction.
//
// It blanks rather than deletes for the same reason blankJSONComments does — the
// byte offsets into the original stay valid.
func blankJSONTrailingCommas(src []byte) []byte {
	out := make([]byte, len(src))
	copy(out, src)
	inString, escaped := false, false
	for i := 0; i < len(out); i++ {
		c := out[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch {
		case c == '"':
			inString = true
		case c == ',':
			for j := i + 1; j < len(out); j++ {
				if out[j] == ' ' || out[j] == '\t' || out[j] == '\r' || out[j] == '\n' {
					continue
				}
				if out[j] == '}' || out[j] == ']' {
					out[i] = ' '
				}
				break
			}
		}
	}
	return out
}

// blankJSONC applies both blanking passes, in the order they compose: comments
// first, so a comment sitting between a trailing comma and its closing brace has
// already become whitespace by the time the comma is judged.
func blankJSONC(src []byte) []byte {
	return blankJSONTrailingCommas(blankJSONComments(src))
}

// jsonMergeDropsFormatting reports whether writing this file back will lose
// something the user wrote that is not a key or a value — a comment, or a
// trailing comma. Used to WARN, never to refuse.
//
// It covers BOTH because the re-encode drops both, and a warning scoped to
// comments alone would say nothing about a file whose only JSONC feature is a
// trailing comma.
func jsonMergeDropsFormatting(raw []byte, t agentTarget) bool {
	if !t.AllowsComments || len(raw) == 0 {
		return false
	}
	return !bytes.Equal(raw, blankJSONC(raw))
}

// mergeEntryKeys overlays this command's keys onto an entry the user may already
// have, instead of replacing the entry wholesale.
//
// 🔴 ASSIGNING THE WHOLE ENTRY DELETES KEYS THE USER ADDED TO **OUR** SERVER.
// The published contract says an existing MCP config is "merged into, preserving
// every other server AND KEY", and that was true for their servers and false for
// ours: a hand-added `Authorization` header — which the no-interpolation branch
// of this very command TELLS Zed users to add — was silently dropped
// on the next run, rc 0, with `--check` still reporting `ok: true`. Measured.
//
// So: start from what is there, and set only the keys this command owns. The one
// nested map that gets the same treatment is the headers object, so an unrelated
// header the user added (`X-Trace-Id`, a proxy key) survives a run that rewrites
// Authorization.
func mergeEntryKeys(existing any, ours map[string]any, headersKey string) map[string]any {
	prev, ok := existing.(map[string]any)
	if !ok {
		return ours
	}
	merged := make(map[string]any, len(prev)+len(ours))
	for k, v := range prev {
		merged[k] = v
	}
	for k, v := range ours {
		if k == headersKey && headersKey != "" {
			if oursHeaders, isMap := v.(map[string]any); isMap {
				merged[k] = mergeHeaderKeys(prev[k], oursHeaders)
				continue
			}
		}
		merged[k] = v
	}
	return merged
}

// mergeHeaderKeys overlays our header(s) onto theirs, keeping every header name
// we do not write.
func mergeHeaderKeys(existing any, ours map[string]any) map[string]any {
	prev, ok := existing.(map[string]any)
	if !ok {
		return ours
	}
	merged := make(map[string]any, len(prev)+len(ours))
	for k, v := range prev {
		merged[k] = v
	}
	for k, v := range ours {
		merged[k] = v
	}
	return merged
}

// mergeJSONMCP merges the servers into `raw`, an agent's existing JSON config
// (empty for a file that does not exist yet), and returns the bytes to write.
//
// Every key it does not own is preserved: the decode is into map[string]any,
// only `t.ServersKey` and the two server names under it are touched, and within
// those two entries only the keys this command writes are assigned
// (mergeEntryKeys). What it does NOT preserve is byte layout — comments, trailing
// commas and key order do not survive a decode/encode round trip through a Go
// map.
func mergeJSONMCP(path string, raw []byte, t agentTarget, token string) ([]byte, error) {
	root, err := decodeAgentJSON(path, raw, t)
	if err != nil {
		return nil, err
	}

	// 🔴 A NON-OBJECT AT THE SERVERS KEY IS REFUSED, NOT REPLACED. `"mcp": []`
	// or `"servers": null` is somebody's file in a shape this CLI does not
	// understand; overwriting it is the "repair" this whole path forbids.
	section := map[string]any{}
	if existing, ok := root[t.ServersKey]; ok && existing != nil {
		m, ok := existing.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s has a %q key that is not an object (%T) — this command merges into "+
				"that object and will not replace it; fix or rename it and re-run `civitai agent-setup`",
				path, t.ServersKey, existing)
		}
		section = m
	}
	for _, srv := range civitaiMCPServers {
		section[srv.Name] = mergeEntryKeys(section[srv.Name], mcpEntry(t, srv, token), t.HeadersKey)
	}
	root[t.ServersKey] = section

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// jsonMCPRegistered reports which of the two servers are present in an agent's
// existing JSON config. A missing file is not an error — nothing is registered.
func jsonMCPRegistered(path string, raw []byte, t agentTarget) (map[string]bool, error) {
	found := map[string]bool{}
	root, err := decodeAgentJSON(path, raw, t)
	if err != nil {
		return nil, err
	}
	section, _ := root[t.ServersKey].(map[string]any)
	for _, srv := range civitaiMCPServers {
		if _, ok := section[srv.Name]; ok {
			found[srv.Name] = true
		}
	}
	return found, nil
}

// ---------------------------------------------------------------------------
// TOML (Codex)
// ---------------------------------------------------------------------------

// The TOML side is hand-rolled and LINE-BASED on purpose.
//
// 🔴 A ROUND TRIP THROUGH A TOML MARSHALLER LOSES COMMENTS AND KEY ORDER, and
// `~/.codex/config.toml` is a hand-maintained file full of both — the model, the
// approval policy, the sandbox settings. Re-emitting it from a decoded value
// would "preserve every key" in the sense a test can assert while destroying the
// file a human actually reads. So instead the file is split into blocks at its
// table headers, the `[mcp_servers.<name>]` blocks this command owns are
// replaced or appended, and every other byte is passed through untouched.
//
// 🔴 THE RESIDUAL, STATED — AND THE PREVIOUS STATEMENT OF IT WAS WRONG ABOUT
// THE EFFECT. It said a header-shaped line inside a multi-line ARRAY would cause
// "a mis-placed block boundary rather than data loss". Measured, it caused a
// HARD REFUSAL: `matrix = [\n  [1, 2],\n]` produced `unterminated table header
// "[1, 2],"` and the run exited non-zero telling the user their valid TOML was
// broken. A legal array of tables (`[[profiles]]` twice) was refused the same
// way, as `table [profiles] is defined twice`, because both headers strip to one
// name. Both are now handled: bracket depth is tracked across lines so a
// continuation line is never read as a header, and `[[…]]` is exempt from the
// duplicate-table rule that only applies to real tables.
//
// What is still NOT understood: nothing this file has been shown to get wrong.
// The parser tracks table headers, `[[array of tables]]` headers, comments,
// multi-line basic/literal strings (`"""` / `'''`) and array nesting depth. A
// full TOML parser remains rejected — it costs a direct dependency and a
// comment-and-order-destroying round trip for a file with six keys in it.

// tomlBlock is one table (or the preamble before the first table), verbatim.
type tomlBlock struct {
	// Name is the table name with quotes stripped, "" for the preamble.
	Name string
	// Array is true when the header was `[[name]]` — an ARRAY OF TABLES entry.
	//
	// 🔴 IT IS NOT A TABLE AND MUST NOT BE TREATED AS ONE. TOML allows any number
	// of `[[profiles]]` headers; that is the whole point of the form. Folding it
	// into Name alone made two legal entries look like one table defined twice,
	// and the duplicate rule then refused the file.
	Array bool
	// Lines are the block's source lines, header included, unmodified.
	Lines []string
}

// parseTOMLBlocks splits a TOML document at its table headers. It returns an
// error only for a header it cannot read — an unterminated `[` — because that IS
// malformed TOML and refusing beats guessing where the block ends.
func parseTOMLBlocks(path string, src string) ([]tomlBlock, error) {
	// 🔴 AN EMPTY SOURCE HAS AN EMPTY PREAMBLE, NOT A ONE-LINE ONE.
	// `strings.Split("", "\n")` returns `[""]`, so without this guard a config
	// file that does not exist yet gets a preamble holding one blank line — and
	// the rendered document then OPENS with a blank line before its first table.
	// Measured on a fresh `~/.codex/config.toml`. Harmless to TOML and wrong in
	// a file a human reads; trimming it afterwards is not the fix, because that
	// would also strip a leading blank line an EXISTING file legitimately has,
	// which is a byte this command has no business touching.
	if src == "" {
		return []tomlBlock{{}}, nil
	}
	blocks := []tomlBlock{{}}
	inMultiline := false
	// 🔴 BRACKET DEPTH IS WHAT SEPARATES A HEADER FROM A CONTINUATION LINE. At
	// depth 0 a line starting with `[` is a table header; inside an open array
	// value it is data, and `[1, 2],` is a perfectly ordinary element. Without
	// this the parser refused valid TOML by name.
	depth := 0
	for _, line := range strings.Split(src, "\n") {
		if inMultiline {
			blocks[len(blocks)-1].Lines = append(blocks[len(blocks)-1].Lines, line)
			if closesTOMLMultiline(line) {
				inMultiline = false
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if depth == 0 && strings.HasPrefix(trimmed, "[") {
			// A trailing comment is legal after a header (`[table] # mine`), so
			// it is stripped before the terminator check rather than making the
			// header look unterminated.
			head := strings.TrimSpace(stripTOMLLineComment(trimmed))
			if !strings.HasSuffix(head, "]") {
				return nil, mcpMalformed(path, fmt.Errorf("unterminated table header %q", trimmed))
			}
			blocks = append(blocks, tomlBlock{
				Name:  tomlTableName(head),
				Array: strings.HasPrefix(head, "[[") && strings.HasSuffix(head, "]]"),
				Lines: []string{line},
			})
			continue
		}
		blocks[len(blocks)-1].Lines = append(blocks[len(blocks)-1].Lines, line)
		if opensTOMLMultiline(trimmed) {
			inMultiline = true
			continue
		}
		depth += tomlBracketDelta(line)
		if depth < 0 {
			depth = 0
		}
	}
	return blocks, nil
}

// stripTOMLLineComment removes a `#` comment from a line, respecting quoting so
// a `#` inside a string is not mistaken for one.
func stripTOMLLineComment(line string) string {
	inBasic, inLiteral, escaped := false, false, false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case escaped:
			escaped = false
		case inBasic && c == '\\':
			escaped = true
		case inBasic:
			if c == '"' {
				inBasic = false
			}
		case inLiteral:
			if c == '\'' {
				inLiteral = false
			}
		case c == '"':
			inBasic = true
		case c == '\'':
			inLiteral = true
		case c == '#':
			return line[:i]
		}
	}
	return line
}

// tomlBracketDelta is a line's net `[` minus `]`, counting only brackets that
// are neither quoted nor commented — i.e. how much deeper into an array value
// the document is when the line ends.
func tomlBracketDelta(line string) int {
	body := stripTOMLLineComment(line)
	delta := 0
	inBasic, inLiteral, escaped := false, false, false
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case escaped:
			escaped = false
		case inBasic && c == '\\':
			escaped = true
		case inBasic:
			if c == '"' {
				inBasic = false
			}
		case inLiteral:
			if c == '\'' {
				inLiteral = false
			}
		case c == '"':
			inBasic = true
		case c == '\'':
			inLiteral = true
		case c == '[':
			delta++
		case c == ']':
			delta--
		}
	}
	return delta
}

// opensTOMLMultiline reports whether a line starts a multi-line string that the
// same line does not close. An odd count of either delimiter leaves it open.
func opensTOMLMultiline(line string) bool {
	if i := strings.Index(line, "#"); i == 0 {
		return false
	}
	return strings.Count(line, `"""`)%2 == 1 || strings.Count(line, "'''")%2 == 1
}

// closesTOMLMultiline reports whether a continuation line ends the open string.
func closesTOMLMultiline(line string) bool {
	return strings.Contains(line, `"""`) || strings.Contains(line, "'''")
}

// tomlTableName strips `[`/`]` (and `[[`/`]]`) and any quoting on the segments,
// so `[mcp_servers."civitai"]` and `[mcp_servers.civitai]` are the same table.
func tomlTableName(header string) string {
	name := strings.TrimSpace(strings.Trim(strings.TrimSpace(header), "[]"))
	parts := strings.Split(name, ".")
	for i, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"'`)
		parts[i] = p
	}
	return strings.Join(parts, ".")
}

// tomlServerTable is the table name one server is registered under.
func tomlServerTable(t agentTarget, srv mcpServer) string {
	return t.ServersKey + "." + srv.Name
}

// renderTOMLServer renders one `[mcp_servers.<name>]` block.
//
// The header quotes the server name so a name needing quoting cannot produce a
// header this same parser would then read differently — the two Civitai names do
// not need it today, and a renderer that only stays correct for today's inputs
// is how the next name breaks it.
//
// 🔴 THE TOML SIDE USES A DIFFERENT MECHANISM, NOT A DIFFERENT SPELLING. Codex
// documents no `${…}` interpolation at all; it takes the NAME of an environment
// variable in `bearer_token_env_var` and does the `Bearer ` prefixing itself. So
// this renders `bearer_token_env_var = "CIVITAI_TOKEN"` and NEVER an
// `http_headers` line, which is documented as static values only — i.e. as the
// literal credential this whole path forbids.
func renderTOMLServer(t agentTarget, srv mcpServer, token string) []string {
	lines := []string{fmt.Sprintf("[%s.%q]", t.ServersKey, srv.Name)}
	lines = append(lines, fmt.Sprintf("%s = %q", t.URLKey, srv.URL))
	if token != "" && t.EnvBearerKey != "" {
		lines = append(lines, fmt.Sprintf("%s = %q", t.EnvBearerKey, tokenEnvVar))
	}
	if v := mcpAuthValue(t, token != ""); v != "" {
		lines = append(lines, fmt.Sprintf("%s = { %q = %q }", t.HeadersKey, "Authorization", v))
	}
	return lines
}

// mergeTOMLBlock rebuilds one of OUR tables: this command's keys, re-rendered,
// followed by every line of the existing block that this command does not own.
//
// 🔴 REPLACING THE BLOCK WHOLESALE DELETED THE USER'S KEYS. Codex server tables
// legitimately carry `startup_timeout_sec`, `tool_timeout_sec`, `enabled`,
// `env` — none of which this command writes and all of which vanished on the
// next run, silently, at rc 0. Same defect as the JSON side (mergeEntryKeys),
// same published contract broken ("preserving every other server AND KEY").
//
// Comments inside the block are carried through with the lines they annotate.
//
// 🔴 A KEY IS SKIPPED ONLY WHEN `ours` ACTUALLY RE-RENDERS IT — AND THE FIRST
// VERSION OF THIS FUNCTION SKIPPED A FIXED SET INSTEAD, WHICH DELETED THE OTHER
// HALF. The set was {URLKey, EnvBearerKey, HeadersKey}, but renderTOMLServer
// emits `http_headers` NEVER (Codex's EnvHeaderSyntax is "", so mcpAuthValue
// returns "") and `bearer_token_env_var` only WITH a token. So a key that is not
// re-rendered was dropped with nothing put back: a Codex user's hand-added
// `http_headers = { Authorization = … }` — the only static-header key Codex
// documents, i.e. the way a Codex user authenticates — vanished on the next run,
// at rc 0, with `--check` still reporting `ok: true`. Measured. This is the same
// defect the JSON side had, one layer down, and it survived the round-1 fix
// because that fix was written against a fixture holding only keys this command
// NEVER writes.
//
// So the owned set is DERIVED from `ours`, which makes it structurally
// impossible for the two to disagree: mergeEntryKeys (JSON) overlays exactly the
// keys present in `ours`, and this is the same rule spelled for lines.
//
// 🔴 THE RESIDUAL, STATED: with a token configured this writes
// `bearer_token_env_var` while leaving a user's `http_headers` Authorization in
// place, so Codex is handed two sources for one header. That is deliberate — it
// is their key, in their file, and deleting it silently is the defect above —
// but it is a state a diff will look surprising in. Item 34's rule is about what
// this command WRITES; it is not a licence to delete what it finds.
func mergeTOMLBlock(b tomlBlock, ours []string) []string {
	// The header line carries no `=`, so tomlKeyOnLine returns "" for it and it
	// contributes nothing to the set.
	ownedKeys := map[string]bool{}
	for _, line := range ours {
		if key := tomlKeyOnLine(line); key != "" {
			ownedKeys[key] = true
		}
	}
	out := append([]string{}, ours...)
	// Skip the header line: `ours` already carries this command's own spelling
	// of it, and emitting the old one too would define the table twice.
	for _, line := range b.Lines[1:] {
		if key := tomlKeyOnLine(line); key != "" && ownedKeys[key] {
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

// tomlKeyOnLine returns the bare key a `key = value` line assigns, or "" when
// the line is not a simple assignment (a comment, a blank, a continuation).
func tomlKeyOnLine(line string) string {
	body := strings.TrimSpace(stripTOMLLineComment(line))
	i := strings.Index(body, "=")
	if i <= 0 {
		return ""
	}
	key := strings.TrimSpace(body[:i])
	if key == "" || strings.ContainsAny(key, " \t") {
		return ""
	}
	return strings.Trim(key, `"'`)
}

// mergeTOMLMCP replaces or appends this command's two tables and returns the
// whole document. Every block it does not own is emitted byte-for-byte.
func mergeTOMLMCP(path string, src string, t agentTarget, token string) ([]byte, error) {
	blocks, err := parseTOMLBlocks(path, src)
	if err != nil {
		return nil, err
	}

	owned := map[string]mcpServer{}
	for _, srv := range civitaiMCPServers {
		owned[tomlServerTable(t, srv)] = srv
	}

	// 🔴 A DUPLICATE TABLE IS REFUSED — BUT AN ARRAY OF TABLES IS NOT A
	// DUPLICATE. TOML forbids defining a table twice, so a file carrying two
	// `[mcp_servers.civitai]` blocks is already broken and picking one to replace
	// would silently change which definition wins. `[[profiles]]` twice is the
	// opposite: it is the DOCUMENTED way to write a list, it is what an array of
	// tables looks like, and refusing it told the user their valid config was
	// broken. Measured.
	seen := map[string]bool{}
	for _, b := range blocks {
		if b.Name == "" || b.Array {
			continue
		}
		if seen[b.Name] {
			return nil, mcpMalformed(path, fmt.Errorf("table [%s] is defined twice", b.Name))
		}
		seen[b.Name] = true
	}

	var out []string
	replaced := map[string]bool{}
	for _, b := range blocks {
		srv, mine := owned[b.Name]
		// An `[[mcp_servers.civitai]]` is somebody else's shape, not our table —
		// rewriting it as a table would change what the file means.
		if !mine || b.Array {
			out = append(out, b.Lines...)
			continue
		}
		// Re-render OUR keys and carry every other key the block held through
		// verbatim (see mergeTOMLBlock). The trailing blank lines it carried are
		// dropped with it and re-added below, so repeated runs are idempotent.
		out = append(out, mergeTOMLBlock(b, renderTOMLServer(t, srv, token))...)
		out = append(out, "")
		replaced[b.Name] = true
	}

	for _, srv := range civitaiMCPServers {
		if replaced[tomlServerTable(t, srv)] {
			continue
		}
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		out = append(out, renderTOMLServer(t, srv, token)...)
		out = append(out, "")
	}

	doc := strings.TrimRight(strings.Join(out, "\n"), "\n")
	if doc == "" {
		return nil, nil
	}
	return []byte(doc + "\n"), nil
}

// tomlMCPRegistered reports which servers already have a table in the document.
func tomlMCPRegistered(path string, src string, t agentTarget) (map[string]bool, error) {
	blocks, err := parseTOMLBlocks(path, src)
	if err != nil {
		return nil, err
	}
	present := map[string]bool{}
	for _, b := range blocks {
		// An `[[mcp_servers.civitai]]` array entry is not the table this command
		// writes, so it does not count as registered.
		if b.Array {
			continue
		}
		present[b.Name] = true
	}
	found := map[string]bool{}
	for _, srv := range civitaiMCPServers {
		if present[tomlServerTable(t, srv)] {
			found[srv.Name] = true
		}
	}
	return found, nil
}

// ---------------------------------------------------------------------------
// The format-agnostic entry points
// ---------------------------------------------------------------------------

// readIfExists reads a file, treating "not there" as empty rather than as a
// failure. Every other read error is returned: an unreadable config is not an
// absent one, and merging over it would destroy it.
func readIfExists(path string) ([]byte, bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return b, true, nil
}

// renderMCPConfig produces the bytes that would be written to an agent's config,
// merged over whatever is there now. It performs NO write, so `--dry-run` and the
// real run render through exactly the same code and cannot disagree.
func renderMCPConfig(path string, t agentTarget, token string) ([]byte, error) {
	raw, _, err := readIfExists(path)
	if err != nil {
		return nil, err
	}
	switch t.Format {
	case formatTOML:
		return mergeTOMLMCP(path, string(raw), t, token)
	default:
		return mergeJSONMCP(path, raw, t, token)
	}
}

// mcpRegisteredServers reports which servers an agent's config already carries.
func mcpRegisteredServers(path string, t agentTarget) (map[string]bool, error) {
	raw, _, err := readIfExists(path)
	if err != nil {
		return nil, err
	}
	if t.Format == formatTOML {
		return tomlMCPRegistered(path, string(raw), t)
	}
	return jsonMCPRegistered(path, raw, t)
}

// writeMCPConfig writes the merged config, creating parent directories.
//
// 🔴 0600 ON A FILE WE CREATE — AND NOT BECAUSE THE ENTRY CARRIES A TOKEN. It
// does not; that is item 34's whole rule, and the comment that used to sit here
// justified the mode with a credential this command stopped writing. The mode
// stays for a different, still-true reason: this file is exactly where the
// no-interpolation branch TELLS a Zed user to paste a literal token by hand, and
// where an authenticated user's own header will end up. Creating it
// world-readable would hand them a 0644 home for a credential they were invited
// to add. So the strict mode is about what the file is FOR, not about what this
// command puts in it.
//
// 🔴 AND THAT IS WHY IT DIFFERS FROM AGENTS.md / CLAUDE.md AT 0644
// (writeProjectFile). Those two are instruction files: they are meant to be
// committed, read by the whole team and by CI, they can never hold a credential,
// and a 0600 AGENTS.md would break a checkout shared between accounts. Two files
// created side by side with different modes looks like an oversight and is a
// decision — the split is credential-adjacency, not scope.
//
// Preserving the mode on an existing file is the other half: tightening
// somebody's config to 0600 behind their back is also a change they did not ask
// for.
func writeMCPConfig(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return writeFileAtomic(path, data, mode)
}

// writeFileAtomic writes via a temp file in the same directory and a rename, so
// an interrupted run cannot leave a half-written config behind — which for these
// files means an agent that no longer starts.
//
// 🔴 A SYMLINKED DESTINATION IS FOLLOWED, NOT REPLACED. A rename onto
// `~/.codex/config.toml -> ~/dotfiles/codex.toml` destroys the link and leaves a
// regular file, with the dotfiles copy orphaned and every future edit to it
// invisible — silently, at rc 0. Measured. Dotfile repositories are the normal
// way these files are managed, so this is the common case, not an edge one, and
// it contradicts this file's whole "the config belongs to the user" posture.
// The destination is therefore resolved first and the write lands on the real
// file, in the real file's directory so the rename stays on one filesystem.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	target, err := resolveWriteTarget(path)
	if err != nil {
		return err
	}
	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(target)+".*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}

// checkWriteTargetResolvable is resolveWriteTarget's refusal WITHOUT the write —
// a pure read (Lstat plus a link resolution), so the planning phase can reach the
// same verdict the write phase would.
//
// 🔴 A REFUSAL ONLY THE WRITE PATH CAN REACH IS INVISIBLE TO `--dry-run`, AND
// THAT MADE THE DRY RUN LIE. Measured on a broken-symlink `~/.codex/config.toml`:
// `--dry-run` reported `create`, `ok: true`, exit 0, for a destination the real
// run refuses by name. The file header's claim that dry-run "cannot report a path
// or an action the write path would not take" was true only for the checks that
// happened to sit in planMCPConfig. Moving this one there makes the claim true of
// it too — and, on the real run, turns a bare error into a `blocked` row.
func checkWriteTargetResolvable(path string) error {
	_, err := resolveWriteTarget(path)
	return err
}

// resolveWriteTarget returns the real path a write to `path` must land on: the
// path itself when it is a regular file or does not exist, and the link's target
// when it is a symlink.
//
// A BROKEN symlink is refused by name rather than materialised into a regular
// file: the user pointed this path somewhere, and creating a file here instead
// silently un-does that.
func resolveWriteTarget(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return path, nil
		}
		return "", err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		dest, readErr := os.Readlink(path)
		if readErr != nil {
			dest = "?"
		}
		return "", fmt.Errorf("%s is a symlink to %s, which cannot be resolved (%v) — this command follows the "+
			"link rather than replacing it with a regular file; fix or remove the link and re-run "+
			"`civitai agent-setup`", path, dest, err)
	}
	return resolved, nil
}

// mcpPasteBlock renders the config an `other` agent's user has to place by hand.
//
// It is the SAME entry builder the file writers use, so the block a human pastes
// cannot drift from the block the CLI writes — which is the failure this command
// exists to prevent, moved from a hosted doc into the binary.
//
// 🔴 IT CARRIES NO AUTHORIZATION HEADER, EVEN WITH A TOKEN CONFIGURED, AND THAT
// IS THE STRICTEST CASE RATHER THAN AN OVERSIGHT. Pasted text lands in a config
// file by hand, so a literal credential here is the same leak with one extra
// step — and worse, the destination agent is by definition one this CLI does not
// know, so no interpolation syntax can be assumed correct for it either. The
// next-step block names the header and the four spellings instead.
func mcpPasteBlock() string {
	// The shape shown is the most widely used one: a `mcpServers` object with
	// `type`/`url`. The next-step block names the per-agent keys that differ,
	// because the ONE thing a paste cannot carry is which key name the reader's
	// own agent wants.
	shape := agentTargets[agentClaude]
	servers := map[string]any{}
	for _, srv := range civitaiMCPServers {
		// The empty token is not a bug: it is what makes this header-less.
		servers[srv.Name] = mcpEntry(shape, srv, "")
	}
	out, err := json.MarshalIndent(map[string]any{shape.ServersKey: servers}, "", "  ")
	if err != nil {
		return ""
	}
	return string(out)
}

// mcpKeyDifferences renders the one-line-per-agent summary of the key names that
// differ, for the `other` path. Sorted so the output is stable.
func mcpKeyDifferences() []string {
	ids := make([]string, 0, len(agentTargets))
	for id := range agentTargets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		t := agentTargets[id]
		key := t.ServersKey
		if t.Format == formatTOML {
			key = "[" + t.ServersKey + ".<name>]"
		}
		out = append(out, fmt.Sprintf("%-9s %-20s url key: %s", id, key, t.URLKey))
	}
	return out
}
