#!/usr/bin/env python3
"""Run ONE runner.py trial offline: no Docker, no OpenRouter, no money.

    fake_trial.py <runner.py> <outdir> <capture.json> [brief]

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

The fake response carries NO tool_calls, so the loop stops at `finished` after
one turn and the container shell is never reached.
"""
import importlib.util
import json
import os
import subprocess
import sys
import types
import urllib.request

runner_path, outdir, capture_path = sys.argv[1], sys.argv[2], sys.argv[3]
brief = sys.argv[4] if len(sys.argv) > 4 else ""

spec = importlib.util.spec_from_file_location("dogfood_runner", runner_path)
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)

captured = {}


def fake_run(cmd, *a, **kw):
    """Stands in for every Docker call. Records nothing but the fact of it."""
    captured.setdefault("subprocess", []).append(list(cmd))
    return subprocess.CompletedProcess(cmd, 0, stdout="fake\n", stderr="")


class _Resp:
    def __init__(self, body):
        self._body = body

    def read(self):
        return self._body

    def __enter__(self):
        return self

    def __exit__(self, *a):
        return False


def fake_urlopen(req, *a, **kw):
    captured["url"] = req.full_url
    captured["payload"] = json.loads(req.data.decode())
    return _Resp(json.dumps({
        "choices": [{"message": {"content": "done", "tool_calls": []}}],
        "usage": {"prompt_tokens": 11, "completion_tokens": 3, "cost": 0.0001},
    }).encode())


runner.subprocess = types.SimpleNamespace(
    run=fake_run,
    CompletedProcess=subprocess.CompletedProcess,
    TimeoutExpired=subprocess.TimeoutExpired,
)
urllib.request.urlopen = fake_urlopen

os.environ["OPENROUTER_API_KEY"] = "sk-or-fake-not-a-real-key"
argv = ["runner.py", "--model", "fake/model", "--image", "fake-image",
        "--trial", "faketrial", "--out", outdir]
if brief:
    argv += ["--brief", brief]
sys.argv = argv

rc = runner.main()
captured["rc"] = rc
with open(capture_path, "w") as f:
    json.dump(captured, f)
sys.exit(0 if rc == 0 else 1)
