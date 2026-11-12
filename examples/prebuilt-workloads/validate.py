#!/usr/bin/env python3
"""Checks a running setup of prebuilt workspaces for workloads.

A workload gets a workspace to itself, claimed from the workspaces its
template keeps ready. The workload's identity is an account of its own that
owns the workspace and shares it with the sponsor.

The script needs an administrator's and a member's command line, kubectl
for the cluster, and nothing else:

    CODER_URL=https://coder.example.com \\
    CODER_SESSION_TOKEN=<administrator's token> CODER_BIN=coder \\
    SPONSOR_TOKEN=<member's token> SPONSOR_CODER_BIN=<member's coder> \\
    python3 validate.py

It writes what it saw to RUNS_DIR and exits non-zero when a check failed.
"""

import concurrent.futures
import datetime
import json
import os
import pathlib
import statistics
import subprocess
import sys
import time
import urllib.error
import urllib.request

API = os.environ.get("CODER_URL", "").rstrip("/")
ADMIN_TOKEN = os.environ.get("CODER_SESSION_TOKEN", "")
ADMIN = os.environ.get("CODER_BIN", "coder")
SPONSOR_TOKEN = os.environ.get("SPONSOR_TOKEN", "")
SPONSOR = os.environ.get("SPONSOR_CODER_BIN", "")
TEMPLATE = os.environ.get("TEMPLATE", "prebuilt")
PRESET = os.environ.get("PRESET", "agent")
NAMESPACE = os.environ.get("NAMESPACE", "coder-sandbox-workloads")
KUBE_CONTEXT = os.environ.get("KUBE_CONTEXT", "")
RUNS_DIR = pathlib.Path(os.environ.get("RUNS_DIR", "runs"))
IDENTITY_HEADER = "Coder-Workload-Identity"
TOKEN_PATH = "/var/run/secrets/coder.com/identity/token"
RUN_ID = datetime.datetime.now(datetime.timezone.utc).strftime("%H%M%S")


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


def kubectl(*args):
    argv = ["kubectl"]
    if KUBE_CONTEXT:
        argv += ["--context", KUBE_CONTEXT]
    return run([*argv, "-n", NAMESPACE, *args])


def http(method, path, token=None, body=None, headers=None):
    """Calls coderd directly. Returns (status, decoded JSON or text)."""
    req = urllib.request.Request(API + path, method=method)
    if token:
        req.add_header("Coder-Session-Token", token)
    for key, value in (headers or {}).items():
        req.add_header(key, value)
    data = None
    if body is not None:
        data = json.dumps(body).encode()
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, data=data, timeout=60) as res:
            status, raw = res.status, res.read()
    except urllib.error.HTTPError as err:
        status, raw = err.code, err.read()
    try:
        return status, json.loads(raw)
    except ValueError:
        return status, raw.decode(errors="replace")


def wait_until(predicate, timeout=180, interval=2):
    deadline = time.time() + timeout
    while time.time() < deadline:
        value = predicate()
        if value:
            return value
        time.sleep(interval)
    return None


def launch(command, name=None, scopes=(), detach=False, keep=False, binary=None, template=None, preset=None, timeout=300):
    """Runs a workload in a workspace of its own, as the sponsor."""
    args = ["run", "--template", template or TEMPLATE]
    if preset is None:
        preset = PRESET if (template or TEMPLATE) == TEMPLATE else ""
    if preset:
        args += ["--preset", preset]
    if name:
        args += ["--name", name]
    for scope in scopes:
        args += ["--scope", scope]
    if detach:
        args.append("--detach")
    if keep:
        args.append("--keep")
    began = time.time()
    result = run([binary or SPONSOR, *args, "--", *command], timeout=timeout)
    return result, time.time() - began


def inside(script, **kwargs):
    """Runs a shell script as a workload and returns its result."""
    result, _ = launch(["sh", "-c", script], **kwargs)
    return result


def listings(result):
    """What a workload's listings returned: name -> list, or None when refused."""
    found = {}
    for line in result.stdout.splitlines():
        name, _, raw = line.partition("=")
        try:
            found[name] = json.loads(raw)
        except ValueError:
            found[name] = None
    return found


LIST = "echo \"{0}=$(coder {1} --output json 2>/dev/null | tr -d '\\n')\""
LIST_ALL = "; ".join([LIST.format("templates", "templates list"), LIST.format("workspaces", "list --all"), LIST.format("users", "users list")])


def workloads(token=None, finished=True):
    status, body = http("GET", f"/api/v2/workloads?finished={'true' if finished else 'false'}", token or SPONSOR_TOKEN)
    return body if status == 200 else []


def workload_named(name, token=None):
    return next((w for w in workloads(token) if w["name"] == name), None)


def workspace(workspace_id):
    status, body = http("GET", f"/api/v2/workspaces/{workspace_id}?include_deleted=true", ADMIN_TOKEN)
    return body if status == 200 else None


def pool():
    """The names of the workspaces that are ready and wait for an owner."""
    status, body = http("GET", f"/api/v2/workspaces?q=owner:prebuilds+template:{TEMPLATE}", ADMIN_TOKEN)
    if status != 200:
        return []
    ready = []
    for ws in body["workspaces"]:
        build = ws["latest_build"]
        agents = [a for res in build["resources"] for a in (res.get("agents") or [])]
        if build["status"] == "running" and agents and all(a["status"] == "connected" and a["lifecycle_state"] == "ready" for a in agents):
            ready.append(ws)
    return ready


def pool_size():
    status, presets = http("GET", f"/api/v2/templateversions/{template()['active_version_id']}/presets", ADMIN_TOKEN)
    return next((p.get("DesiredPrebuildInstances") or 0 for p in presets if p["Name"] == PRESET), 0) if status == 200 else 0


def template():
    status, body = http("GET", f"/api/v2/templates?q=exact_name:{TEMPLATE}", ADMIN_TOKEN)
    return body[0] if status == 200 and body else None


def settle(size):
    return wait_until(lambda: len(pool()) >= size, timeout=300)


def pod_of(workspace_id):
    return f"coder-{workspace_id}"


def pod_token(pod_name):
    return kubectl("exec", pod_name, "--", "cat", TOKEN_PATH).stdout.strip()


def exchange(token):
    """Presents a pod token to coderd, as the daemon in the pod does."""
    return http("POST", "/api/v2/workspaceagents/kubernetes-pod-identity", body={"token": token, "wait_seconds": 0})


def daemon_session(pod_name):
    status, body = exchange(pod_token(pod_name))
    return body.get("session_token", "") if status == 200 and isinstance(body, dict) else ""


def main():
    report = Report()
    size = 0

    report.section("Preflight")
    status, _ = http("GET", "/healthz")
    report.check("P1", "coderd is up", status == 200, f"GET /healthz -> {status}")
    _, experiments = http("GET", "/api/v2/experiments", ADMIN_TOKEN)
    report.check("P2", "the runtime-workloads experiment is enabled", "runtime-workloads" in experiments, experiments)
    size = pool_size() if template() else 0
    report.check("P3", "the template keeps workspaces ready", size > 0, f"template {TEMPLATE}, preset {PRESET}: {size} kept ready")
    _, me = http("GET", "/api/v2/users/me", SPONSOR_TOKEN)
    sponsor_name = me.get("username", "") if isinstance(me, dict) else ""
    sponsor_roles = [r["name"] for r in me.get("roles", [])] if isinstance(me, dict) else []
    report.check("P4", "the sponsor is an ordinary member, not an administrator", sponsor_name != "" and "owner" not in sponsor_roles,
                 f"sponsor {sponsor_name}, site roles {sponsor_roles}")
    if not all(r["ok"] for r in report.results):
        return report.finish()
    report.check("P5", "the pool is full", settle(size), f"{len(pool())} of {size} workspaces ready and waiting")

    report.section("A. Speed")
    times = []
    for _ in range(5):
        settle(size)
        result, took = launch(["true"])
        if result.returncode == 0:
            times.append(took)
    report.check(
        "A1", "a workload starts in a workspace of its own, runs and returns in a few seconds",
        len(times) == 5 and statistics.median(times) < 8,
        f"`coder run --template {TEMPLATE} -- true`, five runs with a full pool: median {statistics.median(times):.2f}s, "
        f"range {min(times):.2f}s to {max(times):.2f}s" if times else "no run succeeded",
    )
    settle(size)
    result, _ = launch(["sh", "-c", "echo out; echo err >&2; exit 7"])
    report.check("A2", "its output and its exit code come back", result.returncode == 7 and "out" in result.stdout,
                 f"exit code {result.returncode}, stdout {result.stdout.strip()!r}")

    report.section("B. Who owns the workspace, and who the workload is")
    settle(size)
    waiting = {pod_of(ws["id"]): daemon_session(pod_of(ws["id"])) for ws in pool()}
    asked = datetime.datetime.now(datetime.timezone.utc)
    name_b = f"own-{RUN_ID}"
    result, _ = launch(["sh", "-c", "coder identity --output json 2>/dev/null; sleep 600"], name=name_b, detach=True, keep=True,
                       scopes=["workspace:read", "template:read"])
    live = wait_until(lambda: workload_named(name_b), timeout=60)
    ws = workspace(live["workspace_id"]) if live else None
    _, owner = http("GET", f"/api/v2/users/{ws['owner_id']}", ADMIN_TOKEN) if ws else (0, {})
    report.check(
        "B1", "the workspace belongs to the workload's identity, an account that cannot sign in, and not to the sponsor",
        ws is not None and owner.get("is_service_account") is True and owner.get("login_type") == "none"
        and ws["owner_name"] != sponsor_name and live.get("user_id") == ws["owner_id"],
        f"workspace {ws['owner_name']}/{ws['name']}; owner is_service_account={owner.get('is_service_account')}, "
        f"login_type={owner.get('login_type')}; sponsor {live['sponsor_username']}" if ws else result.stderr.strip(),
    )
    pod = json.loads(kubectl("get", "pod", pod_of(ws["id"]), "-o", "json").stdout or "{}") if ws else {}
    created = pod.get("metadata", {}).get("creationTimestamp", "")
    report.check(
        "B2", "its pod was running before it was asked for: the workspace was claimed, not built",
        created != "" and datetime.datetime.fromisoformat(created.replace("Z", "+00:00")) < asked and ws["latest_build"]["build_number"] == 2,
        f"pod created {created}, workload asked for {asked.isoformat(timespec='seconds')}, build number {ws['latest_build']['build_number']}" if ws else "",
    )
    session = daemon_session(pod_of(ws["id"])) if ws else ""
    _, who = http("GET", "/api/v2/workspaceagents/me/identity", session, headers={IDENTITY_HEADER: live["id"]}) if ws else (0, {})
    report.check(
        "B3", "the workload is itself: sponsored by the member, vouched for by the cluster as that pod",
        isinstance(who, dict) and who.get("kind") == "workload" and who.get("sponsor_username") == sponsor_name
        and who.get("attestation_subject") == "k8s-pod:" + pod.get("metadata", {}).get("uid", "?")
        and who.get("owner_username") == ws["owner_name"],
        f"kind {who.get('kind')}, sponsor {who.get('sponsor_username')}, owner {who.get('owner_username')}, "
        f"attested {who.get('attestation_subject')}, scopes {who.get('scopes')}" if isinstance(who, dict) else who,
    )

    report.section("C. Nothing of the sponsor's")
    settle(size)
    bare = listings(inside(LIST_ALL))
    report.check("C1", "without scopes the workload sees nothing in Coder: no template, no workspace, no user",
                 set(bare) == {"templates", "workspaces", "users"} and not any(bare.values()),
                 {name: (len(items) if items is not None else "refused") for name, items in bare.items()})
    settle(size)
    scoped = listings(inside(LIST_ALL, scopes=["template:read"]))
    report.check("C2", "with a scope it can do that and no more",
                 TEMPLATE in [t.get("Template", t).get("name") for t in scoped.get("templates") or []]
                 and not scoped.get("workspaces") and not scoped.get("users"),
                 {name: (len(items) if items is not None else "refused") for name, items in scoped.items()})
    _, sponsor_key = http("GET", "/api/v2/users/me/gitsshkey", SPONSOR_TOKEN)
    _, daemon_key = http("GET", "/api/v2/workspaceagents/me/gitsshkey", session)
    _, identity_key = http("GET", f"/api/v2/users/{ws['owner_id']}/gitsshkey", ADMIN_TOKEN) if ws else (0, {})
    report.check(
        "C3", "the Git key its daemon is given is the identity's own, not the sponsor's",
        isinstance(daemon_key, dict) and daemon_key.get("public_key") and daemon_key.get("public_key") == identity_key.get("public_key")
        and daemon_key.get("public_key") != sponsor_key.get("public_key"),
        f"daemon's key {str(daemon_key.get('public_key'))[:40] if isinstance(daemon_key, dict) else daemon_key}..., "
        f"sponsor's key {str(sponsor_key.get('public_key'))[:40]}...",
    )
    status, tokens = http("GET", f"/api/v2/users/{ws['owner_id']}/keys/tokens", ADMIN_TOKEN) if ws else (0, None)
    report.check("C4", "the identity has no API token", status == 200 and not tokens, f"GET /users/<identity>/keys/tokens -> {status} {tokens or []}")
    human = f"human-{RUN_ID}"
    run([ADMIN, "create", human, "--template", TEMPLATE, "--preset", PRESET, "--yes"])
    settle(size)
    listed = inside("coder list --all --output json 2>/dev/null", scopes=["workspace:read", "template:read"])
    try:
        names = [f"{w['owner_name']}/{w['name']}" for w in json.loads(listed.stdout)]
    except ValueError:
        names = None
    report.check(
        "C5", "allowed to read workspaces, it sees its own and none of anyone else's",
        names is not None and len(names) == 1 and names[0].startswith("workload-"),
        f"`coder list --all` inside the workload: {names}",
    )

    report.section("D. Nobody is somebody else")
    status, _ = exchange("eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6eDp5In0.forged")
    report.check("D1", "a token the cluster did not issue is refused", status not in (200, 404), f"forged token -> {status}")
    spare = pool()[0] if pool() else None
    spare_session = daemon_session(pod_of(spare["id"])) if spare else ""
    _, spare_who = http("GET", "/api/v2/workspaceagents/me/identity", spare_session)
    status_as, _ = http("GET", "/api/v2/workspaceagents/me/identity", spare_session, headers={IDENTITY_HEADER: live["id"] if live else ""})
    report.check(
        "D2", "a pod that waits in the pool is the pool's: it is no workload and cannot say it is one",
        isinstance(spare_who, dict) and spare_who.get("owner_username") == "prebuilds" and spare_who.get("kind") == "daemon" and status_as == 401,
        f"waiting pod: kind {spare_who.get('kind')}, owner {spare_who.get('owner_username')}; naming a live workload -> {status_as}"
        if isinstance(spare_who, dict) else spare_who,
    )
    before = waiting.get(pod_of(ws["id"]), "") if ws else ""
    status_before, _ = http("GET", "/api/v2/workspaceagents/me/identity", before)
    report.check("D3", "what a pod held while it waited is worth nothing once it is claimed",
                 before != "" and status_before == 401, f"the session the pod had in the pool -> {status_before}")
    settle(size)
    name_d = f"other-{RUN_ID}"
    launch(["sleep", "600"], name=name_d, detach=True, keep=True)
    other = wait_until(lambda: workload_named(name_d), timeout=60)
    other_session = daemon_session(pod_of(other["workspace_id"])) if other else ""
    status_me, _ = http("GET", "/api/v2/workspaceagents/me/identity", other_session, headers={IDENTITY_HEADER: other["id"] if other else ""})
    status_x, _ = http("GET", "/api/v2/workspaceagents/me/identity", other_session, headers={IDENTITY_HEADER: live["id"] if live else ""})
    status_y, _ = http("GET", f"/api/v2/workspaces/{ws['id'] if ws else ''}", other_session, headers={IDENTITY_HEADER: live["id"] if live else ""})
    report.check("D4", "one workload cannot act as another",
                 status_me == 200 and status_x == 401 and status_y == 401,
                 f"as itself -> {status_me}; naming the other workload -> {status_x}; reading the other's workspace as it -> {status_y}")
    settle(size)
    nested = inside(f"coder run --template {TEMPLATE} -- true; echo rc=$?", scopes=["workspace:read", "template:read"])
    report.check("D5", "a workload cannot sponsor a workload", "rc=0" not in nested.stdout and "rc=" in nested.stdout,
                 " ".join((nested.stdout + nested.stderr).split())[-160:])

    report.section("E. What the sponsor can do")
    status_s, shared = http("GET", f"/api/v2/workspaces/{ws['id']}", SPONSOR_TOKEN) if ws else (0, {})
    _, acl = http("GET", f"/api/v2/workspaces/{ws['id']}/acl", ADMIN_TOKEN) if ws else (0, {})
    roles = {u["username"]: u["role"] for u in acl.get("users", [])} if isinstance(acl, dict) else {}
    report.check("E1", "the identity shares its workspace with its sponsor, to use and not to own",
                 status_s == 200 and roles == {sponsor_name: "use"}, f"sponsor reads it -> {status_s}; shared with {roles}")
    entered = run([SPONSOR, "ssh", f"{ws['owner_name']}/{ws['name']}", "--", "echo", "entered"], timeout=120) if ws else None
    report.check("E2", "the sponsor can enter it", entered is not None and "entered" in entered.stdout,
                 " ".join(entered.stdout.split()) if entered else "")
    status_h, _ = http("GET", f"/api/v2/users/{sponsor_name}/workspace/{human}", SPONSOR_TOKEN)
    foreign = run([SPONSOR, "run", "--remove", name_b, human])
    report.check("E3", "the sponsor has no way into workspaces that are not shared with them", status_h == 404 and foreign.returncode != 0,
                 f"an administrator's workspace -> {status_h}")
    removed = run([SPONSOR, "run", "--remove", name_d])
    gone = wait_until(lambda: (workspace(other["workspace_id"]) or {}).get("latest_build", {}).get("transition") == "delete", timeout=120) if other else None
    report.check("E4", "the sponsor can end the workload, and its workspace goes with it",
                 removed.returncode == 0 and bool(gone), (removed.stderr or removed.stdout).strip()[-120:])

    report.section("F. Afterwards")
    settle(size)
    name_f = f"brief-{RUN_ID}"
    launch(["sh", "-c", "echo done; exit 3"], name=name_f)
    brief = workload_named(name_f)
    over = wait_until(lambda: (workspace(brief["workspace_id"]) or {}).get("latest_build", {}).get("status") == "deleted", timeout=180) if brief else None
    account = (workspace(brief["workspace_id"]) or {}).get("owner_name", "?") if brief else "?"
    retired = wait_until(lambda: http("GET", f"/api/v2/users?q={account}", ADMIN_TOKEN)[1].get("users") == [], timeout=30)
    pod_left = kubectl("get", "pod", pod_of(brief["workspace_id"]), "--ignore-not-found", "-o", "name").stdout.strip() if brief else "?"
    report.check("F1", "when the command is over its workspace, its pod and its identity are gone",
                 bool(over) and bool(retired) and pod_left == "",
                 f"workspace deleted={bool(over)}; account {account} still listed={not retired}; pod left: {pod_left or 'none'}")
    report.check("F2", "what ran stays on record for the sponsor, with how it ended",
                 brief is not None and brief["status"] == "exited" and brief.get("exit_code") == 3,
                 f"{brief['name']}: {brief['status']}, exit code {brief.get('exit_code')}, sponsor {brief['sponsor_username']}" if brief else "")
    _, audit = http("GET", "/api/v2/audit?limit=200&q=resource_type%3Aworkload_identity", ADMIN_TOKEN)
    mine = [e for e in audit.get("audit_logs", []) if (e.get("additional_fields") or {}).get("workload_identity_id") == (brief or {}).get("id")]
    _, builds = http("GET", "/api/v2/audit?limit=200&q=resource_type%3Aworkspace_build", ADMIN_TOKEN)
    built = sorted({e["action"] for e in builds.get("audit_logs", [])
                    if (e.get("additional_fields") or {}).get("workspace_id") == (brief or {}).get("workspace_id")})
    report.check("F3", "the audit log has the workload's start and end and its workspace's build and removal",
                 sorted(e["action"] for e in mine) == ["start", "stop"] and "delete" in built,
                 f"workload entries {sorted(e['action'] for e in mine)} by {sorted({e['user']['username'] for e in mine if e.get('user')})}; "
                 f"workspace builds {built}")

    report.section("G. What was there before still works")
    _, human_ws = http("GET", f"/api/v2/users/me/workspace/{human}", ADMIN_TOKEN)
    report.check("G1", "a person still claims a prebuilt workspace and owns it",
                 isinstance(human_ws, dict) and human_ws.get("owner_name") != "" and not human_ws.get("owner_name", "").startswith("workload-")
                 and human_ws["latest_build"]["build_number"] == 2,
                 f"{human_ws.get('owner_name')}/{human_ws.get('name')}, build number {human_ws.get('latest_build', {}).get('build_number')}"
                 if isinstance(human_ws, dict) else human_ws)

    report.section("H. More at once than are ready")
    settle(size)
    with concurrent.futures.ThreadPoolExecutor(max_workers=size + 2) as executor:
        burst = list(executor.map(lambda _: launch(["true"], timeout=600), range(size + 2)))
    took = sorted(round(t, 1) for _, t in burst)
    report.check("H1", "more workloads than the pool holds all run: the rest get workspaces that are built for them",
                 all(r.returncode == 0 for r, _ in burst), f"{size + 2} at once on a pool of {size}: they returned after {took} seconds")

    # Leave nothing behind.
    run([SPONSOR, "run", "--remove", name_b])
    run([ADMIN, "delete", human, "--yes"])
    return report.finish()


if __name__ == "__main__":
    if not (API and ADMIN_TOKEN and SPONSOR_TOKEN and SPONSOR):
        print("set CODER_URL, CODER_SESSION_TOKEN, SPONSOR_TOKEN and SPONSOR_CODER_BIN", file=sys.stderr)
        sys.exit(2)
    sys.exit(0 if main() else 1)
