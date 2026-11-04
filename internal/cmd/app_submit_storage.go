package cmd

// app_submit_storage.go prints the App Storage schema-migration notes an
// ACCEPTED submit returns (`storageMigrationWarnings`, civitai/civitai
// src/pages/api/v1/blocks/submit-version.ts): notes that never refuse — chiefly
// a declared migration op that covers nothing and so will not run. Until this
// file the CLI did not decode the field, so an author never saw them.
//
// A REFUSED submit needs nothing here: the server's 400 `message` already
// itemises each uncovered breaking change (describeBreakingChange, up to 20),
// and that message is what the `Error:` line prints.
//
// 🔴 THE VERDICT IS THE SERVER'S AND HAS NO LOCAL MIRROR. Whether an op covers
// a change is decided against the PREVIOUS APPROVED manifest, which only
// civitai.com holds; `civitai app validate` checks the `migrations` key's SHAPE
// only (internal/validate's storage_collection_migrations_test.go says why).
//
// Every string here is SERVER text, so each goes through safeTermSingle before
// it reaches the terminal.

import (
	"fmt"
	"io"

	"github.com/civitai/cli/internal/ui"
)

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
		printFinding(w, warn) // NEGATIVE CONTROL: no safeTermSingle
	}
}
