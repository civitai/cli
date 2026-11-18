// The DELEGATION of one brief's render half to another brief's assertion.
//
// NOT AN ASSERTION. This module is imported by `ship.assert.mjs` and
// `t1.assert.mjs`, neither of which grades anything itself: both briefs are the
// `genpost` brief plus trailing clauses that no browser can see (a store
// submission, and store-listing media), so their RENDERABLE half is `genpost`'s
// byte for byte and is graded by `genpost.assert.mjs`.
//
// 🔴 IT IS ONE FILE BECAUSE THE TRANSLATION IT PERFORMS IS A PREDICATE, AND A
// PREDICATE OPEN-CODED AT N SITES IS WRONG AT N−1 OF THEM. What looks like a
// three-line `spawnSync` is really the 0/1/2 exit contract `oracle.sh` reads,
// plus the one case that is easy to get wrong in exactly one direction: a child
// KILLED BY A SIGNAL reports `status: null`, and `process.exit(null)` exits
// **0** — i.e. a delegate the OOM killer took would report the block as PASSING.
// `ship.assert.mjs` got that right; a hand-copied second spawner is one edit
// away from not. The same argument `_esc.sh` carries for the graders' escaping.
//
// 🔴 WHY BY SPAWN RATHER THAN BY IMPORT, unchanged from the argument
// `ship.assert.mjs` shipped with: `genpost.assert.mjs` is exercised by a real
// browser under `TestOracleGradesTheGenpostBrief` and five siblings.
// Refactoring it to export a grader would put that working, measured file in the
// blast radius of a brief that has never been run. stdio is INHERITED, so the one
// JSON line, the `observed` sequence, the reason and the exit code all pass
// through untouched — including exit 2, the "the harness could not run" code that
// must never become a verdict about the block.
//
// 🔴 `env: process.env` IS LOAD-BEARING, NOT A DEFAULT SPELLED OUT. The arm
// switches (`CIVITAI_ASSERT_UNCONSENTED`, `CIVITAI_ASSERT_POST_PATH`) and
// `CIVITAI_CHROME` reach the delegate only through it; dropping it would leave
// every delegating brief permanently on the default arm while `oracle.sh` printed
// the arm banner it was asked for. The app that motivated the unconsented arm,
// `ab-ship-mimo-02`, is a delegating cell.
import { spawnSync } from 'node:child_process';

// delegate runs `delegatePath` with this process's argv tail and EXITS. It never
// returns.
//
//   assertionName  the name that appears in the JSON line if the delegate could
//                  not be run at all — the only output this module ever authors.
export function delegate(delegatePath, assertionName) {
  const r = spawnSync(process.execPath, [delegatePath, ...process.argv.slice(2)], {
    stdio: 'inherit',
    env: process.env,
  });
  // A child killed by a signal reports `status: null`. That is "the assertion
  // could not run", not "the block failed" — exit 2, the same third state the
  // rest of this harness uses. 🔴 Falling through to `process.exit(r.status)`
  // here would exit 0 and read as a PASS.
  if (r.error || r.status === null) {
    console.log(JSON.stringify({
      assertion: assertionName,
      pass: false,
      reason: `harness error: could not run ${delegatePath}: ${r.error ? r.error.message : `killed by ${r.signal}`}`,
    }));
    process.exit(2);
  }
  process.exit(r.status);
}
