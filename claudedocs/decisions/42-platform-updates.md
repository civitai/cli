# `app pull` and `app submit` merge staff platform updates, and submit refuses while one is unmerged

Civitai staff (moderators) can push an update to someone else's app: one
commit on its own branch (`platform-update/<id>`) of the app's SOURCE
repository (`civitai-apps-src/<slug>`). The server half, and the contract this
client implements, is civitai/civitai#10594 ("Owner-facing API", "Expected
client flow"): `appPlatformUpdates.listPending`, `verifyCommit` and `resolve`.

Implementation: `internal/appapi/platform_updates.go` (the wire contract, in
one file), `internal/cmd/app_platform_updates.go` (the flow), and the two call
sites in `internal/cmd/app_pull.go` and `internal/cmd/app_submit.go`. Guards:
`internal/cmd/app_platform_updates_test.go` and
`internal/cmd/app_platform_updates_edges_test.go`, which run real git against a real
source repository served by `git http-backend` (the `gitServer` of
`internal/cmd/git_credential_git_test.go`, so every fetch authenticates through
the real `civitai git-credential` helper), plus one row each in
`safeTermCoveredBy` and `bareIdentArgs` for `puText`.

Twelve things here look wrong, or look like something to tidy. Each is
deliberate.

## 1. It merges a sha, never the branch

The branch is ordinary git and can move after the push; only the recorded
`commitSha` is what the server's record attests. So the flow fetches the branch
by URL, checks the commit arrived (`git cat-file -e <sha>^{commit}`), fetches
it by sha if not, and runs `git merge --no-ff <commitSha>`. Never `FETCH_HEAD`,
`<remote>/<branch>` or the branch name. A branch tip that is not `commitSha`
has commits that are not part of the update: they are left out, and the owner
is told. A commit the branch no longer holds (the branch was fetched, and the
sha is not reachable) is reported `conflicted` and not merged. A fetch that
FAILS — network, auth, a deleted branch — reports nothing: none of those is a
conflict, so the next run retries. Pinned by `TestPlatformUpdateLeavesOutCommitsOnTopOfTheBranch` (red
when the merge target is `FETCH_HEAD`) and
`TestPlatformUpdateMissingCommitIsNotMerged`, and the failed-fetch arm by
`TestPlatformUpdateFailedFetchIsNotReportedConflicted`.

## 2. The badge comes from `verifyCommit` alone

`listPending` returns `by.verifiedStaff` too, and it is not read for the badge.
The contract names `verifyCommit` as the ONLY source of "verified Civitai
staff", and that call also answers false for an update a moderator withdrew.
Any failure of the call is "not verified": the badge is drawn only on a
positive answer. Pinned by `TestPlatformUpdateUnverifiedCommitHasNoBadge` (red
when the badge is drawn from the listing's field).

## 3. The source repository is fetched by URL, and no remote is added

`git remote add` writes the checkout's config, and a build-repo checkout
(`app pull` keeps one on the build repository) has no remote for the source
repository. The fetch is `git fetch -- <url> <ref>`, with the credential helper
`app pull` installs passed for that one command as `-c` options, exactly as the
clone passes it. A checkout that never had the helper still authenticates and
nothing is written to `.git/config`.

## 4. A dirty app is refused, not merged around — and only the app counts

A merge that does not conflict with uncommitted changes would still tangle the
owner's unfinished work and the staff change in one working tree, and the abort
path could not separate them. The check is scoped to the app directory exactly
as `app submit`'s dirty guard is (`uncommittedAppPaths`: the same `-- .`
pathspec, prefix and `bundleDirtyPaths`), so a sibling package's unfinished work
in a monorepo does not block. A dirty file outside the app that the merge would
touch, or a staged change anywhere (git wants the index to match HEAD), is
refused by git before it starts; that refusal is reported as "it would overwrite
uncommitted changes in <paths>", without `resolve`. Untracked files are not
counted: git refuses a merge that would overwrite one. Pinned by
`TestPlatformUpdateRefusesADirtyTree` and
`TestPlatformUpdateDirtyCheckIsScopedToTheAppDirectory` (its in-app row is an
invariant guard). The pathspec and the prefix filter each scope on their own: a
mutant removing ONE survives, removing both is killed.

## 5. Submit refuses while an update is unmerged, but FAILS OPEN on a failed read

The bundle is built from the directory on disk, so a submit from a checkout
that lacks a pending update, or from a directory that is not a checkout at all,
would publish a version that silently drops it. Every way the update is left
unmerged therefore refuses, naming `--allow-unmerged-platform-update`.

A `listPending` call that FAILS (anything but a 404 — §8 — or a 200) is a
warning and the submit goes ahead, like the listing gate and the SDK-floor read
beside it: a failed read says nothing about whether an update is pending, and
refusing every submit while the procedure is broken would block the whole
submit path on one read. That is a real residual: during such a failure a
stale checkout can submit without the update. The warning says so.

After a merge, submit re-reads everything the merge can have changed: the
validation verdict, the manifest (and the version guard if the version moved),
the SDK floor and the provenance, which then names the merge commit.

## 6. The server checks the bundle; the override is recorded for the reviewer

Operator decision (2026-11-26, "Server checks the bundle"): on every submit the
SERVER checks the uploaded bundle contains each staff update not yet shipped in
an approved version, and refuses unless the client sent the override. So
`--allow-unmerged-platform-update` sets `allowUnmergedPlatformUpdate: true` in
the submit body (`Provenance.AllowUnmergedPlatformUpdate`, counted by
`SubmitBodySize`; omitted without the flag), and the server records it for the
moderator. Client-side it still merges nothing and lists what it left out. The
client check is the first line; the server is the backstop for an older CLI, a
hand-built bundle or a checkout the flow could not merge into. Its refusal, code
`platform_update_missing` (optional `details.updates: [{id, reason, by,
paths}]`, read leniently), is `appapi.PlatformUpdateMissingError`; `app submit`
lists the updates it names and names the flag, and any details shape it cannot
read still prints the server's message. Pinned by
`TestSubmitExplainsTheServersPlatformUpdateMissingRefusal` and the appapi
`platform_update_missing_test.go`.

## 7. A `conflicted` update is retried on every run, and reported once

`listPending` returns OPEN updates — `pending` AND `conflicted` — so an update a
run reported `conflicted` is still listed on the next `app pull` or `app
submit` (it used to vanish after that one report, and a stale checkout could
then submit without it). Each run retries it: already in HEAD's history (the
owner merged it by hand) → `resolve merged` with HEAD, merging nothing; merges
cleanly now → `resolve merged` with the merge commit; still conflicts → abort,
re-print the hand-merge instructions naming the sha, exit non-zero, and submit
refuses. `conflicted` is NOT sent again for an update already listed
`conflicted` (the server would no-op it); `platformUpdateRun.resolve` is the one
place that decides it. Pinned by `TestPlatformUpdateConflictedIsRetriedAndReprinted`
and `TestSubmitRefusesOverAConflictedUpdate` (both red on the pre-change client:
it re-reported `conflicted`). `TestPlatformUpdateConflictedMergedByHandIsReportedMerged`
and `TestPlatformUpdateConflictedThatNowMergesCleanlyIsMerged` are invariant
guards: the client already handled both once the server lists the update.

## 8. Two 404s, told apart by tRPC's own message

Both arrive as HTTP 404 with a tRPC error envelope and code `NOT_FOUND`: an
older server's router answers a procedure it lacks with tRPC's own `No
procedure found on path "…"` (v10: `No "query"-procedure on path "…"`), and the
server answers "App not found" for an app it does not know or the caller may
not edit. Measured against `@trpc/server` 11.17.0: the two bodies differ only in
`message`. So `platformUpdates404` recognises "no such procedure" POSITIVELY —
that message, or a 404 with no envelope at all — and that alone is silent;
every other 404 is `appapi.ErrPlatformUpdatesAppNotFound`. If tRPC rewords its
message, an older server gets a warning line, never a silent "App not found".
`app pull` has just resolved the app as the caller's, so it prints ONE warning
line; `app submit` stays silent (a brand-new slug's first submit lands there,
and the submit itself tells an owner the app is not theirs). `verifyCommit`
treats any 404 as "not verified". Pinned by `TestPlatformUpdate404s`.

## 9. Stopped with no conflicted file is not a conflict

A merge can stop with `MERGE_HEAD` set and nothing conflicted: the tree merged,
then git failed to write the commit (`commit.gpgsign` with a broken gpg, a
failing `pre-merge-commit` or `commit-msg` hook). Only a non-empty `git diff
--name-only --diff-filter=U` is a conflict. Otherwise the merge is aborted and
the error is "git could not complete the merge: <git's line>", with no
`resolve` and no hand-merge instructions. Pinned by
`TestPlatformUpdateMergeGitCannotCommitIsNotAConflict`.

## 10. No common ancestor, no merge (build checkouts)

The update lives in the source repository's history. A checkout of the build
repository, or of any other unrelated history, shares none of it (`git
merge-base HEAD <sha>` exits 1), so nothing is merged or reported, and the
owner is told to merge it from a clone of the source repository (`civitai app
pull <new dir> --app <slug>`) or to apply it here with the printed `git fetch
<url> <sha> && git cherry-pick <sha>`. On submit this is unmerged, so it
refuses. A newly seeded source repository is a fork of the build repository,
so a build checkout of one shares history and merges normally. Pinned by
`TestPlatformUpdateUnrelatedHistoryIsNotMerged` (it also runs the printed
cherry-pick), `TestPlatformUpdateBuildCheckoutWithSharedHistoryMerges` and
`TestSubmitRefusesFromACheckoutWithUnrelatedHistory`.

## 11. A detached HEAD merges but is never reported `merged`

A merge commit on a detached HEAD is on no branch. The flow still merges, so a
submit's bundle carries the update, prints that the commit exists only in this
checkout and must be pushed or merged into a branch, and calls no `resolve`;
the next run from a branch that contains it reports it. Pinned by
`TestPlatformUpdateDetachedHeadMergesButIsNotReported`.

## 12. Fetches are bounded, and printed commands are pasteable

Every `platformGit` fetch runs under `gitNetworkTimeout`, in its own session so
the timeout also kills git's remote helper; a timeout is an ordinary failed
fetch. Pinned by `TestPlatformGitFetchIsBoundedByTheNetworkTimeout`. 🟡 That
constant is 5s, sized for `ls-remote`: a large first fetch of the source history
over a slow link can exceed it and fail every run. The printed hand-merge and
cherry-pick commands single-quote the directory and the URL (`shellQuote`) and
carry the credential helper as `-c` options, so they work in a checkout
`civitai app pull` never configured. Pinned literally by
`TestPlatformUpdatePrintedCommandsAreShellQuoted` and behaviourally by
`TestPlatformUpdateHandMergeCommandRunsInACheckoutWithoutTheHelper`.
