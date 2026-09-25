#!/usr/bin/env node
// The RENDER half of the `ship` brief. See ship.md.
//
//   node ship.assert.mjs <dir|url> [scope,scope,…]
//
// 🔴 A SHIP CELL HAS TWO HALVES AND THIS FILE IS ONLY ONE OF THEM. The block
// must still RENDER AND BEHAVE (this file, driven by `oracle.sh`) *and* it must
// have been SUBMITTED (`../ship.verdict.sh`, which reads the account). Neither
// implies the other: an app can be submitted and broken, and it can work
// perfectly and never be submitted — which is precisely what four credentialed
// `genpost` trials did.
//
// 🔴 AND THIS HALF IS THE `genpost` ASSERTION, DELIBERATELY, RATHER THAN A COPY
// OF IT. The `ship` brief keeps `genpost`'s testable specifics verbatim — the
// same two test ids, the same two button labels, the same two status words, the
// same Post gate — and appends a submit instruction. A forked assertion would be
// one predicate open-coded at two sites, which is how the same bug gets fixed
// once and regenerated at the other site; `genpost.md` records the scaffold
// controls and the generate-only control for exactly these constants, and a
// second copy would silently stop being the thing those controls were run
// against. `ship.md` pins the delegation so a fork cannot happen quietly.
//
// 🔴 IT CANNOT GRADE THE SUBMIT, AND NO BROWSER ASSERTION EVER COULD. A
// submission is server state. It leaves nothing in the DOM, the oracle's host
// emulation is the SDK's `InlineTransport` stub whose `sendRequest` rejects
// unconditionally, and `token.raw` is empty — so nothing can COMPLETE here, by
// construction. Grading the submit needs the account, which is what
// `ship.verdict.sh` reads.
//
// Delegation is by SPAWN rather than by import, and that is a deliberate
// trade: `genpost.assert.mjs` is exercised by a real browser under
// `TestOracleGradesTheGenpostBrief` and friends, and refactoring it to export a
// grader would put that working, measured file in the blast radius of a brief
// that has never been run. stdio is inherited, so the one JSON line, the exit
// code and the 0/1/2 contract `oracle.sh` reads all pass through untouched —
// including exit 2, the "the harness could not run" code that must never become
// a verdict about the block.
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const DELEGATE = path.join(HERE, 'genpost.assert.mjs');

const r = spawnSync(process.execPath, [DELEGATE, ...process.argv.slice(2)], {
  stdio: 'inherit',
  env: process.env,
});
// A child killed by a signal reports `status: null`. That is "the assertion
// could not run", not "the block failed" — exit 2, the same third state the
// rest of this harness uses.
if (r.error || r.status === null) {
  console.log(JSON.stringify({
    assertion: 'ship',
    pass: false,
    reason: `harness error: could not run ${DELEGATE}: ${r.error ? r.error.message : `killed by ${r.signal}`}`,
  }));
  process.exit(2);
}
process.exit(r.status);
