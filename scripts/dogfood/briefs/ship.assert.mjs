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
// 🔴 IT CANNOT GRADE THE APP-STORE SUBMISSION, AND NO BROWSER ASSERTION EVER COULD.
// A submission is server state. It leaves nothing in the DOM, the oracle's host
// emulation is the SDK's `InlineTransport` stub whose `sendRequest` rejects
// everything outside the running arm's answered set, and `token.raw` is empty — so no
// SUBMISSION can complete here, by construction, on any arm.
//
// ⚠ "SUBMIT" IS TWO DIFFERENT THINGS IN THIS RIG AND THIS PARAGRAPH IS ABOUT ONLY
// ONE OF THEM. The post arm DOES answer `SUBMIT_WORKFLOW` — it is one of the four
// workflow/post types it widens the answered set by (`INVARIANT_EXCEPTIONS` in
// `_cdp.mjs`) — so a generation's submit completes there, in the page. What no arm
// answers is the app-store submission: nothing the arm adds reaches the store.
// Grading that needs the account, which is what `ship.verdict.sh` reads.
//
// Delegation is by SPAWN rather than by import, and the MECHANISM now lives in
// `_delegate.mjs` because `t1.assert.mjs` needs the same one: `t1` is this brief
// plus a store-listing-media clause, so its renderable half is `genpost`'s too. A
// second hand-written spawner would be the 0/1/2 contract open-coded twice, and it
// has one case that fails in the reassuring direction — a delegate killed by a
// signal reports `status: null`, and `process.exit(null)` exits **0**, i.e. a PASS.
// See `_delegate.mjs` for the full argument, which is the one this file shipped
// with.
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { delegate } from './_delegate.mjs';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const DELEGATE = path.join(HERE, 'genpost.assert.mjs');

delegate(DELEGATE, 'ship');
