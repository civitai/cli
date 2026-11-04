package cmd

// app_submit_storage.go prints the two App Storage schema-migration fields the
// submit routes return (civitai/civitai src/pages/api/v1/blocks/submit-version.ts):
//
//   - `storageBreakingChanges`, on a REFUSED submit (400, a
//     StorageMigrationRefusal): each breaking schema change since the previous
//     approved version that no declared migration op covers.
//   - `storageMigrationWarnings`, on an ACCEPTED submit (200): notes that never
//     refuse — chiefly a declared op that covers nothing and so will not run.
//
// Until this file the CLI decoded neither, so an author saw a refusal's one long
// message line and never saw a warning at all.
//
// 🔴 THE VERDICT IS THE SERVER'S AND HAS NO LOCAL MIRROR. Whether a change is
// breaking, and whether an op covers it, is decided against the PREVIOUS APPROVED
// manifest, which only civitai.com holds; `civitai app validate` checks the
// `migrations` key's SHAPE only (internal/validate's
// storage_collection_migrations_test.go says why). These printers render what the
// server sent and decide nothing.
//
// Every string here is SERVER text, so each goes through safeTermSingle before it
// reaches the terminal.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/ui"
)

// storageValueMaxRunes mirrors the server's `shown` (storage-schema-migrations.ts),
// which cuts a value's JSON at 80 and appends "…", so a line here reads the same
// as the one inside the refusal message.
const storageValueMaxRunes = 80

// shownStorageValue renders one side (old or new) of a breaking change the way
// the server's describeBreakingChange does: "(none)" for an absent keyword, else
// the value's compact JSON, cut at storageValueMaxRunes.
func shownStorageValue(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return "(none)"
	}
	s := string(trimmed)
	var compact bytes.Buffer
	if json.Compact(&compact, trimmed) == nil {
		s = compact.String()
	}
	if r := []rune(s); len(r) > storageValueMaxRunes {
		s = string(r[:storageValueMaxRunes]) + "…"
	}
	return s
}

// describeStorageBreakingChange is one breaking change as one line, in the
// server's describeBreakingChange format:
// `<collection> <"pointer" | (the document root)> <keyword>: <old> → <new>`.
func describeStorageBreakingChange(c appapi.StorageBreakingChange) string {
	where := "(the document root)"
	if c.Pointer != "" {
		q, _ := json.Marshal(c.Pointer)
		where = string(q)
	}
	return safeTermSingle(fmt.Sprintf("%s %s %s: %s → %s",
		c.Collection, where, c.Keyword, shownStorageValue(c.Old), shownStorageValue(c.New)))
}

// storageBreakingChangesHeader opens the refusal's list; %d is its length.
const storageBreakingChangesHeader = "The server refused this submit over %d storage schema change(s) that no migration covers:"

// storageBreakingChangesFooter says what to do about each line. It names the
// manifest key and the four ops; the error that follows carries the server's
// full remedy text.
const storageBreakingChangesFooter = "Declare an op for each under storage.collections.<name>.migrations in " +
	"block.manifest.json — \"default\", \"rename\", \"remove\" or \"acknowledge\" — then submit again. " +
	"The error below has the server's guidance."

// printStorageBreakingChanges writes a refused submit's `storageBreakingChanges`,
// one per line. An empty list (no key, or a refusal over migrationErrors alone)
// prints nothing.
func printStorageBreakingChanges(w io.Writer, changes []appapi.StorageBreakingChange) {
	if len(changes) == 0 {
		return
	}
	fmt.Fprintln(w, "\n"+ui.For(w).Warn(fmt.Sprintf(storageBreakingChangesHeader, len(changes))))
	for _, c := range changes {
		printFinding(w, describeStorageBreakingChange(c))
	}
	for _, line := range wrapRunes(storageBreakingChangesFooter, 78) {
		fmt.Fprintln(w, line)
	}
}

// storageMigrationWarningsHeader opens an accepted submit's notes; %d is their
// count.
const storageMigrationWarningsHeader = "%d storage migration note(s) from the server (they do not block this submit):"

// printStorageMigrationWarnings writes an accepted submit's
// `storageMigrationWarnings`, one wrapped item each, in the server's order. An
// empty or absent list prints nothing, so a submit with no storage notes reads
// exactly as it did before the field existed.
func printStorageMigrationWarnings(w io.Writer, warnings []string) {
	if len(warnings) == 0 {
		return
	}
	fmt.Fprintln(w, ui.For(w).Warn(fmt.Sprintf(storageMigrationWarningsHeader, len(warnings))))
	for _, warn := range warnings {
		printFinding(w, safeTermSingle(warn))
	}
}
