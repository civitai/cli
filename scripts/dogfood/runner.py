#!/usr/bin/env python3
"""Blind dogfood harness for developer.civitai.com/agent-setup/prompt.md.

One trial = one model driving one throwaway container. The container is the only
thing the model can touch: its single tool shells into it. The repo, the handoff
docs and the operator's home directory are not in that container's filesystem, so
blindness is a mount namespace, not an instruction.

Usage:
  runner.py --model <openrouter-id> --image <docker-image> --trial <id>
            [--brief "<one line>"] [--agent-env CLAUDECODE=1]
            [--max-steps 40] [--out <dir>]
  runner.py --print-task [--brief "<one line>"]   # offline; spends nothing
"""
import argparse
import json
import os
import pathlib
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
    ap.add_argument("--agent-env", default="", help="VAR=VALUE set in the container, "
                                                    "so the CLI detects that agent")
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

    outdir = pathlib.Path(a.out) / a.trial
    outdir.mkdir(parents=True, exist_ok=True)
    log = (outdir / "transcript.jsonl").open("w")

    def rec(kind, **kw):
        log.write(json.dumps({"t": time.time(), "kind": kind, **kw}) + "\n")
        log.flush()

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
    rec("start", trial=a.trial, model=a.model, image=a.image, user=a.user,
        agent_env=a.agent_env, brief=a.brief, container=container)

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
            t0 = time.time()
            try:
                result = sh(container, a.user, cmd)
            except subprocess.TimeoutExpired:
                result = "exit code: -1\n[command timed out after 300s]"
            rec("tool", step=steps, command=cmd, result=result,
                secs=round(time.time() - t0, 1))
            messages.append({"role": "tool", "tool_call_id": c["id"],
                             "content": result})

    rec("end", stop=stop, steps=steps, usage=usage_total, final=final)
    print(json.dumps({"trial": a.trial, "model": a.model, "image": a.image,
                      "stop": stop, "steps": steps, "usage": usage_total}))
    log.close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
