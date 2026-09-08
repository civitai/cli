package cmd

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The two instruction files `agent-setup` writes into the project, and the rule
// that keeps it from destroying either.
//
// 🔴 THE TEMPLATE IS AN EMBEDDED FILE, NOT A GO STRING LITERAL. It is CONTENT —
// the sentences an author's agent reads before it writes their app — and content
// in a `const` is content nobody edits: no markdown preview, no table rendering,
// and a diff that reads as a code change. templates/agents-app.md is the file.
//
// 🔴 AND IT PINS NO `@civitai/*` VERSION, ANYWHERE. Pins live in
// `civitai app init`, which the `pins-vs-published` CI job holds against npm. A
// version literal in an embedded template rots in silence — it keeps rendering,
// keeps looking authoritative, and points an author at a package set that no
// longer works together. A command that fetches the pins fails loudly instead.
// TestAgentsTemplatePinsNoVersion is what makes that a rule rather than an
// intention.

//go:embed templates/agents-app.md
var agentsAppTemplate string

// The managed-block markers, byte for byte. They are the CONTRACT between one
// run and the next: everything between them is this command's to rewrite, and
// everything outside them is the author's and is never touched.
//
// 🔴 THE EM DASH IS PART OF THE MARKER. A hyphen here produces a marker that does
// not match the one already in an author's file, so the next run APPENDS a second
// managed block instead of replacing the first — and then every run after that
// appends another. TestAgentsMarkersAreInTheTemplate pins both spellings against
// the embedded file, so the two cannot drift apart.
const (
	agentsBeginMarker = "<!-- BEGIN civitai agent-setup — managed block, edits here are overwritten -->"
	agentsEndMarker   = "<!-- END civitai agent-setup -->"
)

// The two files this command writes into the project directory.
const (
	agentsFilename = "AGENTS.md"
	claudeFilename = "CLAUDE.md"
	// claudeShim is the WHOLE content of a CLAUDE.md this command creates.
	//
	// 🔴 A ONE-LINE IMPORT, NOT A COPY OF THE TEMPLATE. Claude Code does not read
	// AGENTS.md (code.claude.com/docs/en/memory), so the shim is what makes the
	// one instruction file reach it — and keeping it to an import means there is
	// exactly one place the instructions live and nothing to keep in sync.
	claudeShim = "@" + agentsFilename + "\n"
)

// fileAction is what a run did (or would do) to one file. It is reported in
// `--dry-run`, in the human output and in `--json`, from one value, so the three
// cannot describe the same run differently.
type fileAction string

const (
	actionCreate    fileAction = "create"
	actionReplace   fileAction = "replace-managed-block"
	actionAppend    fileAction = "append-managed-block"
	actionUnchanged fileAction = "unchanged"
	// actionKeep is the CLAUDE.md rule: the file is already there, so it is left
	// exactly as the author wrote it. It is a distinct value from `unchanged`
	// because the reasons differ and the user needs to know which one happened.
	actionKeep fileAction = "keep-existing"
)

// agentsManagedBlock is the block as it is written into a file: the embedded
// template with no leading or trailing blank lines, so the three cases below can
// each control their own spacing.
func agentsManagedBlock() string {
	return strings.TrimSpace(agentsAppTemplate)
}

// mergeAgentsMD applies the three-case AGENTS.md rule to `existing` and returns
// the file's new content plus what it did.
//
//	no file                       -> write the template            (create)
//	file without the block        -> APPEND the block              (append)
//	file with the block           -> replace BETWEEN the markers   (replace)
//
// 🔴 CASE 2 APPENDS AND DOES NOT REWRITE. An author's AGENTS.md is theirs — it
// holds their build commands, their conventions, their warnings — and this
// command has exactly one section to contribute. Overwriting it would delete
// work no version of this CLI could give back.
//
// 🔴 CASE 3 IS BYTE-FOR-BYTE OUTSIDE THE MARKERS. Not "re-render the file with
// the new block": the prose above and below the block is untouched, including
// its whitespace, because a normalising rewrite of somebody's document is a
// change they did not ask for and cannot review.
func mergeAgentsMD(path, existing string) (string, fileAction, error) {
	block := agentsManagedBlock()
	if existing == "" {
		return block + "\n", actionCreate, nil
	}

	// 🔴 A SECOND MANAGED BLOCK IS REFUSED, NOT SILENTLY IGNORED. The replace
	// case below spans the FIRST begin to the FIRST end, so a file carrying two
	// blocks keeps the later one forever: it is never refreshed, it keeps
	// instructing the agent with whatever the template said when it was written,
	// and `--check` stayed green because a block was found. That is the worst of
	// the three states — a stale instruction file that reports healthy — and it
	// is reachable by an ordinary copy-paste or a bad merge resolution.
	if state := agentsMDBlockState(existing); state == blockDuplicated {
		return "", "", fmt.Errorf(
			"%s contains %d civitai managed blocks — only the first is ever refreshed, so the rest go stale "+
				"silently while `civitai agent-setup --check` still reports them as present; delete all but one "+
				"(everything between a BEGIN and its END is this command's to rewrite) and re-run "+
				"`civitai agent-setup`",
			path, strings.Count(existing, agentsBeginMarker))
	}

	begin := strings.Index(existing, agentsBeginMarker)
	end := strings.Index(existing, agentsEndMarker)
	switch {
	case begin < 0 && end < 0:
		// Case 2 — append, with exactly one blank line before the block and
		// every existing byte in front of it left alone.
		prefix := strings.TrimRight(existing, "\n")
		return prefix + "\n\n" + block + "\n", actionAppend, nil

	case begin >= 0 && end > begin:
		// Case 3 — replace the span, markers included.
		head := existing[:begin]
		tail := existing[end+len(agentsEndMarker):]
		updated := head + block + tail
		if updated == existing {
			return existing, actionUnchanged, nil
		}
		return updated, actionReplace, nil
	}

	// 🔴 HALF A MANAGED BLOCK IS REFUSED, NOT REPAIRED. One marker without the
	// other (or an END before its BEGIN) means the file was hand-edited across
	// the boundary, and this command cannot tell which of the surrounding bytes
	// were its own and which are the author's. Appending would leave a nested,
	// self-overlapping block that every subsequent run gets more wrong.
	//
	// 🔴 THE OUT-OF-ORDER CASE GETS ITS OWN SENTENCE, BECAUSE THE SHARED ONE WAS
	// ACTIVELY MISLEADING. With END above BEGIN both markers ARE present, so the
	// generic message read "BEGIN present: true, END present: true — restore both
	// marker lines", sending the reader to look for a marker that is sitting
	// right there. Nothing about the file needs restoring; the two lines need
	// swapping.
	if begin >= 0 && end >= 0 {
		return "", "", fmt.Errorf(
			"%s has the civitai managed block's markers in the wrong order — the END marker appears BEFORE the "+
				"BEGIN marker, so there is no span this command can rewrite. Both lines are present: swap them "+
				"back (BEGIN first, END last) or delete both and re-run `civitai agent-setup`; this command will "+
				"not guess where your text ends and its own begins",
			path)
	}
	return "", "", fmt.Errorf(
		"%s contains only one half of the civitai managed block (BEGIN present: %t, END present: %t) — "+
			"restore the missing marker line or delete the partial block, then re-run `civitai agent-setup`; "+
			"this command will not guess where your text ends and its own begins",
		path, begin >= 0, end >= 0)
}

// agentsBlockState is what a file's managed-block markers add up to. The three
// non-healthy values are distinct because each has a DIFFERENT remedy, and the
// message that collapsed two of them sent readers after a marker that was there.
type agentsBlockState int

const (
	// blockAbsent: neither marker. The block gets appended.
	blockAbsent agentsBlockState = iota
	// blockPresent: exactly one well-ordered BEGIN…END pair.
	blockPresent
	// blockPartial: one marker without the other, or END before BEGIN.
	blockPartial
	// blockDuplicated: more than one BEGIN, or more than one END. Only the first
	// pair is ever refreshed, so the others rot in place — and `--check` used to
	// call this healthy.
	blockDuplicated
)

// agentsMDBlockState classifies a file's markers. `--check` and the merge both
// read THIS, so a state one of them refuses cannot be a state the other calls
// green.
func agentsMDBlockState(content string) agentsBlockState {
	begins := strings.Count(content, agentsBeginMarker)
	ends := strings.Count(content, agentsEndMarker)
	switch {
	case begins > 1 || ends > 1:
		return blockDuplicated
	case begins == 0 && ends == 0:
		return blockAbsent
	case begins == 1 && ends == 1 && strings.Index(content, agentsEndMarker) > strings.Index(content, agentsBeginMarker):
		return blockPresent
	default:
		return blockPartial
	}
}

// 🔴 `agentsMDHasManagedBlock` WAS DELETED, NOT LEFT AS A WRAPPER. It answered a
// BOOLEAN — "is there a block?" — and that shape is precisely why a duplicate
// block read as healthy: two blocks are `true`. `--check` now branches on
// `agentsMDBlockState` directly, so it has to name which of the four states it is
// treating as green. Do not reintroduce a boolean helper here; a caller that only
// wants "present or not" is a caller about to miss `blockDuplicated` again.

// planAgentsMD renders what a run would do to <dir>/AGENTS.md without writing.
// The real run calls exactly this and then writes the returned content, so
// `--dry-run` cannot report an action the write path would not take.
//
// 🔴 THE DESTINATION IS CLASSIFIED HERE TOO, NOT ONLY FOR THE MCP CONFIG. Round
// 2 put checkWriteTargetResolvable into `planMCPConfig` and stopped, so the same
// lie moved one file over: with `AGENTS.md` a broken symlink, `--dry-run --json`
// said `create` / `ok: true` / exit 0 for a destination `writeProjectFile`
// refuses by name. Measured at 897c1cc. The check is a pure read (Lstat plus a
// link resolution), so every plan can afford it and none of them may skip it —
// TestDryRunAndTheRealRunAgreeOnEveryAction carries a fixture per file.
func planAgentsMD(dir string) (path, content string, action fileAction, err error) {
	path = filepath.Join(dir, agentsFilename)
	if terr := checkWriteTargetResolvable(path); terr != nil {
		return path, "", "", terr
	}
	raw, _, rerr := readIfExists(path)
	if rerr != nil {
		return path, "", "", rerr
	}
	content, action, err = mergeAgentsMD(path, string(raw))
	return path, content, action, err
}

// planClaudeMD renders what a run would do to <dir>/CLAUDE.md.
//
// 🔴 ONLY IF ABSENT. An existing CLAUDE.md is never read, never appended to and
// never rewritten — it is the file an author is most likely to have filled with
// project instructions, and the shim has nothing to contribute to one that
// already exists. The action returned says `keep-existing` so the output tells
// them that rather than staying silent about a file it decided not to touch.
//
// 🔴 AND ITS DESTINATION IS CLASSIFIED FOR THE SAME REASON planAgentsMD's is: a
// CLAUDE.md symlinked into a dotfiles repo whose target has gone is refused by
// the writer, so the planner has to reach the same verdict or `--dry-run` lies
// about this file the way it did about the other two.
func planClaudeMD(dir string) (path, content string, action fileAction, err error) {
	path = filepath.Join(dir, claudeFilename)
	if terr := checkWriteTargetResolvable(path); terr != nil {
		return path, "", "", terr
	}
	if _, statErr := os.Stat(path); statErr == nil {
		return path, "", actionKeep, nil
	} else if !os.IsNotExist(statErr) {
		return path, "", "", statErr
	}
	return path, claudeShim, actionCreate, nil
}

// writeProjectFile writes one instruction file. 0644: AGENTS.md and CLAUDE.md
// carry no credential and are meant to be committed and read by everyone on the
// team, unlike the MCP configs (see writeMCPConfig).
func writeProjectFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return writeFileAtomic(path, []byte(content), mode)
}
