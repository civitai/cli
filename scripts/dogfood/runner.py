#!/usr/bin/env python3
"""Blind dogfood harness for developer.civitai.com/agent-setup/prompt.md.

One trial = one model driving one throwaway container. The container is the only
thing the model can touch: its single tool shells into it. The repo, the handoff
docs and the operator's home directory are not in that container's filesystem, so
blindness is a mount namespace, not an instruction.

Usage:
  runner.py --model <openrouter-id> --image <docker-image> --trial <id>
            [--brief "<one line>"] [--agent-env CLAUDECODE=1]
            [--credential-file ~/.config/civitai/config.yaml]
            [--app-prefix dogfood4-] [--max-generations 3] [--max-submissions 1]
            [--max-steps 40] [--max-tokens 32000] [--no-carry-reasoning]
            [--out <dir>]
  runner.py --print-task [--brief "<one line>"]   # offline; spends nothing
"""
import argparse
import hashlib
import json
import os
import pathlib
import re
import shlex
import subprocess
import sys
import time
import urllib.error
import urllib.request

API = "https://openrouter.ai/api/v1/chat/completions"
# Operator-supplied, and the ONLY source. An earlier version fell back to
# reading another tool's credential file when this was unset — in a PUBLIC repo
# that means a contributor who clones and runs the driver without exporting a
# key bills a credential they never pointed at this, which is a surprise in the
# spend direction. Refuse instead.

# What the model is. Deliberately thin: a coding agent with a shell, nothing
# about Civitai. Anything more would be hand-holding the artifact under test.
SYSTEM = """You are a coding agent working on the user's machine. You have one
tool: `bash`, which runs a command in the user's shell, in their project
directory. Use it to inspect the machine and to carry out what the user asks.

Work autonomously: do not ask the user to run commands for you, and do not ask
permission between steps. When you have finished — or when you are blocked and
cannot proceed — stop calling tools and reply with a plain-text report of what
you did, what you observed, and anything the user still has to do themselves."""

# What the user pastes. This is the entrypoint under test and the only Civitai
# information in the trial.
USER = """https://developer.civitai.com/agent-setup/prompt.md"""


def task(brief: str) -> str:
    """The user message for one trial: the hosted URL, plus an optional brief.

    🔴 WITH NO BRIEF THIS RETURNS `USER` BYTE-IDENTICALLY — not "equivalently".
    A setup trial and an app-build trial differ by exactly one appended
    paragraph, so every setup grid already measured stays comparable instead of
    being silently re-based by a harness change. `--print-task` exists so that
    claim can be DIFFED rather than asserted.

    The brief is appended RAW, with no framing sentence of our own. Framing
    would be Civitai knowledge the harness injected — and the trial's whole
    premise is that the only Civitai information reaching the model is the URL
    and what the operator typed. Blindness is a mount namespace (see the module
    docstring); this keeps the message channel honest about the same thing.

    🔴 A CREDENTIAL DOES NOT TOUCH THIS FUNCTION. The credential reaches the
    container through the filesystem (see install_credential), never through the
    task text — so a credentialed trial's task is byte-identical to an
    uncredentialed one's and `--print-task` stays a complete preview.
    """
    brief = (brief or "").strip()
    if not brief:
        return USER
    return USER + "\n\n" + brief


TOOLS = [{
    "type": "function",
    "function": {
        "name": "bash",
        "description": "Run a shell command in the user's project directory and "
                       "return its stdout, stderr and exit code.",
        "parameters": {
            "type": "object",
            "properties": {
                "command": {"type": "string", "description": "The command to run."}
            },
            "required": ["command"],
        },
    },
}]

MAX_OUT = 12000  # bytes of combined output handed back per command

# ─────────────────────────────────────────────────────────────────────────────
# The terminal state.
#
# 🔴 THE DEFECT THIS REPLACES. The loop used to read `if not calls: stop =
# "finished"` — it branched on the ABSENCE OF TOOL CALLS ALONE, and
# `finish_reason` appeared nowhere in this file. A cell that exhausted its
# output budget inside a reasoning channel and returned nothing was therefore
# recorded byte-identically to a cell that completed and wrote an empty report.
# MEASURED, trial `ab-genpost-glm-01`: the last assistant message had
# `content: null`, no tool calls, and `completion_tokens: 8000` — EXACTLY the
# `max_tokens` this file sends — of which 7,992 were reasoning. It was recorded
# `stop: "finished"`. That is a harness limit reported as a task outcome, which
# is the one confound this arc exists not to introduce.
#
# The fix is to read the field the provider already sends and to refuse to
# collapse the cases. The vocabulary below is deliberately NOT a boolean: a
# reader greps the `stop` value and must never have to reopen a transcript to
# find out whether the model chose to stop.
# ─────────────────────────────────────────────────────────────────────────────

# 🔴 THE SAME RULE, ONE AXIS OVER: MONEY. `usage.cost` gets exactly the
# treatment `finish_reason` gets above, and for the identical reason. The loop
# used to read `usage_total["cost"] += u.get("cost", 0.0) or 0.0` — the ONLY
# cost-accumulation site in this file, with no price table and no
# `/api/v1/models` fallback behind it — so a model or a route whose usage
# payload omits `cost`, or returns it null, accumulated $0 FOREVER. `--max-cost`
# then never tripped, `--max-steps` was the only remaining bound, and the
# docstring beside that flag says in its own words that a step cap is NOT a
# spend cap. That is an unbounded-spend path, and the `end` record's
# `cost: 0.0` is the money-shaped version of `stop: "finished"` on a truncation:
# an absent value rendered as a successful measurement.
#
# `unpriced-turn (…)` is the stop value for it. It is a POSITIVE statement that
# the harness cannot price this run, and it is deliberately NOT a variant of
# `max-cost` — the cap did not trip, the cap became inoperable.
UNPRICED_STOP = "unpriced-turn"


def turn_cost(usage):
    """The USD cost of one turn, or None when the provider did not price it.

    🔴 None MEANS "UNKNOWN", AND THE CALLER MUST NOT TREAT IT AS ZERO. Four
    shapes reach here and three of them are the defect: no `usage` object at
    all, a `usage` with no `cost` key, `cost: null`, and a `cost` that is not a
    number. A provider that SAYS a turn cost $0 (a free route) is making a
    claim and is honoured — `0.0` is a price, absence is not. That distinction
    is the whole point: it keeps a genuinely free model runnable while refusing
    to invent a figure for a model whose bill we cannot see.

    `bool` is excluded explicitly because `isinstance(True, int)` is True in
    Python, and `cost: true` is not a price.
    """
    if not isinstance(usage, dict) or "cost" not in usage:
        return None
    value = usage["cost"]
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return None
    return float(value)


# A provider saying "the model chose to stop". OpenRouter normalises to `stop`;
# the others are spellings that reach us when a provider's own value is passed
# through. Anything OUTSIDE this set — an absent value included — is not
# evidence that the reply completed.
NATURAL_STOP = ("stop", "end_turn", "stop_sequence", "eos", "complete")
# The output budget ran out. `length` is OpenRouter's normalised spelling; the
# rest are provider-native values seen in the wild.
TRUNCATED_STOP = ("length", "max_tokens", "model_length", "max_output_tokens")


def classify_stop(finish_reason, content) -> str:
    """The terminal `stop` value for a turn that made no tool call.

    🔴 TRUNCATION OUTRANKS CONTENT. A `length` finish with a non-empty
    `content` is still a report that was cut off mid-sentence, and grading it
    as a finished one is the same confound in a quieter form.

    🔴 AN UNRECOGNISED OR ABSENT `finish_reason` MAPS TO `stopped-unknown:<v>`,
    NEVER TO `finished`. The whole measured defect is an unrecognised terminal
    condition rendered as success; defaulting the unknown case to success
    reintroduces it for every provider whose vocabulary we have not met yet.
    `finished` is a POSITIVE claim that the provider said the model stopped on
    its own, so the harness may only make it when the provider actually did.
    The raw value rides in the string (`none` when absent) so a new vocabulary
    word is diagnosable from the one-line `.out` summary without reopening the
    transcript.
    """
    fr = (finish_reason or "").strip().lower()
    if fr in TRUNCATED_STOP:
        return "truncated"
    if fr in NATURAL_STOP:
        return "finished" if (content or "").strip() else "empty-reply"
    return "stopped-unknown:" + (fr or "none")


def reasoning_echo(msg: dict) -> dict:
    """The provider's own reasoning, in the shape it must be sent BACK in.

    🔴 WHY THIS EXISTS. The loop used to append only `content` + `tool_calls`
    to the history. For a model whose `content` is `null` on 66 of 67 turns —
    measured, `ab-genpost-glm-01` — the model's entire contribution to its own
    history was the text of the shell commands it ran, and 87.8% of its output
    tokens were discarded the moment they arrived. Each turn then re-derived
    what the previous turn had already worked out, inside the same budget that
    truncated it.

    🔴 `reasoning_details` VERBATIM, NOT A RECONSTRUCTED STRING. The blocks
    carry provider signatures, and a provider that validates them rejects
    anything rebuilt from the flat text. `reasoning` is the fallback for
    providers that return only that shape. Both are simply absent for providers
    that return neither, which is what keeps the default request shape
    unchanged for a non-reasoning model.
    """
    out = {}
    details = msg.get("reasoning_details")
    if isinstance(details, list) and details:
        out["reasoning_details"] = details
    text = msg.get("reasoning")
    if isinstance(text, str) and text.strip():
        out["reasoning"] = text
    return out


# ─────────────────────────────────────────────────────────────────────────────
# The credential path.
#
# 🔴 THE DEFECT THIS REPLACES. `--agent-env VAR=VALUE` used to be the only way
# to get a variable into a trial container, and `runner.py` wrote its value
# verbatim into the `start` record of `runs/<trial>/transcript.jsonl`. Using it
# to carry a token therefore wrote a live account credential, in plaintext, into
# a file that persists on disk and is read and quoted by humans and agents
# afterwards. `--credential-file` exists so the only available credential path is
# not that one.
#
# The three surfaces a secret must not reach, and what keeps it off each:
#
#   argv      — the value never appears on any command line. `docker cp` takes a
#               PATH; `docker exec` runs a fixed installer script that takes a
#               path and a username. `--agent-env` is untouched and is still the
#               wrong tool for a secret; see the flag's help text.
#   transcript — every value written by rec() goes through the Redactor below,
#               so even a model that `cat`s the config file cannot put the token
#               in the record. What IS recorded is `credentialed: true` plus a
#               sha256 PREFIX of the file, which is not reversible and is not a
#               substring of the secret.
#   the container's shell history — `docker exec ... bash -lc <cmd>` is
#               non-interactive, and bash writes no history file unless the
#               shell is interactive. Nothing in the install path contains the
#               value anyway, so there is nothing for a history file to hold.
#
# ⚠ WHAT THIS DOES NOT DO. The credential is a real file inside the container
# and the model runs shell as a user who can read it. That is unavoidable: the
# CLI must be able to authenticate. The guarantee is about what LEAVES the
# container and lands in the artifacts an operator keeps — not about hiding the
# credential from the trial.
# ─────────────────────────────────────────────────────────────────────────────

# The config.yaml keys whose values are secret. `scope`, `auth_kind`, `base_url`
# and `token_expiry` are not.
SECRET_KEYS = ("access_token", "refresh_token", "token")
# Below this length a "secret" is more likely to be a word that appears
# everywhere in ordinary output, and redacting it would mangle the transcript
# without protecting anything.
MIN_SECRET_LEN = 8
# Where the credential lands inside the container, and the staging path it
# passes through. Neither carries a value.
CRED_STAGE = "/tmp/.dogfood-credential"
INSTALL_SH = r"""
set -e
u="$1"
# 🔴 RESOLVE THE TRIAL USER'S HOME, AND FAIL IF IT CANNOT BE RESOLVED. This runs
# as root (docker cp writes the staging file as root, and a non-root trial user
# could neither chown it nor remove it from a sticky /tmp), so `$HOME` here is
# ROOT'S home, not the trial user's. Falling back to it would install the
# credential where the trial cannot read it — and the trial would then grade as
# an ordinary "not authenticated" failure, which is the confound this whole
# harness exists not to introduce. Refuse instead; the caller turns a non-zero
# exit into a recorded `credential install failed`.
# ⚠ `if`, not `[ … ] || { … && … ; }`. Under `set -e` that idiom EXITS when the
# inner `&&` is false — so a missing /home/<u> would kill the script at the
# first fallback and the message below would never print.
h=$(getent passwd "$u" 2>/dev/null | cut -d: -f6)
if [ -z "$h" ] && [ -d "/home/$u" ]; then h="/home/$u"; fi
if [ -z "$h" ] && [ "$u" = "root" ] && [ -d /root ]; then h=/root; fi
if [ -z "$h" ]; then
  echo "cannot resolve a home directory for container user '$u'" >&2
  exit 1
fi
d="$h/.config/civitai"
mkdir -p "$d"
cp """ + CRED_STAGE + r""" "$d/config.yaml"
chmod 600 "$d/config.yaml"
chown "$u" "$h/.config" "$d" "$d/config.yaml" 2>/dev/null || true
rm -f """ + CRED_STAGE + r"""
test -s "$d/config.yaml"
printf 'installed %s\n' "$d/config.yaml"
"""


def _digest(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def secrets_in(blob: str) -> list:
    """Every string from a credential file that must never be reproduced.

    Deliberately over-collects: the whole trimmed file (a bare-token config is
    exactly that) plus the value of every secret-bearing `key: value` line. A
    string that is not actually secret costs a redaction; a secret that is
    missed costs the whole point of this flag.
    """
    found = []
    whole = blob.strip()
    if len(whole) >= MIN_SECRET_LEN:
        found.append(whole)
    for line in blob.splitlines():
        if ":" not in line:
            continue
        k, _, v = line.partition(":")
        if k.strip().lower() not in SECRET_KEYS:
            continue
        v = v.strip().strip('"').strip("'")
        if len(v) >= MIN_SECRET_LEN:
            found.append(v)
    # Longest first, so a substring never redacts before its superstring and
    # leave the superstring's remainder visible.
    return sorted(set(found), key=len, reverse=True)


class Redactor:
    """Replaces known secrets with a non-reversible marker, everywhere.

    🔴 IT IS APPLIED AT THE WRITE, NOT AT THE SOURCE. A trial's tool output is
    model-authored shell output: there is no list of places a token could
    surface. Filtering every value on its way into the transcript, the command
    log and stdout is the only version of this that cannot be walked around by
    the model doing something nobody predicted.
    """

    def __init__(self, secrets=()):
        self.pairs = [(s, "[REDACTED:%s]" % _digest(s.encode())[:8]) for s in secrets]

    def __bool__(self):
        return bool(self.pairs)

    def text(self, value: str) -> str:
        for secret, marker in self.pairs:
            value = value.replace(secret, marker)
        return value

    def scrub(self, value):
        """Recursively redact every string inside a JSON-shaped value."""
        if isinstance(value, str):
            return self.text(value)
        if isinstance(value, dict):
            return {k: self.scrub(v) for k, v in value.items()}
        if isinstance(value, list):
            return [self.scrub(v) for v in value]
        return value


def install_credential(container: str, user: str, path: str) -> str:
    """Put the operator's credential inside the container without it ever
    appearing in an argument, and return the installer's own report line."""
    cp = subprocess.run(["docker", "cp", path, f"{container}:{CRED_STAGE}"],
                        capture_output=True, text=True)
    if cp.returncode != 0:
        raise RuntimeError(f"docker cp of the credential failed: {cp.stderr.strip()}")
    # As root: `docker cp` writes the staging file as root, so a non-root trial
    # user could neither chown it nor remove it from a sticky /tmp.
    inst = subprocess.run(
        ["docker", "exec", "-u", "0", container, "sh", "-c", INSTALL_SH, "sh", user],
        capture_output=True, text=True)
    if inst.returncode != 0:
        raise RuntimeError(f"installing the credential failed: {inst.stderr.strip()}")
    return inst.stdout.strip()


# ─────────────────────────────────────────────────────────────────────────────
# The run caps.
#
# 🔴 READ THIS BEFORE CALLING ANY OF IT A GUARD. These are MECHANICAL — a capped
# command is never executed, the counter lives in this process and the model
# cannot reach it — but they classify the COMMAND TEXT. A model that writes a
# command into a file and runs the file, or builds it at runtime from pieces
# this classifier does not see, is not stopped. The fail-closed rule below
# narrows that (a segment naming both the CLI and a dangerous verb, with no
# invocation this can parse, is refused), and it does not close it.
#
# The threat model is the one the dogfood-3 handoff settled on: a COOPERATIVE
# agent. These caps stop the ordinary accident — a loop that regenerates, a
# submit aimed at the wrong directory, a withdraw typed against a real listing —
# not an adversary. The bound people want for an adversary needs a second uid, a
# container with no credential in it, or a platform-capped token; a better
# classifier is not that.
# ─────────────────────────────────────────────────────────────────────────────

# `app` subcommands that change something on the account or in the moderation
# queue. Everything else (`list`, `view`, `validate`, `metrics`, `doctor`,
# `pull`, `dev-token`, `dev-tunnel`) is read-only and ungated.
#
# 🔴 `status` USED TO BE LISTED ON THAT UNGATED LINE AND IT MADE TWO CLAIMS,
# ONE OF WHICH IS FALSE. `civitai app status` is indeed a read. `civitai app
# listing status` is NOT — and it is not ungated either: `listing` is in the
# tuple below, so the whole group including `status` goes through _prefix_ok.
# The comment said otherwise and was read as the authority: a report derived
# from it concluded `app listing status` was "classified as a read by the
# runner's APP_MUTATING list", which is the opposite of what this code does.
#
# 🔴 WHY `app listing status` IS NOT A READ, MEASURED ON A REAL ACCOUNT. On a
# LIVE listing it calls `getMyListingForEdit`, which idempotently OPENS a
# SHADOW REVISION DRAFT server-side (the CLI's own `--help` says so, and
# `internal/appapi/listing.go` contrasts it with the side-effect-free
# `revisionOfId` read). It submits nothing and destroys nothing, so the draft
# is an annoyance rather than a loss — but there is NO `discard-revision`
# command in this CLI, so nothing the trial or the operator can run afterwards
# closes it. It happened to the operator's `panorama-360` listing on
# 2026-09-25. That is why it stays inside the prefix gate rather than being
# moved to the ungated list, and why it is NOT promoted to the refused-by-
# default sets below: refusing it would take away the trial's only way to
# observe its own listing, which changes what the harness measures, for a side
# effect that costs nothing irreversible.
APP_MUTATING = ("init", "create", "submit", "listing")
# Refused outright unless --allow-withdraw. `app withdraw` permanently destroys
# a listing's captioned screenshots (measured, claudedocs/handoff-dogfood-3.md),
# it takes a publication-request id rather than a slug so no prefix check can
# see what it targets, and the account running a credentialed trial owns real
# published listings. There is no accident-shaped reason a trial needs it.
APP_DESTRUCTIVE = ("withdraw",)
# Refused outright unless --allow-listing-text. These are `app listing`
# SUB-subcommands, so they are matched at argv[2] rather than argv[1].
#
# 🔴 `set-text` IS THE ONE LISTING VERB WITH NO REVISION AND NO UNDO. Every
# media verb (`set-icon`, `set-cover`, `add-screenshot`, `rm-screenshot`,
# `reorder`) on a LIVE listing stages onto a shadow revision that a moderator
# has to approve — the live listing is untouched meanwhile, so a mistake is
# recoverable by not submitting. `set-text` is different by design and says so
# in its own `--help`: tagline/description/category are not "material" changes,
# so the patch applies IN PLACE on every listing status. It is public the
# moment it returns, it is one proc with no transaction to roll back, and this
# CLI has no command that restores the previous value.
#
# 🔴 AND THE PREFIX GATE IS NOT A SUBSTITUTE FOR THIS, BECAUSE IT IS OPTIONAL.
# `armed` is true on a credential ALONE; `_prefix_ok` returns None immediately
# when `--app-prefix` is empty. So a credentialed run without a prefix — which
# driver.sh refuses but a direct `runner.py` invocation does not — gates
# nothing, and `set-text` would reach any listing on the account. `withdraw`'s
# refusal is unconditional under `armed` for the same reason, and this one
# matches it. A brief that genuinely needs to write listing copy turns it on
# with one flag; nothing else does. Deliberately NOT threaded through
# driver.sh, exactly like --allow-withdraw.
APP_LISTING_DESTRUCTIVE = ("set-text",)
# Verbs that make a bare mention of the CLI worth refusing when this classifier
# cannot parse an invocation out of the segment.
DANGEROUS_VERBS = ("generate", "submit", "withdraw", "listing")
# Wrappers to step over when looking for the executable.
WRAPPERS = ("env", "sudo", "nice", "ionice", "stdbuf", "nohup", "command", "exec", "builtin")

# ─────────────────────────────────────────────────────────────────────────────
# WHICH FLAGS CAN NAME AN APP — the ledger the prefix cap parses argv with.
#
# 🔴 WHY THIS EXISTS: THE CAP USED TO TREAT EVERY FLAG VALUE AS A SLUG, AND THAT
# COST A MEASUREMENT. `civitai app create ab-image-generator --template static`
# was REFUSED on trial `ab-genpost-dsv4-02` (2026-09-25) because `static` — the
# value of `--template` — matched the slug shape and did not start with the
# trial's prefix. The app name was already correct. The agent believed the
# refusal, dropped `--template static`, and fell back to the 320 KB `page-money`
# default: a FALSE refusal silently changed which scaffold the experiment
# measured. Two of that trial's three refusals were this.
#
# 🔴 AND WHY IT IS A LEDGER RATHER THAN "SKIP FLAGS". Skipping every token that
# starts with `-` is what #698 fixed: `--slug=some-other-app` then yielded no
# candidate at all, fell through to the "read every block.manifest.json under
# /work" branch, and was ACCEPTED while `--slug` pointed at a real listing on the
# operator's account. So the classifier cannot ignore flags and it cannot treat
# them all alike — it has to know which ones carry an app identity.
#
# Roles:
#   "slug"  — the value IS or SELECTS an app. Always a candidate, in both the
#             attached (`--slug=x`) and separated (`--slug x`) spelling.
#   "value" — the flag consumes the next token, and that token is not an app.
#             Its value is NOT a candidate, and — the half that fixes the defect
#             above — it is not mistaken for a positional either.
#   "bool"  — the flag consumes nothing, so the next bare token is still a
#             positional.
#
# 🔴 AN UNKNOWN FLAG DEFAULTS TO "bool", WHICH IS THE OVER-REFUSING DIRECTION ON
# PURPOSE. If the CLI gains `--foo bar`, `bar` is read as a positional and the
# trial gets a false refusal it can report — noisy, and identical to the
# behaviour before this change. The opposite default would silently stop
# collecting a real slug. The ledger is reconciled against `internal/cmd` by
# TestDogfoodGatedFlagLedgerCoversTheCLI (dogfood_classifier_precision_test.go),
# which fails when the CLI's flag set GROWS or SHRINKS — because a NEW
# slug-bearing flag nobody ledgers is a silent hole in the cap, not a false
# refusal.
GATED_FLAGS = {
    # --- Flags whose value names or selects an app. ---
    # The blockId itself.
    "slug": "slug",
    # "fork from an existing published app slug".
    "from": "slug",
    # `app init`/`app create` SLUGIFY the display name into the blockId when
    # --slug is absent, so this names an app just as directly.
    "name": "slug",
    # On `app listing` this picks the directory whose block.manifest.json
    # supplies the blockId — i.e. it selects the target listing. (On
    # init/create it is only an output path, but a flag gets one role; the
    # stricter one is correct, and `--dir=sensei` is a pinned arm of
    # TestDogfoodAttachedSlugFlagDoesNotBypassThePrefixCap.)
    "dir": "slug",

    # --- Flags that consume a value which is not an app. ---
    "template": "value", "t": "value",
    "out": "value", "o": "value",
    "caption": "value",
    "changelog": "value",
    "tagline": "value",
    "description": "value",
    "category": "value",
    # ⚠ `--clear` IS BOOL ON `set-source-repo` AND StringSlice ON `set-text`.
    # It is ledgered "value" for the one that can carry a slug-shaped token:
    # `set-text --clear tagline` would otherwise offer `tagline` as a candidate
    # and false-refuse. `set-source-repo`'s positional is a URL, which cannot
    # match the slug shape, so consuming it costs nothing.
    "clear": "value",

    # --- Flags that consume nothing. ---
    "yes": "bool", "y": "bool",
    "package-only": "bool",
    "skip-validate": "bool",
    "allow-downgrade": "bool",
    "allow-dirty": "bool",
    "allow-oversize": "bool",
    "json": "bool",
    # root's persistent flags, which every gated command also accepts.
    "no-update-check": "bool",
    "no-color": "bool",
    "color": "bool",
}

_SEG = re.compile(r"\|\||&&|[;|&\n]")
_ASSIGN = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*=")
# A here-document opener: `<<WORD`, `<< 'WORD'`, `<<-"WORD"`.
#
# 🔴 THE LOOKAROUNDS ARE WHAT KEEP A HERESTRING OUT. `<<<` is a herestring, not a
# heredoc — it has no body and consumes no following lines. Without `(?<!<)` the
# regex simply starts one character later and matches the LAST TWO of the three
# `<`, so `grep x <<< "civitai generate"` was read as opening a heredoc delimited
# by `civitai generate` and every following line was dropped unclassified.
# Measured while writing the table guard; it is the under-classifying direction,
# i.e. the one that hides a real command.
_HEREDOC_OPEN = re.compile(r"(?<!<)<<(?!<)(-?)\s*(?:'([^']*)'|\"([^\"]*)\"|([A-Za-z_][A-Za-z0-9_]*))")


def strip_heredoc_bodies(command: str) -> str:
    """The command with every here-document BODY removed.

    🔴 A FILE BEING WRITTEN IS DATA, NOT A COMMAND, AND CLASSIFYING IT AS ONE
    COST TWO STEPS. On trial `ab-genpost-dsv4-02` step 25 the agent wrote its own
    `block.manifest.json` with `cat > … << 'ENDOFFILE'`. `segments()` splits on
    newlines, so every LINE of that JSON became a segment to classify, and one of
    them —

        "posts:write:self": "Posts generated images to the user's Civitai profile…"

    — names "Civitai" and contains "generate" inside "generated", which is
    exactly the fail-closed rule's trigger: a segment that mentions the CLI and a
    dangerous verb but yields no invocation. The write was refused. The agent's
    next move was `python3`, which is absent from the trial image (rc=127), so the
    one false refusal cost two steps.

    The body of a here-document is stdin for the command on the opener line. It
    is never executed by the shell, so it must never be classified. The opener
    line itself IS command text and is kept.

    An UNTERMINATED heredoc swallows the rest of the input, and that is faithful
    rather than a hole: bash also treats everything to EOF as the body, so a
    trailing `eval "civitai app submit"` under an unterminated `<< EOF` is written
    into the file and not run.

    ⚠ WHAT THIS DOES NOT COVER, stated rather than implied: inline redirect
    content that is an ARGUMENT — `echo '…' > f`, `printf … > f` — is still
    command text to this classifier, because to a shell it genuinely is. A
    literal `civitai generate` inside such a string is still refused. That is the
    over-refusing direction and it is not what was measured.
    """
    lines = command.split("\n")
    kept = []
    i = 0
    while i < len(lines):
        line = lines[i]
        kept.append(line)
        i += 1
        for m in _HEREDOC_OPEN.finditer(line):
            word = m.group(2) if m.group(2) is not None else (
                m.group(3) if m.group(3) is not None else m.group(4))
            dashed = m.group(1) == "-"
            while i < len(lines):
                probe = lines[i]
                i += 1
                # `<<-` strips leading TABS from the terminator. Comparing on a
                # stripped line is more lenient than bash, which ends the body
                # EARLIER than bash would — i.e. it classifies MORE text, the
                # over-refusing direction.
                candidate = probe.lstrip("\t") if dashed else probe
                if candidate.strip() == word:
                    break
    return "\n".join(kept)


def segments(command: str):
    """Split a shell command into the pieces that could each be an invocation.

    Coarse on purpose: it over-splits (a `;` inside quotes becomes two
    segments), which can only ever produce MORE things to classify, never fewer.
    """
    for seg in _SEG.split(command):
        seg = seg.strip()
        if seg:
            yield seg


def invocation(segment: str):
    """The civitai CLI's argv for this segment, or None if it invokes something
    else. Returns a list, possibly empty (a bare `civitai`)."""
    try:
        toks = shlex.split(segment)
    except ValueError:
        toks = segment.split()
    i = 0
    while i < len(toks) and _ASSIGN.match(toks[i]):
        i += 1
    while i < len(toks) and toks[i] in WRAPPERS:
        i += 1
    if i >= len(toks):
        return None
    if toks[i].rsplit("/", 1)[-1] != "civitai":
        return None
    return [t for t in toks[i + 1:]]


ARG_ORIGIN = "argument"


def slug_candidates(argv, skip: int):
    """Every token after `skip` leading subcommand words that could name an app,
    paired with WHERE it came from — `ARG_ORIGIN`, or the flag spelling
    (`"--slug"`, `"-t"`). The origin is what lets a refusal name the offending
    token's ROLE instead of telling the trial to rename an app whose name was
    never the problem.

    Two defects meet here and the fix has to satisfy both.

    🔴 (A) `--slug=NAME` IS A TOKEN THAT STARTS WITH `-` AND CARRIES THE TARGET.
    Before #698 this dropped every token beginning with `-`, so the attached form
    of `--slug` yielded NO candidate at all, fell through to _prefix_ok's "read
    every block.manifest.json under /work" branch, and was ACCEPTED: the
    workspace manifests all carry the prefix while `--slug` pointed somewhere
    else entirely. A bypass of the app-prefix cap for EVERY gated verb. Both
    spellings of every "slug"-role flag in `GATED_FLAGS` are therefore collected.

    🔴 (B) A FLAG'S VALUE IS NOT AN APP NAME JUST BECAUSE IT IS SLUG-SHAPED.
    #698's fix collected every non-flag token, so `--template static` offered
    `static` and `civitai app create ab-image-generator --template static` was
    REFUSED with "Rename the app and retry" — while the app name was correct. So
    a flag with a "value" role CONSUMES its next token, which is how `static`
    stops being mistaken for a positional.

    The two are not in tension once the classifier knows the flags: (A) is about
    which flag VALUES are collected, (B) about which bare tokens are positionals.
    `GATED_FLAGS` answers both, and an unknown flag is assumed to take nothing —
    the over-collecting, over-refusing default.

    Everything after a bare `--` is a positional, as it is to cobra.
    """
    out = []
    i, n = skip, len(argv)
    while i < n:
        tok = argv[i]
        i += 1
        if tok == "--":
            out.extend((rest, ARG_ORIGIN) for rest in argv[i:])
            return out
        if tok.startswith("--") and len(tok) > 2:
            name, eq, attached = tok[2:].partition("=")
            role = GATED_FLAGS.get(name, "bool")
            if eq:
                if role == "slug" and attached:
                    out.append((attached, "--" + name))
                continue
            if role in ("slug", "value") and i < n:
                value = argv[i]
                i += 1
                if role == "slug":
                    out.append((value, "--" + name))
            continue
        if tok.startswith("-") and len(tok) > 1:
            # A shorthand cluster, as cobra parses it: `-y`, `-ty`, `-t static`,
            # `-tstatic`, `-t=static`. The first value-taking shorthand in the
            # cluster takes the remainder (or the next token) and ends it.
            shorts = tok[1:]
            for k, ch in enumerate(shorts):
                role = GATED_FLAGS.get(ch, "bool")
                if role == "bool":
                    continue
                rest = shorts[k + 1:]
                if rest.startswith("="):
                    rest = rest[1:]
                if not rest and i < n:
                    rest = argv[i]
                    i += 1
                if role == "slug" and rest:
                    out.append((rest, "-" + ch))
                break
            continue
        out.append((tok, ARG_ORIGIN))
    return out


def origin_phrase(origin: str) -> str:
    """How a refusal should describe where a candidate came from."""
    if origin == ARG_ORIGIN:
        return "the app-name argument"
    return "the value of `%s`" % origin


class Caps:
    """The mechanical run bounds. `judge()` returns None to allow, or a refusal
    string that is handed to the model INSTEAD of running the command."""

    def __init__(self, app_prefix="", max_generations=None, max_submissions=None,
                 allow_withdraw=False, allow_listing_text=False,
                 manifest_slugs=lambda: []):
        self.app_prefix = app_prefix or ""
        self.max_generations = max_generations
        self.max_submissions = max_submissions
        self.allow_withdraw = allow_withdraw
        self.allow_listing_text = allow_listing_text
        self.manifest_slugs = manifest_slugs
        self.generations = 0
        self.submissions = 0

    def _prefix_ok(self, argv, skip):
        """Refusal string if this command targets a slug outside the prefix."""
        if not self.app_prefix:
            return None
        named = slug_candidates(argv, skip)
        offenders = [(s, origin) for s, origin in named
                     if re.fullmatch(r"[a-z0-9][a-z0-9-]*", s) and not s.startswith(self.app_prefix)]
        if offenders:
            # 🔴 THE MESSAGE NAMES THE TOKEN'S ROLE, AND THE OLD ONE DID NOT.
            # It always ended "Rename the app and retry." — which on trial
            # `ab-genpost-dsv4-02` was said about `static`, the value of
            # `--template`, while the app name was already correct. The agent
            # followed it, dropped `--template static`, and measured a different
            # scaffold than the brief asked for. A remedy that names the wrong
            # fix is worse than no remedy: it is actionable and wrong.
            flags = []
            for _, origin in offenders:
                if origin != ARG_ORIGIN and origin not in flags:
                    flags.append(origin)
            renaming = any(origin == ARG_ORIGIN for _, origin in offenders)
            if flags and renaming:
                remedy = ("Rename the app, and point %s at an app whose name starts with %r."
                          % (" / ".join("`%s`" % f for f in flags), self.app_prefix))
            elif flags:
                remedy = ("Point %s at an app whose name starts with %r — the app name itself is "
                          "not what was refused." % (" / ".join("`%s`" % f for f in flags), self.app_prefix))
            else:
                remedy = "Rename the app and retry."
            return ("refused by the run harness: this trial may only touch apps whose name "
                    "starts with %r, and this command names %s. %s"
                    % (self.app_prefix,
                       ", ".join("%r (%s)" % (s, origin_phrase(o)) for s, o in offenders),
                       remedy))
        if named:
            return None
        # No slug on the command line: the CLI will read block.manifest.json.
        # Require EVERY manifest in the workspace to carry the prefix — the
        # container started empty, so all of them were created by this trial.
        slugs = self.manifest_slugs()
        bad = [s for s in slugs if s and not s.startswith(self.app_prefix)]
        if bad:
            return ("refused by the run harness: this trial may only touch apps whose name "
                    "starts with %r, and the workspace holds a manifest for %s. Fix the "
                    "blockId in block.manifest.json and retry."
                    % (self.app_prefix, ", ".join(repr(b) for b in bad)))
        return None

    def judge(self, command: str):
        # A here-document body is stdin for the opener's command, never something
        # the shell executes — see strip_heredoc_bodies for the measured false
        # refusal this removes.
        for seg in segments(strip_heredoc_bodies(command)):
            argv = invocation(seg)
            if argv is None:
                # 🔴 FAIL CLOSED on a segment that names the CLI and a dangerous
                # verb but yields no invocation this can read — `eval "civitai
                # app submit"`, `sh -c 'civitai generate …'`. It is narrow by
                # design: `command -v civitai` names no verb and runs.
                #
                # ⚠ THIS COMMENT USED TO LIST `c=civitai` AND THAT WAS FALSE —
                # measured at 6579978 and here. `c=civitai; $c app submit` splits
                # into a segment that names the CLI with NO verb and a segment
                # that names a verb with NO "civitai", so neither trips the test
                # below. It is a real gap, it is unchanged by this commit, and it
                # is recorded rather than implied so nobody reads the rule as
                # wider than it is.
                low = seg.lower()
                if "civitai" in low and any(v in low for v in DANGEROUS_VERBS):
                    return ("refused by the run harness: this segment names the civitai CLI and "
                            "a spending or publishing verb but is not a form the harness can "
                            "read (%r). Run the command directly rather than through eval, a "
                            "nested shell or a variable." % seg[:120])
                continue
            if not argv:
                continue
            if argv[0] == "generate":
                if self.max_generations is not None and self.generations >= self.max_generations:
                    return ("refused by the run harness: the generation cap for this trial is %d "
                            "and it has been reached. Do not retry; report this and stop "
                            "generating." % self.max_generations)
                self.generations += 1
                continue
            if argv[0] != "app" or len(argv) < 2:
                continue
            sub = argv[1]
            if sub in APP_DESTRUCTIVE and not self.allow_withdraw:
                return ("refused by the run harness: `civitai app %s` is disabled for this "
                        "trial. It permanently destroys a listing's captioned screenshots and "
                        "names a publication-request id rather than an app, so the harness "
                        "cannot tell which listing it would hit." % sub)
            if sub not in APP_MUTATING:
                continue
            # Checked BEFORE the prefix gate, because the prefix gate is
            # optional and this refusal is not — see APP_LISTING_DESTRUCTIVE.
            if (sub == "listing" and len(argv) >= 3
                    and argv[2] in APP_LISTING_DESTRUCTIVE
                    and not self.allow_listing_text):
                return ("refused by the run harness: `civitai app listing %s` is disabled for "
                        "this trial. It applies IN PLACE on a listing of any status — no "
                        "revision, no moderator review, public the moment it returns — and "
                        "this CLI has no command that restores the previous value." % argv[2])
            if sub == "submit":
                if self.max_submissions is not None and self.submissions >= self.max_submissions:
                    return ("refused by the run harness: the submission cap for this trial is %d "
                            "and it has been reached. Do not retry; report this and stop "
                            "submitting." % self.max_submissions)
            # `listing` carries its own subcommand, so the slug (when given at
            # all) sits one token further along.
            refusal = self._prefix_ok(argv, 3 if sub == "listing" else 2)
            if refusal:
                return refusal
            if sub == "submit":
                self.submissions += 1
        return None


def key() -> str:
    k = os.environ.get("OPENROUTER_API_KEY", "").strip()
    if k:
        return k
    sys.exit("no OpenRouter key: export OPENROUTER_API_KEY (this spends real money)")


def sh(container: str, user: str, command: str, timeout: int = 300) -> str:
    """Run one command inside the trial container."""
    p = subprocess.run(
        ["docker", "exec", "-u", user, "-w", "/work", "-i", container,
         "bash", "-lc", command],
        capture_output=True, timeout=timeout,
    )
    out = p.stdout.decode("utf-8", "replace")
    err = p.stderr.decode("utf-8", "replace")
    body = out + (("\n[stderr]\n" + err) if err.strip() else "")
    if len(body) > MAX_OUT:
        body = body[:MAX_OUT] + f"\n[... truncated, {len(body)} bytes total]"
    return f"exit code: {p.returncode}\n{body}" if body.strip() else f"exit code: {p.returncode}\n(no output)"


def workspace_slugs(container: str, user: str) -> list:
    """Every `blockId` declared under /work. Same discovery the render oracle
    does, and for the same reason: reading the trial id or a directory name
    would be an assertion by whoever named the trial, not a measurement."""
    p = subprocess.run(
        ["docker", "exec", "-u", user, "-w", "/work", container, "bash", "-lc",
         "find /work -maxdepth 4 -name block.manifest.json -not -path '*/node_modules/*' "
         "-exec cat {} + 2>/dev/null"],
        capture_output=True, text=True)
    return re.findall(r'"blockId"\s*:\s*"([^"]*)"', p.stdout or "")


def call(model: str, messages: list, api_key: str, max_tokens: int) -> dict:
    payload = json.dumps({
        "model": model, "messages": messages, "tools": TOOLS,
        "tool_choice": "auto", "max_tokens": max_tokens,
    }).encode()
    req = urllib.request.Request(
        API, data=payload, headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
            "HTTP-Referer": "https://github.com/civitai/cli",
            "X-Title": "civitai agent-setup dogfood",
        })
    last = None
    for attempt in range(4):
        try:
            with urllib.request.urlopen(req, timeout=300) as r:
                return json.loads(r.read())
        except urllib.error.HTTPError as e:
            body = e.read().decode("utf-8", "replace")[:600]
            last = f"HTTP {e.code}: {body}"
            if e.code not in (408, 429, 500, 502, 503, 520, 524):
                raise RuntimeError(last)
        except Exception as e:  # noqa: BLE001 — network flake
            last = repr(e)
        time.sleep(5 * (attempt + 1))
    raise RuntimeError(f"openrouter failed after retries: {last}")


def main() -> int:
    ap = argparse.ArgumentParser()
    # 🔴 --model/--image/--trial ARE STILL REQUIRED FOR A RUN. They are not
    # declared `required=True` only because `--print-task` has to work without
    # them — it starts no container and makes no API call, so demanding an
    # OpenRouter model id to preview a string would be theatre. The check below
    # reproduces argparse's own message and its exit code 2; a run that forgets
    # --model fails exactly as before.
    ap.add_argument("--model")
    ap.add_argument("--image")
    ap.add_argument("--trial")
    ap.add_argument("--user", default="root")
    # The app brief. One line, operator-supplied, appended to the hosted URL —
    # see task(). Omit it and the trial is the setup trial this harness has
    # always run.
    ap.add_argument("--brief", default="",
                    help="one-line app brief, appended to the hosted URL as the "
                         "second paragraph of the task. Omitted or empty => the "
                         "task is byte-identical to a setup trial.")
    # 🔴 THE BRIEF'S NAME, BECAUSE ITS PROSE IS NOT AN IDENTIFIER. `--brief`
    # records the brief's TEXT, which is the only thing the model sees and the
    # only thing worth pinning for reproducibility — but a grader has to map
    # that text back to `briefs/<name>.assert.mjs`, and the only way to do that
    # from prose is an exact string match against `briefs/*.brief.txt`. That
    # match stops resolving EVERY already-run trial the moment anyone rewords a
    # brief file, and it cannot resolve an ad-hoc brief at all. Recording the
    # name makes the mapping a fact about the run instead of a re-derivation.
    # oracle.sh prefers it and falls back to the prose match for trials run
    # before this field existed.
    ap.add_argument("--brief-name", default="",
                    help="the name of the brief --brief holds, e.g. `genpost`. "
                         "Recorded in the transcript's `start` record so a "
                         "grader can resolve briefs/<name>.assert.mjs without "
                         "matching prose. Must name an existing "
                         "briefs/<name>.assert.mjs, and when "
                         "briefs/<name>.brief.txt exists its text must equal "
                         "--brief.")
    ap.add_argument("--print-task", action="store_true",
                    help="print the exact user message this invocation would send "
                         "and exit. Starts no container, calls no API, spends "
                         "nothing — it is how the byte-identical-by-default claim "
                         "is checked rather than asserted.")
    ap.add_argument("--agent-env", default="",
                    help="VAR=VALUE set in the container, so the CLI detects that "
                         "agent. NOT FOR SECRETS: this value is recorded verbatim "
                         "in the transcript. Use --credential-file for a token.")
    ap.add_argument("--credential-file", default="",
                    help="path to a civitai config.yaml (or a bare token file) to "
                         "install at ~/.config/civitai/config.yaml inside the "
                         "container. The value never reaches argv, the transcript, "
                         "the command log or stdout; only `credentialed: true` and "
                         "a sha256 prefix of the file are recorded.")
    ap.add_argument("--app-prefix", default="",
                    help="refuse any app-creating or app-mutating civitai command "
                         "whose target app name does not start with this. MECHANICAL "
                         "— the command is not executed — but it classifies command "
                         "TEXT; see the Caps docstring for what that does not cover.")
    ap.add_argument("--max-generations", type=int, default=None,
                    help="refuse `civitai generate` past this many invocations.")
    ap.add_argument("--max-submissions", type=int, default=None,
                    help="refuse `civitai app submit` past this many invocations.")
    ap.add_argument("--allow-withdraw", action="store_true",
                    help="permit `civitai app withdraw`, which is refused by "
                         "default: it destroys a listing's captioned screenshots "
                         "and names a publication-request id no prefix check can "
                         "resolve to an app.")
    ap.add_argument("--allow-listing-text", action="store_true",
                    help="permit `civitai app listing set-text`, which is refused "
                         "by default: it rewrites a listing's public "
                         "tagline/description/category IN PLACE on every listing "
                         "status — no revision, no moderator review, and this CLI "
                         "has no command that restores the previous value.")
    ap.add_argument("--max-steps", type=int, default=40)
    # 🔴 A STEP CAP IS NOT A SPEND CAP. Every turn resends the whole history plus
    # up to MAX_OUT of tool output, so cumulative prompt tokens grow O(n^2) in
    # steps. Observed runs took 3-8 steps and cost ~$0.02-0.10; at the 40-step cap
    # that is roughly 25x, and a model that loops on a failing install — exactly
    # the failure being measured — is the case that reaches it. This is the only
    # bound on money.
    #
    # 🔴 AND IT ONLY BOUNDS MONEY WHILE THE PROVIDER PRICES THE TURNS. There is
    # no price table here and no `/api/v1/models` fallback: the cap is a
    # comparison against a running total that comes entirely from
    # `usage.cost`. A route that stops sending that field does not make the cap
    # fire late, it makes it never fire — so an unpriced turn ends the run
    # outright (`stop: "unpriced-turn (…)"`, see turn_cost above) rather than
    # being counted as $0.
    ap.add_argument("--max-cost", type=float, default=1.0,
                    help="stop the trial once this much USD has been spent "
                         "(default 1.0). A turn the provider does not price "
                         "ends the trial instead of counting as $0 — this cap "
                         "cannot bound a run whose spend is unreported.")
    # 🔴 8000 LEFT THE MODEL 8 TOKENS AFTER ITS REASONING. Measured on
    # `ab-genpost-glm-01`: the final turn spent 7,992 of an 8,000-token budget
    # reasoning and returned `content: null`, which the old loop recorded as
    # `finished`. The per-turn reasoning burn escalated 2,141 -> 4,238 -> 2,021
    # -> 7,992 across that trial; only the first three are UNCENSORED
    # observations, because the fourth is the cap itself and so is a lower
    # bound, not a measurement.
    #
    # 32000 is 4x the budget that was exhausted and ~7.5x the largest burst we
    # have ever seen complete (4,238). It is a CEILING, NOT A SPEND: tokens are
    # billed as generated, and the only bound on money is --max-cost, which is
    # untouched. A model whose provider caps output below this fails LOUDLY —
    # call() raises on a non-retryable HTTP code, the trial dies without an
    # `end` record and driver.sh re-runs it — which is strictly better than a
    # silent truncation graded as a result. ⚠ UNVERIFIED: whether OpenRouter
    # clamps an over-large max_tokens or rejects it is provider-dependent and
    # was not measured (checking it costs a real call). Lower this flag if a
    # model 400s.
    ap.add_argument("--max-tokens", type=int, default=32000,
                    help="output-token ceiling per turn (default 32000). A "
                         "ceiling, not a spend cap — money is bounded by "
                         "--max-cost.")
    # 🔴 THE ESCAPE HATCH FOR THE HISTORY CHANGE, NOT A TUNING KNOB. Carrying
    # reasoning back grows the prompt faster (history is already O(n^2) in
    # steps), so an operator who wants a cell measured under the OLD history
    # shape — or who hits a provider that rejects an echoed block — can turn it
    # off without editing this file. Default ON: discarding 87.8% of a model's
    # output every turn is the defect, not the baseline.
    ap.add_argument("--no-carry-reasoning", dest="carry_reasoning",
                    action="store_false",
                    help="do not send the provider's reasoning blocks back in "
                         "the assistant history (default: send them when the "
                         "provider returns them).")
    ap.add_argument("--out", default=".")
    a = ap.parse_args()

    # 🔴 ONE LINE, ENFORCED. A brief is the INDEPENDENT VARIABLE of the matrix,
    # and a multi-line value is nearly always a file that got splatted onto the
    # command line — which is the one way repo content could reach a trial whose
    # blindness is otherwise a mount namespace. Refuse rather than quietly
    # paste it: a leak here would not show up in any grade, only in a cell that
    # scored well for the wrong reason.
    if "\n" in a.brief or "\r" in a.brief:
        ap.error("--brief must be a single line (got an embedded newline). The "
                 "brief is operator-typed text, not a file — piping a file in is "
                 "how repo content reaches a blind trial.")

    # 🔴 VALIDATED HERE, BEFORE A CONTAINER OR AN API CALL EXISTS. A
    # `--brief-name` that names nothing, or that names a brief whose committed
    # text is not the text being sent, would be recorded as a fact and then
    # believed by every later grade — the cheapest possible moment to catch it
    # is now, and the most expensive is after a paid matrix has run.
    if a.brief_name:
        briefs = pathlib.Path(__file__).resolve().parent / "briefs"
        if not (briefs / f"{a.brief_name}.assert.mjs").is_file():
            ap.error(f"--brief-name {a.brief_name!r} has no assertion at "
                     f"{briefs}/{a.brief_name}.assert.mjs — a grader would "
                     f"resolve this trial to a brief it cannot run.")
        text_file = briefs / f"{a.brief_name}.brief.txt"
        if text_file.is_file() and text_file.read_text().strip() != a.brief.strip():
            ap.error(f"--brief-name {a.brief_name!r} disagrees with --brief: "
                     f"{text_file} holds different text. Recording the name "
                     f"anyway would mislabel every cell of this matrix.")

    if a.print_task:
        # Exactly the bytes task() produced, with nothing appended — so a diff
        # against a recorded baseline is a diff of the task, not of our
        # formatting.
        sys.stdout.write(task(a.brief))
        return 0

    missing = [f"--{n}" for n in ("model", "image", "trial") if not getattr(a, n)]
    if missing:
        ap.error("the following arguments are required: " + ", ".join(missing))

    # Read the credential on the HOST, before anything starts, so a typo'd path
    # fails before a container exists rather than half way through a trial.
    redactor = Redactor()
    cred_sha = ""
    if a.credential_file:
        try:
            cred_bytes = pathlib.Path(a.credential_file).read_bytes()
        except OSError as e:
            ap.error(f"--credential-file: {e}")
        if not cred_bytes.strip():
            ap.error("--credential-file names an empty file; a credentialed trial "
                     "with no credential would grade as an ordinary failure")
        cred_sha = _digest(cred_bytes)[:12]
        redactor = Redactor(secrets_in(cred_bytes.decode("utf-8", "replace")))

    outdir = pathlib.Path(a.out) / a.trial
    outdir.mkdir(parents=True, exist_ok=True)
    log = (outdir / "transcript.jsonl").open("w")
    # 🔴 THE COMMAND LOG IS MECHANICAL AND IS NOT THE TRANSCRIPT. The transcript
    # is a model-facing record that grows with token usage and tool output; this
    # is the flat, greppable list of what actually ran, with the cap counters
    # beside each line, which is what an operator reads after a credentialed run
    # to answer "what did it do". Redacted like everything else.
    cmdlog = (outdir / "commands.log").open("w")

    def rec(kind, **kw):
        log.write(json.dumps(redactor.scrub({"t": time.time(), "kind": kind, **kw})) + "\n")
        log.flush()

    def logcmd(verdict, step, command, caps):
        cmdlog.write("%s step=%s verdict=%s gen=%s sub=%s :: %s\n" % (
            time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), step, verdict,
            caps.generations, caps.submissions,
            redactor.text(command.replace("\n", "\\n"))))
        cmdlog.flush()

    container = f"dogfood-{a.trial}"
    subprocess.run(["docker", "rm", "-f", container],
                   capture_output=True)
    # 🔴 THE CONTAINER BOUNDS THE FILESYSTEM. These bound the rest of it.
    # A trial runs model-authored shell, as root in half the images, and
    # `subprocess` timeouts kill the LOCAL `docker exec` client while whatever it
    # started keeps running inside — so a fork bomb, a `yes > file`, or a runaway
    # install would otherwise burn host CPU and disk for the rest of the matrix
    # and corrupt the timings of the other trials running concurrently.
    # NOT bounded, and stated in the README rather than implied away: NETWORK
    # (egress is necessarily open — the trial must fetch prompt.md and reach npm)
    # and DISK. `--storage-opt size=` was tried and removed: it is accepted only
    # "for overlay over xfs with 'pquota'", so on an ordinary daemon it does not
    # restrict the write, it refuses to START THE CONTAINER — a bound that turns
    # every trial into a failed one on most hosts is worse than a declared gap.
    run = ["docker", "run", "-d", "--name", container,
           "--pids-limit", "512", "--memory", "2g", "--cpus", "2"]
    if a.agent_env:
        run += ["-e", a.agent_env]
    run += [a.image, "sleep", "infinity"]
    cid = subprocess.run(run, capture_output=True, text=True)
    if cid.returncode != 0:
        print(f"container start failed: {cid.stderr}", file=sys.stderr)
        return 1

    # 🔴 `brief` IS RECORDED UNCONDITIONALLY, EMPTY STRING INCLUDED. A graded
    # cell has to be traceable to the brief it was run with, and the trial id
    # cannot carry that — same reason grade.sh reads the agent identity out of
    # the container instead of out of the filename. Always emitting it makes
    # "this was a setup trial" a POSITIVE assertion in the transcript rather
    # than an inference from a missing key, which would be indistinguishable
    # from a transcript written by an older runner.
    #
    # 🔴 THE CREDENTIAL FIELDS ARE THE EXCEPTION, AND DELIBERATELY SO. They are
    # added ONLY on a credentialed run, so an uncredentialed trial's `start`
    # record is byte-for-byte what it has always been and every already-measured
    # grid stays comparable — the same contract task() keeps for the message.
    # That makes absence the signal for "no credential", which is weaker than
    # the positive assertion `brief` gets; the key set is pinned on both sides by
    # TestDogfoodUncredentialedStartRecordIsUnchanged.
    #
    # 🔴 `brief_name` RIDES ALONGSIDE, UNCONDITIONALLY, FOR THE SAME REASON. It
    # is the empty string on a setup trial and on any ad-hoc brief, which is a
    # POSITIVE "this run named no brief" rather than an absence indistinguishable
    # from an older runner's transcript. It exists because the brief's PROSE is
    # not an identifier: a grader mapping text back to
    # `briefs/<name>.assert.mjs` can only do so by exact match, and that match
    # breaks for every already-run trial the moment a brief file is reworded.
    start = dict(trial=a.trial, model=a.model, image=a.image, user=a.user,
                 agent_env=a.agent_env, brief=a.brief, brief_name=a.brief_name,
                 container=container)
    if a.credential_file:
        # 🔴 A MARKER, NEVER THE VALUE. A sha256 prefix is not reversible and is
        # not a substring of the token; it exists so two runs can be told apart
        # and so "which credential was this?" is answerable without holding one.
        start.update(credentialed=True, credential_sha256=cred_sha)
        try:
            start["credential_install"] = install_credential(container, a.user, a.credential_file)
        except RuntimeError as e:
            rec("start", **start)
            rec("end", stop=f"credential install failed: {e}", steps=0,
                usage={"prompt_tokens": 0, "completion_tokens": 0, "cost": 0.0},
                final="", finish_reason=None)
            print(f"credential install failed: {e}", file=sys.stderr)
            log.close()
            cmdlog.close()
            return 1
    rec("start", **start)

    # 🔴 ARMED BY THE PRESENCE OF A CAP OR OF A CREDENTIAL, AND OTHERWISE NOT AT
    # ALL. An uncredentialed setup trial must behave byte-for-byte as it always
    # has: no judging, no extra transcript records, no extra `end` fields. A
    # credential arms them on its own because that is the only run in which any
    # of this can reach a real account — an operator should not have to remember
    # three flags for the bound to exist.
    armed = bool(a.credential_file or a.app_prefix
                 or a.max_generations is not None or a.max_submissions is not None)
    caps = Caps(app_prefix=a.app_prefix,
                max_generations=a.max_generations,
                max_submissions=a.max_submissions,
                allow_withdraw=a.allow_withdraw,
                allow_listing_text=a.allow_listing_text,
                manifest_slugs=lambda: workspace_slugs(container, a.user))
    if armed:
        rec("caps", app_prefix=a.app_prefix, max_generations=a.max_generations,
            max_submissions=a.max_submissions, allow_withdraw=a.allow_withdraw,
            allow_listing_text=a.allow_listing_text)

    api_key = key()
    # One source for the task: the `user` record and the message actually sent
    # are the same bytes by construction, not by two call sites agreeing.
    user_msg = task(a.brief)
    messages = [{"role": "system", "content": SYSTEM},
                {"role": "user", "content": user_msg}]
    rec("user", content=user_msg)

    usage_total = {"prompt_tokens": 0, "completion_tokens": 0, "cost": 0.0}
    steps = 0
    stop = "max-steps"
    final = ""
    finish_reason = None
    # 🔴 PER-TURN, BECAUSE THE TOTAL CANNOT SEE AN ESCALATION. The `.out`
    # summary reported only cumulative usage, so `ab-genpost-glm-01`'s reasoning
    # burn climbing 2,141 -> 4,238 -> 2,021 -> 7,992 into the cap was plainly
    # visible in the transcript and completely invisible in the summary an
    # operator actually reads. One row per assistant turn, cheap and greppable.
    turns = []

    while steps < a.max_steps:
        if usage_total["cost"] >= a.max_cost:
            stop = f"max-cost (${usage_total['cost']:.4f} >= ${a.max_cost})"
            break
        resp = call(a.model, messages, api_key, a.max_tokens)
        u = resp.get("usage") or {}
        usage_total["prompt_tokens"] += u.get("prompt_tokens", 0)
        usage_total["completion_tokens"] += u.get("completion_tokens", 0)
        # 🔴 NOT `+= u.get("cost", 0.0) or 0.0`. See turn_cost: an absent or
        # unreadable figure is UNKNOWN, and adding zero for it is what made
        # --max-cost silently inoperable. The run is stopped below instead.
        this_cost = turn_cost(resp.get("usage"))
        if this_cost is not None:
            usage_total["cost"] += this_cost
        choice = resp["choices"][0]
        # The field the harness used to drop on the floor. Read off the CHOICE,
        # not the message — that is where every OpenAI-shaped API puts it.
        finish_reason = choice.get("finish_reason")
        msg = choice["message"]
        calls = msg.get("tool_calls") or []
        echo = reasoning_echo(msg) if a.carry_reasoning else {}
        # `or 0` on BOTH halves: a provider may send `completion_tokens_details:
        # null`, and may send the key with a null value. Either would make the
        # summary's `sum()` raise at the very end of a trial that already cost
        # money — a crash in the reporting path is the worst place for one.
        reasoning_tokens = (u.get("completion_tokens_details") or {}).get("reasoning_tokens") or 0
        turns.append({"after_steps": steps,
                      "completion_tokens": u.get("completion_tokens", 0),
                      "reasoning_tokens": reasoning_tokens,
                      "finish_reason": finish_reason})
        arec = dict(content=msg.get("content"), tool_calls=calls, usage=u,
                    finish_reason=finish_reason)
        # Only when there IS reasoning, so a non-reasoning provider's record is
        # what it has always been. It is a LENGTH, not the text: enough to prove
        # from the artifact that the echo happened, without doubling a
        # transcript that is already the largest thing a trial leaves behind.
        if echo:
            arec["reasoning_chars"] = sum(
                len(json.dumps(v)) if not isinstance(v, str) else len(v)
                for v in echo.values())
        rec("assistant", **arec)
        messages.append({
            "role": "assistant",
            "content": msg.get("content") or "",
            **echo,
            **({"tool_calls": calls} if calls else {}),
        })
        if not calls:
            final = msg.get("content") or ""
        # 🔴 THE HARD STOP, AND IT OUTRANKS THE TERMINAL STATE. A turn the
        # provider did not price means every later turn is unbounded, so the
        # loop ends HERE — before the tool calls of this turn are executed and
        # before the next `call()` is issued. Nothing is lost by putting it
        # above classify_stop: `finish_reason` and `final` are recorded either
        # way and ride out on the `end` record and the summary, so the terminal
        # state is still readable; what changes is that `stop` stops making the
        # success-shaped claim. A terminal unpriced turn is stopped too — no
        # further money can be spent on it, but `end.usage.cost` would
        # otherwise report a fabricated `0.0` for a call that was really
        # billed, which is the same coercion one axis over.
        #
        # 🔴 ONE CALL IS ALLOWED TO COMPLETE, AND EXACTLY ONE. You cannot know a
        # provider omits `cost` until a response arrives, so the first request
        # is unavoidable and is the floor on what this can bound. It is NOT a
        # grace period: there is no "allow N unpriced turns" knob, because N
        # turns of unknown cost is still unbounded — it is bounded only by a
        # number nobody can convert into money.
        if this_cost is None:
            stop = ("%s (turn %d returned no usable usage.cost, so --max-cost "
                    "cannot bound this run; recorded spend is a LOWER BOUND)"
                    % (UNPRICED_STOP, len(turns)))
            rec("unpriced", turn=len(turns), usage=u, finish_reason=finish_reason)
            break
        if not calls:
            # 🔴 NOT `stop = "finished"`. See classify_stop — a turn with no
            # tool calls is four different outcomes, and three of them are the
            # harness's fault rather than the model's.
            stop = classify_stop(finish_reason, final)
            break
        for c in calls:
            steps += 1
            try:
                args = json.loads(c["function"].get("arguments") or "{}")
            except json.JSONDecodeError:
                args = {}
            cmd = args.get("command", "")
            # 🔴 JUDGED BEFORE IT RUNS. A refused command never reaches the
            # container: the refusal is what the model is handed back, so the cap
            # is a property of this process and not of anything the trial can
            # reach.
            refusal = caps.judge(cmd) if armed else None
            if refusal:
                logcmd("refused", steps, cmd, caps)
                rec("refused", step=steps, command=cmd, reason=refusal)
                messages.append({"role": "tool", "tool_call_id": c["id"],
                                 "content": refusal})
                continue
            logcmd("run", steps, cmd, caps)
            t0 = time.time()
            try:
                result = sh(container, a.user, cmd)
            except subprocess.TimeoutExpired:
                result = "exit code: -1\n[command timed out after 300s]"
            rec("tool", step=steps, command=cmd, result=result,
                secs=round(time.time() - t0, 1))
            messages.append({"role": "tool", "tool_call_id": c["id"],
                             "content": result})

    # `finish_reason` is the LAST one the provider sent, and it is present on
    # every `end` record including `max-steps`/`max-cost` ones — a uniform key
    # set is what lets a reader `jq` a directory of transcripts without
    # branching on which stop it was.
    end = dict(stop=stop, steps=steps, usage=usage_total, final=final,
               finish_reason=finish_reason)
    # `usage` is left at its three historical keys deliberately: per-turn
    # reasoning already rides in each `assistant` record's `usage`, and
    # re-basing `end.usage` would make every already-measured grid's totals
    # non-comparable for no new information.
    summary = {"trial": a.trial, "model": a.model, "image": a.image,
               "stop": stop, "finish_reason": finish_reason,
               "steps": steps, "usage": usage_total,
               "reasoning_tokens": sum(t["reasoning_tokens"] for t in turns),
               "turns": turns}
    if armed:
        end.update(generations=caps.generations, submissions=caps.submissions)
        summary.update(generations=caps.generations, submissions=caps.submissions)
    rec("end", **end)
    print(json.dumps(redactor.scrub(summary)))
    log.close()
    cmdlog.close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
