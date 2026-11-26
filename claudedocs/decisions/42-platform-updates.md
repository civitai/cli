# `app pull` and `app submit` merge staff platform updates, and submit refuses while one is unmerged

Civitai staff (moderators) can push an update to someone else's app: one
commit on its own branch (`platform-update/<id>`) of the app's SOURCE
repository (`civitai-apps-src/<slug>`). The server half, and the contract this
client implements, is civitai/civitai#10594 ("Owner-facing API", "Expected
client flow"): `appPlatformUpdates.listPending`, `verifyCommit` and `resolve`.

Implementation: `internal/appapi/platform_updates.go` (the wire contract, in
one file), `internal/cmd/app_platform_updates.go` (the flow), and the two call
sites in `internal/cmd/app_pull.go` and `internal/cmd/app_submit.go`. Guards:
`internal/cmd/app_platform_updates_test.go`, which runs real git against a real
source repository served by `git http-backend` (the `gitServer` of
`internal/cmd/git_credential_git_test.go`, so every fetch authenticates through
the real `civitai git-credential` helper), plus one row each in
`safeTermCoveredBy` and `bareIdentArgs` for `puText`.

Seven things here look wrong, or look like something to tidy. Each is
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
FAILS — network, auth, a deleted branch — reports nothing: `conflicted` would
drop the update off the pending list for good (§7), so the next run retries. Pinned by `TestPlatformUpdateLeavesOutCommitsOnTopOfTheBranch` (red
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

## 4. A dirty tree is refused, not merged around

A merge that does not conflict with uncommitted changes would still tangle the
owner's unfinished work and the staff change in one working tree, and the abort
path could not separate them. Untracked files are not counted: git itself
refuses a merge that would overwrite one, and that refusal is reported without
calling `resolve` (the update was never tried against the tree). Pinned by
`TestPlatformUpdateRefusesADirtyTree`.

## 5. Submit refuses while an update is unmerged, but FAILS OPEN on a failed read

The bundle is built from the directory on disk, so a submit from a checkout
that lacks a pending update, or from a directory that is not a checkout at all,
would publish a version that silently drops it. Every way the update is left
unmerged therefore refuses, naming `--allow-unmerged-platform-update`.

A `listPending` call that FAILS (anything but a 404, which is "no updates") is a
warning and the submit goes ahead, like the listing gate and the SDK-floor read
beside it: a failed read says nothing about whether an update is pending, and
refusing every submit while the procedure is broken would block the whole
submit path on one read. That is a real residual: during such a failure a
stale checkout can submit without the update. The warning says so.

After a merge, submit re-reads everything the merge can have changed: the
validation verdict, the manifest (and the version guard if the version moved),
the SDK floor and the provenance, which then names the merge commit.

## 6. The override is recorded nowhere server-side

The submit body has no field for it, and sending an unknown key to a schema
that may be strict would risk refusing every submit that uses the flag. So it
is CLI-only: it merges nothing, refuses nothing, and lists the updates it left
out on stderr.

## 7. A `conflicted` update is invisible to the next run

`listPending` returns PENDING updates only, so once a run reports `conflicted`
the next `app pull` or `app submit` no longer sees it, and submit cannot refuse
over it. The contract says the owner may report `merged` later; there is no
owner-side list of conflicted updates to drive that. The conflict output names
the sha and the hand-merge command, and that is the only place it is said.
Closing this needs a server change (list conflicted updates to the owner too).
