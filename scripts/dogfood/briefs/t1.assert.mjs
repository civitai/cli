#!/usr/bin/env node
// The RENDER half of the `t1` brief. See t1.md.
//
//   node t1.assert.mjs <dir|url> [scope,scope,…]
//
// 🔴 A T1 CELL HAS THREE HALVES AND THIS FILE IS ONLY ONE OF THEM. The block must
// RENDER AND BEHAVE (this file, driven by `oracle.sh`), it must have been
// SUBMITTED, *and* its store listing must carry an ICON AND A COVER — the publish
// floor. The last two are both server state and are graded together by
// `../ship.verdict.sh`, which emits `SHIP=`, `FLOOR=` and the combined `T1=`.
// None of the three implies another: an app can be submitted and broken, it can
// work perfectly and never be submitted (which is exactly what four credentialed
// `genpost` trials did), and it can be submitted and working with an empty
// listing.
//
// 🔴 IT CANNOT GRADE EITHER SERVER HALF, AND NO BROWSER ASSERTION EVER COULD. The
// oracle's host emulation is the SDK's `InlineTransport` stub whose `sendRequest`
// rejects everything outside the running arm's answered set, and `token.raw` is
// empty — so no app-store submission and no listing attach can complete here, by
// construction, on any arm. A submission and a listing asset leave nothing in the
// DOM.
//
// ⚠ "SUBMIT" IS TWO DIFFERENT THINGS IN THIS RIG. The opt-in post arm DOES answer
// `SUBMIT_WORKFLOW`, so a canned GENERATION's submit completes in the page. What no
// arm answers is the app-store submission, and nothing any arm adds reaches the
// store listing. See ship.md and briefs/genpost.md.
//
// 🔴 AND THIS HALF IS THE `genpost` ASSERTION, DELIBERATELY, RATHER THAN A COPY OF
// IT — the same argument ship.assert.mjs carries, one rung further up. `t1` is the
// `ship` brief verbatim plus a listing-media clause, and `ship` is the `genpost`
// brief verbatim plus a submit clause, so all three share one renderable half: the
// same two test ids, the same two button labels, the same two status words, the
// same Post gate. A forked assertion would be one predicate open-coded at three
// sites, and `genpost.md`'s scaffold control (all three templates) and
// `neg-nopost` generate-only control were measured against THOSE constants in THAT
// file — a fork silently stops being the thing they were run against.
// `TestT1BriefAndAssertionAgree` pins the whole normalised brief string so a fork
// cannot happen quietly.
//
// The spawn mechanism is `_delegate.mjs`, shared with `ship.assert.mjs`. Read it
// for why by spawn rather than by import, and for the `status === null` case that
// would otherwise report a signal-killed delegate as a PASS.
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { delegate } from './_delegate.mjs';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const DELEGATE = path.join(HERE, 'genpost.assert.mjs');

delegate(DELEGATE, 't1');
