#!/usr/bin/env python3
"""Checks the workspace relay on a running deployment that enforces it.

It creates a workspace whose template hands the owner's session token to
what runs in it, and then uses that token and the daemon's own token the way
somebody would who got hold of them.

The script needs an administrator's command line, Docker for the workspace's
container, and openssl:

    CODER_URL=http://127.0.0.1:3590 CODER_SESSION_TOKEN=<administrator's token> \\
    CODER_BIN=./scripts/coder-dev.sh METRICS_URL=http://127.0.0.1:2290 \\
    python3 validate.py

It writes what it saw to RUNS_DIR and exits non-zero when a check failed.
"""

import base64
import datetime
import json
import os
import pathlib
import re
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

API = os.environ.get("CODER_URL", "").rstrip("/")
ADMIN_TOKEN = os.environ.get("CODER_SESSION_TOKEN", "")
ADMIN = os.environ.get("CODER_BIN", "coder")
METRICS = os.environ.get("METRICS_URL", "").rstrip("/")
TEMPLATE = os.environ.get("TEMPLATE", "relay-demo")
RUNS_DIR = pathlib.Path(os.environ.get("RUNS_DIR", "runs"))
RUN_ID = datetime.datetime.now(datetime.timezone.utc).strftime("%H%M%S")
LEASE_HEADER = "Coder-Workspace-Relay-Lease"


class Report:
    def __init__(self):
        self.results, self.lines = [], []

    def say(self, line=""):
        print(line, flush=True)
        self.lines.append(line)

    def section(self, title):
        self.say(f"\n== {title}")

    def check(self, check_id, title, ok, evidence=""):
        self.results.append({"id": check_id, "title": title, "ok": bool(ok), "evidence": str(evidence)})
        self.say(f"[{'PASS' if ok else 'FAIL'}] {check_id} {title}")
        if evidence != "":
            self.say(f"       {evidence}")

    def finish(self):
        failed = [r for r in self.results if not r["ok"]]
        run_dir = RUNS_DIR / datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
        run_dir.mkdir(parents=True, exist_ok=True)
        self.section("Summary")
        self.say(f"{len(self.results) - len(failed)} of {len(self.results)} checks passed. Evidence: {run_dir}")
        for r in failed:
            self.say(f"  failed: {r['id']} {r['title']}")
        (run_dir / "results.json").write_text(json.dumps(self.results, indent=2) + "\n")
        (run_dir / "transcript.txt").write_text("\n".join(self.lines) + "\n")
        return not failed


def run(argv, timeout=300):
    return subprocess.run(argv, capture_output=True, text=True, timeout=timeout, check=False)


def api(token, method, path, body=None, headers=None, base=None):
    """Returns the status and the decoded body of an API request."""
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request((base or API) + path, data=data, method=method, headers={
        "Coder-Session-Token": token,
        "Content-Type": "application/json",
        **(headers or {}),
    })
    try:
        with urllib.request.urlopen(req, timeout=60) as res:
            raw = res.read()
            return res.status, (json.loads(raw) if raw else None)
    except urllib.error.HTTPError as err:
        raw = err.read()
        try:
            return err.code, json.loads(raw)
        except ValueError:
            return err.code, raw.decode(errors="replace")


def metric(name, **labels):
    """Reads one counter of the control plane."""
    if not METRICS:
        return None
    with urllib.request.urlopen(METRICS + "/metrics", timeout=30) as res:
        text = res.read().decode()
    total = 0.0
    for line in text.splitlines():
        if not line.startswith(name + "{"):
            continue
        if all(f'{k}="{v}"' in line for k, v in labels.items()):
            total += float(line.rsplit(" ", 1)[1])
    return total


def inside(workspace, script, timeout=120):
    """Runs a shell script in the workspace, as a process the daemon starts."""
    return run([ADMIN, "ssh", workspace, "--", script], timeout=timeout)


def wait_for(what, timeout=180, interval=2.0):
    deadline = time.time() + timeout
    while time.time() < deadline:
        got = what()
        if got:
            return got
        time.sleep(interval)
    return None


class DaemonKey:
    """An Ed25519 key, as an instance of a workspace daemon makes at start."""

    def __init__(self):
        self.dir = tempfile.mkdtemp(prefix="relay-key-")
        self.pem = os.path.join(self.dir, "key.pem")
        run(["openssl", "genpkey", "-algorithm", "ed25519", "-out", self.pem])
        der = subprocess.run(["openssl", "pkey", "-in", self.pem, "-pubout", "-outform", "DER"],
                             capture_output=True, check=True).stdout
        self.public = der[-32:]

    def lease_request(self):
        nanos = time.time_ns()
        stamp = datetime.datetime.fromtimestamp(nanos // 10**9, datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%S")
        message = os.path.join(self.dir, "message")
        pathlib.Path(message).write_bytes(f"coder-workspace-relay-lease\n{nanos}".encode())
        signature = subprocess.run(["openssl", "pkeyutl", "-sign", "-inkey", self.pem, "-rawin", "-in", message],
                                   capture_output=True, check=True).stdout
        return {
            "public_key": base64.b64encode(self.public).decode(),
            "timestamp": f"{stamp}.{nanos % 10**9:09d}Z",
            "signature": base64.b64encode(signature).decode(),
        }


def main():
    if not (API and ADMIN_TOKEN):
        print(__doc__)
        return 2
    report = Report()
    say, check = report.say, report.check
    name = f"relay-{RUN_ID}"
    container = None
    say(f"deployment {API}, template {TEMPLATE}, workspace {name}")

    made = run([ADMIN, "create", name, "--template", TEMPLATE, "-y"], timeout=600)
    if made.returncode != 0:
        say(made.stdout[-600:] + made.stderr[-600:])
        return 1
    _, me = api(ADMIN_TOKEN, "GET", "/api/v2/users/me")
    container = f"coder-{me['username']}-{name}"

    def environment():
        got = inside(name, "printenv CODER_URL; printenv CODER_SESSION_TOKEN; printenv CODER_AGENT_TOKEN; printenv CODER_AGENT_URL")
        lines = got.stdout.strip().splitlines()
        return lines if got.returncode == 0 and len(lines) == 4 else None

    env = wait_for(environment)
    if not env:
        say("the workspace did not come up")
        return 1
    relay_url, session_token, daemon_token, control_plane = env
    control_plane = control_plane.rstrip("/")

    report.section("Inside the workspace")
    check("A1", "processes are given the daemon's local address as CODER_URL, whatever the template set",
          re.fullmatch(r"http://127\.0\.0\.1:\d+", relay_url) is not None and relay_url != control_plane,
          f"CODER_URL={relay_url}, the control plane is {control_plane}")
    who = inside(name, "coder whoami")
    check("A2", "the command line works unchanged, through the daemon",
          who.returncode == 0 and me["username"] in who.stdout, " ".join(who.stdout.split())[-150:] if who.stdout.strip() else who.stderr[-200:])
    code = 'curl -s -o /dev/null -w "%{http_code}" -H "Coder-Session-Token: $CODER_SESSION_TOKEN"'
    through = inside(name, f'{code} "$CODER_URL/api/v2/users/me"').stdout.strip()
    direct = inside(name, f'{code} "$CODER_AGENT_URL/api/v2/users/me"').stdout.strip()
    check("A3", "the session token works through the daemon and not against the control plane directly, even from inside",
          (through, direct) == ("200", "401"), f"through the daemon {through}, directly {direct}")
    stream = inside(name, f'{code} "$CODER_URL/api/v2/workspaces?limit=1"').stdout.strip()
    check("A4", "other API calls and their responses pass through", stream == "200", stream)

    report.section("A session token that left the workspace")
    refused_before = metric("coderd_workspace_relay_requests_total", route="bare", outcome="refused")
    s, body = api(session_token, "GET", "/api/v2/users/me")
    check("B1", "the token is refused anywhere else, and the answer says why",
          s == 401 and "workspace's daemon" in json.dumps(body), f"{s} {body.get('message') if isinstance(body, dict) else body}")
    s, _ = api(session_token, "GET", "/api/v2/users/me", headers={LEASE_HEADER: "made-up"})
    check("B2", "a made-up lease does not help", s == 401, s)
    s, _ = api(ADMIN_TOKEN, "GET", "/api/v2/users/me")
    check("B3", "a person's own session is as it was", s == 200, s)
    refused_after = metric("coderd_workspace_relay_requests_total", route="bare", outcome="refused")
    check("B4", "the control plane counts what it refused",
          METRICS == "" or (refused_before is not None and refused_after >= refused_before + 1),
          f"bare and refused: {refused_before} -> {refused_after}" if METRICS else "no METRICS_URL given")

    report.section("A daemon token that left the workspace")
    intruder = DaemonKey()
    s, body = api("", "POST", "/api/v2/workspaceagents/me/relay-lease", intruder.lease_request(),
                  headers={"Coder-Session-Token": daemon_token})
    check("C1", "the daemon's token alone obtains no lease while the daemon runs",
          s == 409, f"{s} {body.get('message') if isinstance(body, dict) else body}")

    report.section("The daemon stops and starts again")
    def relay_works():
        got = inside(name, f'{code} "$CODER_URL/api/v2/users/me"', timeout=30)
        return got.returncode == 0 and got.stdout.strip() == "200"

    claimed_before = metric("coderd_workspace_relay_leases_total", result="claimed")
    run(["docker", "restart", container], timeout=120)
    started = time.time()
    back = wait_for(relay_works, timeout=180)
    graceful = time.time() - started
    claimed_after = metric("coderd_workspace_relay_leases_total", result="claimed")
    check("D1", "a daemon that was stopped gives the relay up, and the next one does not wait for the lease to lapse",
          back and graceful < 45 and (not METRICS or claimed_after >= claimed_before + 1),
          f"working again {graceful:.0f}s after the restart; claimed {claimed_before} -> {claimed_after}")

    takeover_before = metric("coderd_workspace_relay_leases_total", result="takeover")
    run(["docker", "kill", "-s", "KILL", container], timeout=60)
    run(["docker", "start", container], timeout=60)
    started = time.time()
    # While the new daemon waits for the old lease to lapse, what it starts
    # must already be given its address: a process keeps the address it was
    # started with.
    early = inside(name, f'printenv CODER_URL; {code} "$CODER_URL/api/v2/users/me"', timeout=60).stdout.strip().splitlines()
    early_at = time.time() - started
    back = wait_for(relay_works, timeout=240)
    crash = time.time() - started
    takeover_after = metric("coderd_workspace_relay_leases_total", result="takeover")
    check("D2", "a daemon that was killed is replaced once its lease has lapsed, and the control plane records the takeover",
          back and (not METRICS or takeover_after >= takeover_before + 1),
          f"working again {crash:.0f}s after the kill; takeover {takeover_before} -> {takeover_after}")
    check("D3", "while the new daemon waits, processes are already given its address, and are refused until it has a lease",
          len(early) == 2 and re.fullmatch(r"http://127\.0\.0\.1:\d+", early[0]) is not None and early[1] == "401",
          f"{early_at:.0f}s after the kill: {early}")

    run([ADMIN, "delete", name, "-y"], timeout=600)
    return 0 if report.finish() else 1


if __name__ == "__main__":
    sys.exit(main())
