package cmd

import (
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
// run: the read tools on both work anonymously, so registering them before login
// is useful rather than half-done.
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
}

var civitaiMCPServers = []mcpServer{
	{
		Name:  "civitai",
		URL:   "https://mcp.civitai.com/mcp",
		What:  "the Civitai site — models, images, articles, your account",
		Check: "mcp-site",
	},
	{
		Name:  "civitai-orchestration",
		URL:   "https://orchestration.civitai.com/mcp",
		What:  "the generation orchestrator — workflows and image generation",
		Check: "mcp-orch",
	},
}

// mcpEntry renders ONE server entry for ONE agent.
//
// 🔴 NO PLACEHOLDER TOKEN, EVER. With no token configured the entry is written
// WITHOUT the header — auth here is a Bearer API key, not OAuth, so an entry
// carrying `Bearer <your-token-here>` is a config that fails at request time with
// a 401 and looks correct in every file the user can read. The read tools work
// anonymously, so a header-less entry is genuinely useful; the next-step block
// says what is missing and how to add it.
func mcpEntry(t agentTarget, srv mcpServer, token string) map[string]any {
	m := map[string]any{t.URLKey: srv.URL}
	if t.TypeKey != "" {
		m[t.TypeKey] = t.TypeValue
	}
	if t.EnabledKey != "" {
		m[t.EnabledKey] = true
	}
	if token != "" && t.HeadersKey != "" {
		m[t.HeadersKey] = map[string]any{"Authorization": "Bearer " + token}
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

// mergeJSONMCP merges the servers into `raw`, an agent's existing JSON config
// (empty for a file that does not exist yet), and returns the bytes to write.
//
// Every key it does not own is preserved: the decode is into map[string]any and
// only `t.ServersKey` and the two server names under it are assigned. What it
// does NOT preserve is byte layout — comments and key order do not survive a
// decode/encode round trip through a Go map. That is stated rather than hidden;
// a JSONC settings file with comments fails the DECODE first and is refused by
// name, so the case where a comment would be silently dropped does not arise.
func mergeJSONMCP(path string, raw []byte, t agentTarget, token string) ([]byte, error) {
	root := map[string]any{}
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, &root); err != nil {
			return nil, mcpMalformed(path, err)
		}
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
		section[srv.Name] = mcpEntry(t, srv, token)
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
	if len(strings.TrimSpace(string(raw))) == 0 {
		return found, nil
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, mcpMalformed(path, err)
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
// 🔴 THE RESIDUAL, STATED. This parser understands table headers, comments and
// multi-line basic/literal strings (it tracks `"""` and `'''` so a header-shaped
// line inside one is not mistaken for a header). It does NOT understand a
// header-shaped line inside a multi-line ARRAY value spanning lines. No MCP or
// Codex config plausibly contains one, the effect would be a mis-placed block
// boundary rather than data loss, and the alternative — a full TOML parser —
// costs a direct dependency for a file with six keys in it.

// tomlBlock is one table (or the preamble before the first table), verbatim.
type tomlBlock struct {
	// Name is the table name with quotes stripped, "" for the preamble.
	Name string
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
	for _, line := range strings.Split(src, "\n") {
		if inMultiline {
			blocks[len(blocks)-1].Lines = append(blocks[len(blocks)-1].Lines, line)
			if closesTOMLMultiline(line) {
				inMultiline = false
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			if !strings.HasSuffix(trimmed, "]") {
				return nil, mcpMalformed(path, fmt.Errorf("unterminated table header %q", trimmed))
			}
			blocks = append(blocks, tomlBlock{Name: tomlTableName(trimmed), Lines: []string{line}})
			continue
		}
		blocks[len(blocks)-1].Lines = append(blocks[len(blocks)-1].Lines, line)
		if opensTOMLMultiline(trimmed) {
			inMultiline = true
		}
	}
	return blocks, nil
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
func renderTOMLServer(t agentTarget, srv mcpServer, token string) []string {
	lines := []string{fmt.Sprintf("[%s.%q]", t.ServersKey, srv.Name)}
	lines = append(lines, fmt.Sprintf("%s = %q", t.URLKey, srv.URL))
	if token != "" && t.HeadersKey != "" {
		lines = append(lines, fmt.Sprintf("%s = { %q = %q }", t.HeadersKey, "Authorization", "Bearer "+token))
	}
	return lines
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

	// 🔴 A DUPLICATE TABLE IS REFUSED. TOML forbids defining a table twice, so a
	// file carrying two `[mcp_servers.civitai]` blocks is already broken; picking
	// one to replace would silently change which definition wins.
	seen := map[string]bool{}
	for _, b := range blocks {
		if b.Name == "" {
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
		if !mine {
			out = append(out, b.Lines...)
			continue
		}
		// Replace the block's contents; the trailing blank lines it carried are
		// dropped with it and re-added below, so repeated runs are idempotent.
		out = append(out, renderTOMLServer(t, srv, token)...)
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
// 🔴 0600 ON A FILE WE CREATE, and the existing mode is preserved otherwise. The
// entry carries a bearer token whenever one is configured, so a world-readable
// new file would be this command leaking a credential into a path the user never
// chose. Preserving the mode on an existing file is the other half: tightening
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
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
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
	return os.Rename(tmpName, path)
}

// mcpPasteBlock renders the config an `other` agent's user has to place by hand.
//
// It is the SAME entry builder the file writers use, so the block a human pastes
// cannot drift from the block the CLI writes — which is the failure this command
// exists to prevent, moved from a hosted doc into the binary.
func mcpPasteBlock(token string) string {
	// The shape shown is the most widely used one: a `mcpServers` object with
	// `type`/`url`/`headers`. The next-step block names the per-agent keys that
	// differ, because the ONE thing a paste cannot carry is which key name the
	// reader's own agent wants.
	shape := agentTargets[agentClaude]
	servers := map[string]any{}
	for _, srv := range civitaiMCPServers {
		servers[srv.Name] = mcpEntry(shape, srv, token)
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
