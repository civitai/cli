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
            [--max-steps 40] [--out <dir>]
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
h=$(getent passwd "$u" 2>/dev/null | cut -d: -f6)
[ -n "$h" ] || h="$HOME"
[ -n "$h" ] || h=/root
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
# queue. Everything else (`list`, `view`, `status`, `validate`, `metrics`,
# `doctor`, `pull`, `dev-token`, `dev-tunnel`) is read-only and ungated.
APP_MUTATING = ("init", "create", "submit", "listing")
# Refused outright unless --allow-withdraw. `app withdraw` permanently destroys
# a listing's captioned screenshots (measured, claudedocs/handoff-dogfood-3.md),
# it takes a publication-request id rather than a slug so no prefix check can
# see what it targets, and the account running a credentialed trial owns real
# published listings. There is no accident-shaped reason a trial needs it.
APP_DESTRUCTIVE = ("withdraw",)
# Verbs that make a bare mention of the CLI worth refusing when this classifier
# cannot parse an invocation out of the segment.
DANGEROUS_VERBS = ("generate", "submit", "withdraw", "listing")
# Wrappers to step over when looking for the executable.
WRAPPERS = ("env", "sudo", "nice", "ionice", "stdbuf", "nohup", "command", "exec", "builtin")

_SEG = re.compile(r"\|\||&&|[;|&\n]")
_ASSIGN = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*=")


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


def positional(argv, skip: int):
    """The first non-flag token after `skip` leading subcommand words.

    ⚠ It does not know which flags take a value, so a slug can be missed (the
    token after `--dir ./x` is `./x`, not a slug). Missing one is safe here: the
    caller falls back to reading every manifest in the container, which is the
    stricter check.
    """
    out = []
    for tok in argv[skip:]:
        if tok.startswith("-"):
            continue
        out.append(tok)
    return out


class Caps:
    """The mechanical run bounds. `judge()` returns None to allow, or a refusal
    string that is handed to the model INSTEAD of running the command."""

    def __init__(self, app_prefix="", max_generations=None, max_submissions=None,
                 allow_withdraw=False, manifest_slugs=lambda: []):
        self.app_prefix = app_prefix or ""
        self.max_generations = max_generations
        self.max_submissions = max_submissions
        self.allow_withdraw = allow_withdraw
        self.manifest_slugs = manifest_slugs
        self.generations = 0
        self.submissions = 0

    def _prefix_ok(self, argv, skip):
        """Refusal string if this command targets a slug outside the prefix."""
        if not self.app_prefix:
            return None
        named = positional(argv, skip)
        offenders = [s for s in named
                     if re.fullmatch(r"[a-z0-9][a-z0-9-]*", s) and not s.startswith(self.app_prefix)]
        if offenders:
            return ("refused by the run harness: this trial may only touch apps whose name "
                    "starts with %r, and this command names %s. Rename the app and retry."
                    % (self.app_prefix, ", ".join(repr(o) for o in offenders)))
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
        for seg in segments(command):
            argv = invocation(seg)
            if argv is None:
                # 🔴 FAIL CLOSED on a segment that names the CLI and a dangerous
                # verb but yields no invocation this can read — `eval "civitai
                # app submit"`, `sh -c 'civitai generate …'`, `c=civitai`. It is
                # narrow by design: `command -v civitai` names no verb and runs.
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


def call(model: str, messages: list, api_key: str) -> dict:
    payload = json.dumps({
        "model": model, "messages": messages, "tools": TOOLS,
        "tool_choice": "auto", "max_tokens": 8000,
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
    ap.add_argument("--max-steps", type=int, default=40)
    # 🔴 A STEP CAP IS NOT A SPEND CAP. Every turn resends the whole history plus
    # up to MAX_OUT of tool output, so cumulative prompt tokens grow O(n^2) in
    # steps. Observed runs took 3-8 steps and cost ~$0.02-0.10; at the 40-step cap
    # that is roughly 25x, and a model that loops on a failing install — exactly
    # the failure being measured — is the case that reaches it. This is the only
    # bound on money.
    ap.add_argument("--max-cost", type=float, default=1.0,
                    help="stop the trial once this much USD has been spent (default 1.0)")
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
    start = dict(trial=a.trial, model=a.model, image=a.image, user=a.user,
                 agent_env=a.agent_env, brief=a.brief, container=container)
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
                usage={"prompt_tokens": 0, "completion_tokens": 0, "cost": 0.0}, final="")
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
                manifest_slugs=lambda: workspace_slugs(container, a.user))
    if armed:
        rec("caps", app_prefix=a.app_prefix, max_generations=a.max_generations,
            max_submissions=a.max_submissions, allow_withdraw=a.allow_withdraw)

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

    while steps < a.max_steps:
        if usage_total["cost"] >= a.max_cost:
            stop = f"max-cost (${usage_total['cost']:.4f} >= ${a.max_cost})"
            break
        resp = call(a.model, messages, api_key)
        u = resp.get("usage") or {}
        usage_total["prompt_tokens"] += u.get("prompt_tokens", 0)
        usage_total["completion_tokens"] += u.get("completion_tokens", 0)
        usage_total["cost"] += u.get("cost", 0.0) or 0.0
        msg = resp["choices"][0]["message"]
        calls = msg.get("tool_calls") or []
        rec("assistant", content=msg.get("content"), tool_calls=calls, usage=u)
        messages.append({
            "role": "assistant",
            "content": msg.get("content") or "",
            **({"tool_calls": calls} if calls else {}),
        })
        if not calls:
            final = msg.get("content") or ""
            stop = "finished"
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

    end = dict(stop=stop, steps=steps, usage=usage_total, final=final)
    summary = {"trial": a.trial, "model": a.model, "image": a.image,
               "stop": stop, "steps": steps, "usage": usage_total}
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
