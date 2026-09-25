#!/usr/bin/env python3
"""Run ONE runner.py trial offline: no Docker, no OpenRouter, no money.

    fake_trial.py <runner.py> <outdir> <capture.json> [brief] [-- <extra runner args>]

Imports runner.py as a module, replaces the two things that touch the outside
world — `subprocess.run` (Docker) and `urllib.request.urlopen` (OpenRouter) —
and drives `main()` to completion. The request body the runner WOULD have sent
is written to <capture.json>, and the real transcript lands under <outdir>.

🔴 THIS EXISTS SO THE BRIEF CAN BE TESTED WHERE IT MATTERS, NOT WHERE IT IS
CHEAP. Asserting that runner.py contains the substring `--brief` is a claim
about the file; asserting that the JSON leaving for the model carries the
brief, and that the transcript records it, is a claim about the trial. The
model-facing message is the entire task a blind agent receives, so a change
that quietly stops delivering it would otherwise be invisible until a matrix
came back mysteriously all-red.

🔴 AND SO THE CREDENTIAL PATH CAN BE PROVED, NOT ASSERTED. The capture records
EVERY `subprocess.run` argv the runner issued — which is the process-argv
surface a token must never reach — so the leak test can grep it with the same
pattern it greps the transcript and the command log with. `FAKE_TOOL_OUTPUT`
makes the stub Docker echo an arbitrary string back as a command's output, so
a trial in which the model `cat`s the credential file can be reproduced
offline and the redactor watched to catch it.

🔴 AND SO A TRUNCATED TURN CAN BE REPLAYED. `FAKE_FINAL_RESPONSE` serves a
whole recorded OpenRouter response body as the final turn, which is how the
real `ab-genpost-glm-01` truncation — a turn with `content: null`, no tool
calls and `completion_tokens` equal to the budget — is driven through the
classifier from bytes rather than from a hand-built dict.

Env knobs:
  FAKE_TOOL_COMMAND   a command to have the fake model "run" (default: none,
                      so the loop stops after one turn with no tool calls)
  FAKE_TOOL_OUTPUT    what the stub Docker returns as that command's output
  FAKE_TRIAL_ID       the trial id (default `faketrial`)
  FAKE_FINISH_REASON  finish_reason on the FINAL turn (default `stop`; the
                      literal `__absent__` omits the field entirely, which is
                      what a provider that does not send it looks like)
  FAKE_FINAL_CONTENT  content on the final turn (default `done`; set it to the
                      empty string for an empty reply, or to `__null__` for
                      the JSON null the real truncation carried)
  FAKE_REASONING      a `reasoning` string returned on every turn
  FAKE_REASONING_DETAILS  a JSON array returned as `reasoning_details`
  FAKE_REASONING_TOKENS   per-turn usage.completion_tokens_details.reasoning_tokens
  FAKE_USAGE_COST     what `usage.cost` is on EVERY turn (default 0.0001).
                      `__absent__` omits the key; `__null__` sets it to JSON
                      null; `__nousage__` omits the whole `usage` object; any
                      other value is used verbatim, as a float when it parses
                      as one and as a STRING when it does not — which is how a
                      provider sending `"cost": "0.004"` is reproduced. These
                      are the shapes that made --max-cost inoperable: a turn
                      the harness cannot price was counted as $0.
  FAKE_FINAL_RESPONSE path to a JSON file holding a COMPLETE response body to
                      return verbatim as the final turn. Overrides every knob
                      above for that turn.

The fake response carries no tool_calls once FAKE_TOOL_COMMAND has been served
once, so the loop always terminates.
"""
import importlib.util
import json
import os
import subprocess
import sys
import types
import urllib.request

runner_path, outdir, capture_path = sys.argv[1], sys.argv[2], sys.argv[3]
rest = sys.argv[4:]
extra = []
if "--" in rest:
    i = rest.index("--")
    extra = rest[i + 1:]
    rest = rest[:i]
brief = rest[0] if rest else ""

spec = importlib.util.spec_from_file_location("dogfood_runner", runner_path)
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)

captured = {}
# One command per assistant turn, newline-separated, in order. A cap that only
# fires on the Nth invocation cannot be tested with a single turn.
TOOL_COMMANDS = [c for c in os.environ.get("FAKE_TOOL_COMMAND", "").split("\n") if c]
TOOL_OUTPUT = os.environ.get("FAKE_TOOL_OUTPUT", "")

# 🔴 THE DEFAULT IS A WELL-BEHAVED PROVIDER, AND IT IS STATED RATHER THAN
# IMPLIED. Every test written before finish_reason existed runs through this
# path, so the default has to be the one that means "the model chose to stop"
# — otherwise those tests would start asserting a classification they were
# never about.
FINISH_REASON = os.environ.get("FAKE_FINISH_REASON", "stop")
FINAL_CONTENT = os.environ.get("FAKE_FINAL_CONTENT", "done")
REASONING = os.environ.get("FAKE_REASONING", "")
REASONING_DETAILS = os.environ.get("FAKE_REASONING_DETAILS", "")
REASONING_TOKENS = int(os.environ.get("FAKE_REASONING_TOKENS", "0") or 0)
FINAL_RESPONSE = os.environ.get("FAKE_FINAL_RESPONSE", "")
USAGE_COST = os.environ.get("FAKE_USAGE_COST", "")


def _usage() -> dict:
    """The `usage` block for one turn, or None for a response carrying none.

    🔴 THE DEFAULT IS A PROVIDER THAT PRICES ITS TURNS, STATED RATHER THAN
    IMPLIED — every test written before --max-cost could be defeated runs
    through this path, so the default has to stay the well-behaved shape.
    """
    usage = {"prompt_tokens": 11, "completion_tokens": 3, "cost": 0.0001}
    if REASONING_TOKENS:
        usage["completion_tokens_details"] = {"reasoning_tokens": REASONING_TOKENS}
    if not USAGE_COST:
        return usage
    if USAGE_COST == "__nousage__":
        return None
    if USAGE_COST == "__absent__":
        usage.pop("cost")
        return usage
    if USAGE_COST == "__null__":
        usage["cost"] = None
        return usage
    try:
        usage["cost"] = float(USAGE_COST)
    except ValueError:
        usage["cost"] = USAGE_COST
    return usage


def fake_run(cmd, *a, **kw):
    """Stands in for every Docker call, and RECORDS ITS FULL ARGV.

    The argv list is the leak surface `ps` would expose on a real run, so the
    test greps exactly this.
    """
    cmd = list(cmd)
    captured.setdefault("subprocess", []).append(cmd)
    out = "fake\n"
    # `docker exec … bash -lc <command>` is how sh() runs the model's command;
    # hand back the planted output so the redactor has something to catch.
    if TOOL_OUTPUT and "exec" in cmd and "bash" in cmd and "-lc" in cmd:
        out = TOOL_OUTPUT + "\n"
    # runner.py reads bytes from sh() and str everywhere it passes text=True.
    if kw.get("text"):
        return subprocess.CompletedProcess(cmd, 0, stdout=out, stderr="")
    return subprocess.CompletedProcess(cmd, 0, stdout=out.encode(), stderr=b"")


class _Resp:
    def __init__(self, body):
        self._body = body

    def read(self):
        return self._body

    def __enter__(self):
        return self

    def __exit__(self, *a):
        return False


_served = {"n": 0}


def _reasoning_fields() -> dict:
    out = {}
    if REASONING:
        out["reasoning"] = REASONING
    if REASONING_DETAILS:
        out["reasoning_details"] = json.loads(REASONING_DETAILS)
    return out


def fake_urlopen(req, *a, **kw):
    captured["url"] = req.full_url
    captured.setdefault("payloads", []).append(json.loads(req.data.decode()))
    captured["payload"] = captured["payloads"][0]
    usage = _usage()

    def body(choice):
        # `usage` is OMITTED, not sent as null, when there is none — a provider
        # that returns no usage object at all is a different wire shape from
        # one that returns `"usage": null`, and the runner must refuse both.
        out = {"choices": [choice]}
        if usage is not None:
            out["usage"] = usage
        return _Resp(json.dumps(out).encode())

    if _served["n"] < len(TOOL_COMMANDS):
        cmd = TOOL_COMMANDS[_served["n"]]
        _served["n"] += 1
        msg = {"content": None, "tool_calls": [{
            "id": "call_%d" % _served["n"], "type": "function",
            "function": {"name": "bash",
                         "arguments": json.dumps({"command": cmd})},
        }], **_reasoning_fields()}
        choice = {"message": msg, "finish_reason": "tool_calls"}
        return body(choice)
    # The terminal turn. A recorded body wins outright: replaying real bytes is
    # the only version of this that cannot quietly disagree with the artifact.
    if FINAL_RESPONSE:
        with open(FINAL_RESPONSE, "rb") as f:
            return _Resp(f.read())
    content = FINAL_CONTENT
    if content == "__null__":
        content = None
    msg = {"content": content, "tool_calls": [], **_reasoning_fields()}
    choice = {"message": msg}
    if FINISH_REASON != "__absent__":
        choice["finish_reason"] = FINISH_REASON
    return body(choice)


runner.subprocess = types.SimpleNamespace(
    run=fake_run,
    CompletedProcess=subprocess.CompletedProcess,
    TimeoutExpired=subprocess.TimeoutExpired,
)
urllib.request.urlopen = fake_urlopen

os.environ["OPENROUTER_API_KEY"] = "sk-or-fake-not-a-real-key"
trial = os.environ.get("FAKE_TRIAL_ID", "faketrial")
argv = ["runner.py", "--model", "fake/model", "--image", "fake-image",
        "--trial", trial, "--out", outdir]
if brief:
    argv += ["--brief", brief]
argv += extra
sys.argv = argv

rc = runner.main()
captured["rc"] = rc
with open(capture_path, "w") as f:
    json.dump(captured, f)
sys.exit(0 if rc == 0 else 1)
