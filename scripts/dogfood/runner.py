#!/usr/bin/env python3
"""Blind dogfood harness for developer.civitai.com/agent-setup/prompt.md.

One trial = one model driving one throwaway container. The container is the only
thing the model can touch: its single tool shells into it. The repo, the handoff
docs and the operator's home directory are not in that container's filesystem, so
blindness is a mount namespace, not an instruction.

Usage:
  runner.py --model <openrouter-id> --image <docker-image> --trial <id>
            [--agent-env CLAUDECODE=1] [--max-steps 40] [--out <dir>]
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
# Operator-supplied, never stored in this repo. OPENROUTER_API_KEY wins; the
# opencode credential file is a convenience fallback for a machine that has one.
AUTH_FALLBACK = pathlib.Path.home() / ".local/share/opencode/auth.json"

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
    if AUTH_FALLBACK.exists():
        k = json.loads(AUTH_FALLBACK.read_text()).get("openrouter", {}).get("key", "")
        if k:
            return k
    sys.exit("no OpenRouter key: set OPENROUTER_API_KEY")


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
    ap.add_argument("--model", required=True)
    ap.add_argument("--image", required=True)
    ap.add_argument("--trial", required=True)
    ap.add_argument("--user", default="root")
    ap.add_argument("--agent-env", default="", help="VAR=VALUE set in the container, "
                                                    "so the CLI detects that agent")
    ap.add_argument("--max-steps", type=int, default=40)
    ap.add_argument("--out", default=".")
    a = ap.parse_args()

    outdir = pathlib.Path(a.out) / a.trial
    outdir.mkdir(parents=True, exist_ok=True)
    log = (outdir / "transcript.jsonl").open("w")

    def rec(kind, **kw):
        log.write(json.dumps({"t": time.time(), "kind": kind, **kw}) + "\n")
        log.flush()

    container = f"dogfood-{a.trial}"
    subprocess.run(["docker", "rm", "-f", container],
                   capture_output=True)
    run = ["docker", "run", "-d", "--name", container]
    if a.agent_env:
        run += ["-e", a.agent_env]
    run += [a.image, "sleep", "infinity"]
    cid = subprocess.run(run, capture_output=True, text=True)
    if cid.returncode != 0:
        print(f"container start failed: {cid.stderr}", file=sys.stderr)
        return 1
    rec("start", trial=a.trial, model=a.model, image=a.image, user=a.user,
        agent_env=a.agent_env, container=container)

    api_key = key()
    messages = [{"role": "system", "content": SYSTEM},
                {"role": "user", "content": USER}]
    rec("user", content=USER)

    usage_total = {"prompt_tokens": 0, "completion_tokens": 0, "cost": 0.0}
    steps = 0
    stop = "max-steps"
    final = ""

    while steps < a.max_steps:
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
