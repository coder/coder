#!/usr/bin/env python3
"""Validates workloads that run inside a workspace daemon.

It expects what README.md sets up: coderd with the runtime-workloads
experiment and a running workspace from the template in this directory.

Settings come from the environment:

  CODER_URL            coderd, as reachable from here (required)
  CODER_SESSION_TOKEN  API token of the workspace owner (required)
  POD_CODER_URL        coderd's access URL, as pods reach it. Defaults to
                       CODER_URL.
  CODER_BIN            the coder CLI built from this branch. Defaults to coder.
  KUBE_CONTEXT         kubeconfig context. Defaults to the current context.
  NAMESPACE            defaults to coder-hosted-workloads
  WORKSPACE            workspace from this template. Defaults to dev.
  OTHER_WORKSPACE      optional: owner/name of a running workspace from this
                       template that another user owns and the token's user
                       can see. Enables the checks that need a second
                       workspace daemon.
  RUNS_DIR             where results go. Defaults to ./runs.

Workloads are probed from the inside: the script runs shell scripts as
workloads and reads what they print. It also plays an attacker outside the
sandbox, sending requests straight to coderd.

The script writes a transcript and results.json to RUNS_DIR/<timestamp>/ and
exits non-zero if any check fails. No credential is written to the output.
It stops and starts the workspace, so run it against a throwaway setup.
"""

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
import uuid

CODER = os.environ.get("CODER_BIN", "coder")
NAMESPACE = os.environ.get("NAMESPACE", "coder-hosted-workloads")
KUBECTL = ["kubectl"] + (["--context", os.environ["KUBE_CONTEXT"]] if os.environ.get("KUBE_CONTEXT") else [])
API = os.environ["CODER_URL"].rstrip("/")
POD_API = os.environ.get("POD_CODER_URL", API).rstrip("/")
WORKSPACE = os.environ.get("WORKSPACE", "dev")
OTHER_WORKSPACE = os.environ.get("OTHER_WORKSPACE", "")
RUNS_DIR = pathlib.Path(os.environ.get("RUNS_DIR", "runs"))
IDENTITY_HEADER = "Coder-Workload-Identity"
READ_SCOPES = ["workspace:read", "template:read"]
# A workload's output stream carries stdout and stderr, so the CLI's notices
# are dropped to leave only the JSON.
IDENTITY_COMMAND = ["sh", "-c", "coder identity --output json 2>/dev/null"]


class Report:
    def __init__(self):
        stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
        self.dir = RUNS_DIR / stamp
        self.dir.mkdir(parents=True)
        self.transcript = open(self.dir / "transcript.txt", "w")
        self.results = []

    def write(self, text=""):
        print(text, flush=True)
        self.transcript.write(text + "\n")
        self.transcript.flush()

    def section(self, title):
        self.write()
        self.write(f"== {title}")

    def info(self, text):
        self.write(f"       {text}")

    def check(self, check_id, title, ok, observed=""):
        self.results.append({"id": check_id, "title": title, "pass": bool(ok), "observed": str(observed)})
        self.write(f"[{'PASS' if ok else 'FAIL'}] {check_id} {title}")
        if observed:
            for line in str(observed).splitlines():
                self.info(line)
        return ok

    def finish(self):
        failed = [r for r in self.results if not r["pass"]]
        summary = {"checks": len(self.results), "passed": len(self.results) - len(failed), "failed": len(failed)}
        (self.dir / "results.json").write_text(json.dumps({"summary": summary, "checks": self.results}, indent=2) + "\n")
        self.section("Summary")
        self.write(f"{summary['passed']} of {summary['checks']} checks passed. Evidence: {self.dir}")
        for r in failed:
            self.write(f"  failed: {r['id']} {r['title']}")
        return not failed


def run(cmd, timeout=300, stdin=None):
    return subprocess.run(cmd, capture_output=True, text=True, timeout=timeout, input=stdin)


def coder(*args, timeout=300):
    return run([CODER, *args], timeout=timeout)


def kubectl(*args, timeout=120, namespaced=True):
    return run(KUBECTL + (["-n", NAMESPACE] if namespaced else []) + list(args), timeout=timeout)


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
        with urllib.request.urlopen(req, data=data, timeout=30) as res:
            status, raw = res.status, res.read()
    except urllib.error.HTTPError as err:
        status, raw = err.code, err.read()
    try:
        return status, json.loads(raw)
    except ValueError:
        return status, raw.decode(errors="replace")


def vouch(path, token, identity):
    """Sends what a workspace daemon sends when it forwards a workload's request."""
    status, _ = http("GET", path, token=token, headers={IDENTITY_HEADER: identity} if identity else None)
    return status


def workspace(name):
    out = coder("list", "--all", "--output", "json")
    owner, _, short = name.rpartition("/")
    for ws in json.loads(out.stdout):
        if ws["name"] == short and (not owner or ws["owner_name"] == owner):
            return ws
    raise RuntimeError(f"workspace {name} not found: {out.stderr}")


def agents(ws):
    return [a for r in ws["latest_build"]["resources"] for a in (r.get("agents") or [])]


def workloads():
    out = coder("run", "--list", WORKSPACE, "--output", "json")
    return json.loads(out.stdout or "[]")


def wait_until(predicate, timeout=300, interval=2):
    deadline = time.time() + timeout
    while time.time() < deadline:
        value = predicate()
        if value:
            return value
        time.sleep(interval)
    return None


def workspace_pod(ws):
    out = kubectl("get", "pods", "-l", f"coder.com/workspace-id={ws['id']}", "-o", "json")
    items = [p for p in json.loads(out.stdout)["items"] if not p["metadata"].get("deletionTimestamp")]
    if not items:
        # The label is on the Deployment; its pods carry the instance label.
        out = kubectl("get", "pods", "-l", f"app.kubernetes.io/instance=coder-{ws['id']}", "-o", "json")
        items = [p for p in json.loads(out.stdout)["items"] if not p["metadata"].get("deletionTimestamp")]
    return items[0] if items else None


def in_pod(pod_name, script):
    return kubectl("exec", pod_name, "--", "sh", "-c", script)


def daemon_token(pod_name):
    """Reads the workspace daemon's agent token, as root in the workspace can."""
    return in_pod(pod_name, 'printf %s "$CODER_AGENT_TOKEN"').stdout.strip()


def sandbox_containers(pod_name):
    out = in_pod(pod_name, "docker ps --quiet --filter label=com.coder.workload")
    return out.stdout.split()


def run_workload(command, name=None, scopes=(), detach=False, target=None, timeout=300):
    args = ["run", target or WORKSPACE]
    if name:
        args += ["--name", name]
    for scope in scopes:
        args += ["--scope", scope]
    if detach:
        args.append("--detach")
    return coder(*args, "--", *command, timeout=timeout)


def probe(script, name=None, scopes=()):
    """Runs a shell script as a workload. Returns its output split by `@@ marker` lines."""
    out = run_workload(["sh", "-c", script], name=name, scopes=scopes)
    sections, current = {}, None
    for line in out.stdout.splitlines():
        if line.startswith("@@ "):
            current = line[3:].strip()
            sections[current] = []
        elif current is not None:
            sections[current].append(line)
    return {key: "\n".join(lines).strip() for key, lines in sections.items()}, out


def api_json(text):
    """Decodes what `coder exp api` printed: a JSON value, or an error message."""
    for line in text.splitlines():
        line = line.strip()
        if line[:1] in "[{":
            try:
                return json.loads(line)
            except ValueError:
                pass
    return {"message": text}


def ids(value):
    if isinstance(value, dict) and "workspaces" in value:
        value = value["workspaces"]
    if isinstance(value, list):
        return sorted(item["id"] for item in value)
    return value.get("message") if isinstance(value, dict) else value


def remove_all_workloads():
    for workload in workloads():
        coder("run", "--remove", workload["name"], WORKSPACE)


def main():
    report = Report()
    token = os.environ["CODER_SESSION_TOKEN"]
    _, me = http("GET", "/api/v2/users/me", token=token)
    user = me["username"]

    report.section("Preconditions")
    status, _ = http("GET", "/healthz")
    report.check("P1", "coderd is up", status == 200, f"GET /healthz -> {status}")
    status, experiments = http("GET", "/api/v2/experiments", token=token)
    report.check("P2", "the runtime-workloads experiment is enabled", "runtime-workloads" in experiments, experiments)
    remove_all_workloads()
    dev = workspace(WORKSPACE)
    pod = workspace_pod(dev)
    pod_name = pod["metadata"]["name"]
    daemons = agents(dev)
    report.check(
        "P3", "the workspace has one daemon, connected, in one pod",
        len(daemons) == 1 and daemons[0]["status"] == "connected" and dev["health"]["healthy"],
        f"agents={[(a['name'], a['status']) for a in daemons]} pod={pod_name}",
    )
    daemon = daemons[0]
    build_before = dev["latest_build"]["id"]

    # ------------------------------------------------------------------
    report.section("A. The workspace daemon and the human")
    out = in_pod(pod_name, f"CODER_AGENT_URL={POD_API} $(ls -d /tmp/coder.*/coder | head -1) identity --output json")
    human = json.loads(out.stdout)
    report.check(
        "A1", "the workspace daemon has its own identity, resting on its agent token",
        human["kind"] == "daemon" and human["attestation"] == "agent-token"
        and human["id"] == f"{dev['id']}/agent/{daemon['id']}" and human["owner_username"] == user,
        f"id={human['id']} kind={human['kind']} attestation={human['attestation']}",
    )
    out = coder("whoami")
    report.check(
        "A2", "`coder whoami` on the human's own machine shows the user and no workload identity",
        f"authenticated as {user}" in out.stdout and "Workload identity" not in out.stdout, out.stdout.strip(),
    )
    out = coder("identity")
    report.check(
        "A3", "`coder identity` outside a workspace says there is no workload identity",
        out.returncode != 0 and "no workload identity" in out.stderr, out.stderr.strip().splitlines()[-1:],
    )

    # ------------------------------------------------------------------
    report.section("B. Running workloads inside the daemon")
    started = time.time()
    out = run_workload(IDENTITY_COMMAND, name="first")
    elapsed = time.time() - started
    try:
        first = json.loads(out.stdout)
    except ValueError:
        first = {}
    report.check(
        "B1", "`coder run dev -- coder identity` prints the workload's own identity",
        out.returncode == 0 and first.get("kind") == "workload" and first.get("attestation") == "workspace-daemon"
        and first.get("id") == f"{dev['id']}/workload/{first.get('identity_id')}"
        and first.get("sponsor_username") == user and first.get("scopes", []) == []
        and first.get("agent_id") == daemon["id"] and first.get("sandbox", {}).get("runtime") == "docker",
        f"id={first.get('id')}\nsponsor={first.get('sponsor_username')} scopes={first.get('scopes', [])} "
        f"sandbox={first.get('sandbox', {}).get('runtime')} took {elapsed:.1f}s" if first else out.stderr[-400:],
    )
    out = run_workload(["sh", "-c", "echo to-stdout; echo to-stderr >&2; exit 3"])
    report.check(
        "B2", "the command's output is streamed and its exit code returned",
        out.returncode == 3 and "to-stdout" in out.stdout and "to-stderr" in out.stdout,
        f"exit code {out.returncode}; output {out.stdout.split()}",
    )
    timings = []
    for _ in range(5):
        started = time.time()
        run_workload(["true"])
        timings.append(time.time() - started)
    median = statistics.median(timings)
    report.check(
        "B3", "a workload starts, runs and returns in seconds",
        median < 10, f"`coder run {WORKSPACE} -- true`, five runs: median {median:.2f}s, range {min(timings):.2f}s to {max(timings):.2f}s",
    )

    out_alpha = run_workload(["sleep", "900"], name="alpha", detach=True)
    # beta listens on a port, so that C5 can tell blocked from absent.
    out_beta = run_workload(["nc", "-lk", "-p", "8080", "-e", "echo", "beta-says-hi"], name="beta", scopes=READ_SCOPES, detach=True)
    listed = {w["name"]: w for w in workloads()}
    alpha, beta = listed.get("alpha"), listed.get("beta")
    report.check(
        "B4", "two detached workloads run side by side, each with its own identity and sandbox",
        out_alpha.returncode == 0 and out_beta.returncode == 0 and alpha and beta
        and len({alpha["id"], beta["id"], first.get("identity_id")}) == 3
        and alpha["sandbox"]["id"] != beta["sandbox"]["id"] and len(sandbox_containers(pod_name)) == 2
        and alpha["scopes"] == [] and beta["scopes"] == READ_SCOPES,
        f"alpha={alpha['identity'] if alpha else None}\nbeta={beta['identity'] if beta else None}\n"
        f"containers in the workspace's Docker: {len(sandbox_containers(pod_name))}",
    )
    now = workspace(WORKSPACE)
    pods = json.loads(kubectl("get", "pods", "-o", "json").stdout)["items"]
    same_pod = workspace_pod(now)["metadata"]["uid"] == pod["metadata"]["uid"]
    report.check(
        "B5", "nothing was provisioned: no workspace build, no new pod, no new agent",
        now["latest_build"]["id"] == build_before and same_pod and len(agents(now)) == 1
        and not any("workload" in p["metadata"]["name"] for p in pods),
        f"latest build unchanged; pods in the namespace: {len(pods)}; agents in the workspace: {len(agents(now))}",
    )

    # ------------------------------------------------------------------
    report.section("C. Isolation and masquerade attempts")
    token_of_daemon = daemon_token(pod_name)
    beta_ip = in_pod(pod_name, f"docker inspect -f '{{{{.NetworkSettings.IPAddress}}}}' {beta['sandbox']['id']}").stdout.strip()
    gateway = in_pod(pod_name, "docker network inspect bridge -f '{{(index .IPAM.Config 0).Gateway}}'").stdout.strip()
    reachable_from_host = in_pod(pod_name, f"nc -w 3 {beta_ip} 8080 </dev/null").stdout.strip()

    sections, out = probe(f"""
echo "@@ identity"
coder exp api workspaceagents/me/identity 2>/dev/null
echo "@@ claims-beta"
coder exp api workspaceagents/me/identity "{IDENTITY_HEADER}:{beta['id']}" "Coder-Session-Token:forged" \\
  "Authorization:Bearer forged" "Cookie:coder_session_token=forged" "coder_session_token==forged" \\
  "access_token==forged" "identity=={beta['id']}" 2>/dev/null
echo "@@ env"
env | grep -c -E 'CODER_AGENT_TOKEN|SESSION_TOKEN'
env | grep '^CODER' | sort
echo "@@ user"
id
grep CapEff /proc/self/status
echo "@@ files"
ls /run/coder
touch /run/coder/x 2>&1
ls /var/run/docker.sock /tmp/coder-workloads /tmp/coder.* 2>&1 | sed 's/^ls: //'
echo "@@ processes"
ls /proc | grep -c -E '^[0-9]+$'
tr '\\0' ' ' < /proc/1/cmdline | cut -c1-12
echo "@@ network"
nc -w 3 {beta_ip} 8080 </dev/null >/dev/null 2>&1 && echo beta-reached || echo beta-blocked
nc -w 3 {gateway} 2375 </dev/null >/dev/null 2>&1 && echo docker-api-reached || echo docker-api-unreachable
nc -w 3 {gateway} 2376 </dev/null >/dev/null 2>&1 && echo docker-tls-reached || echo docker-tls-unreachable
echo "@@ daemon-api"
coder exp api workspaceagents/me/gitsshkey 2>/dev/null
coder exp api workspaceagents/me/external-auth match==github.com 2>/dev/null
echo "@@ direct"
wget -q -O- --header "{IDENTITY_HEADER}: {beta['id']}" {POD_API}/api/v2/workspaces 2>&1 | tail -1
wget -q -O- --header "{IDENTITY_HEADER}: {beta['id']}" {POD_API}/api/v2/workspaceagents/me/identity 2>&1 | tail -1
echo "@@ view"
coder exp api workspaces 2>/dev/null
echo "@@ end"
""", name="mallory")
    mallory = api_json(sections.get("identity", ""))
    claimed = api_json(sections.get("claims-beta", ""))
    report.check(
        "C1", "a workload that claims to be beta in headers, cookie, bearer token and query is still itself",
        mallory.get("name") == "mallory" and claimed.get("identity_id") == mallory.get("identity_id")
        and claimed.get("identity_id") not in (None, beta["id"]),
        f"mallory is {mallory.get('identity_id')}; claiming beta ({beta['id']}) it is {claimed.get('identity_id')}",
    )
    env = sections.get("env", "").splitlines()
    report.check(
        "C2", "the sandbox holds no credential: no agent token, no session token, only the path of its channel",
        env[:1] == ["0"] and [line.split("=")[0] for line in env[1:]] == ["CODER_WORKLOAD_NAME", "CODER_WORKLOAD_SOCKET"],
        env,
    )
    report.check(
        "C3", "it runs unprivileged, sees its own channel read-only, and nothing of the workspace or of other workloads",
        "uid=1000" in sections.get("user", "") and "0000000000000000" in sections.get("user", "")
        and sections.get("files", "").splitlines()[0] == "workload.sock" and "Read-only file system" in sections.get("files", "")
        and sections.get("files", "").count("No such file or directory") == 3,
        sections.get("user", "").splitlines() + sections.get("files", "").splitlines(),
    )
    processes = sections.get("processes", "").splitlines()
    report.check(
        "C4", "it sees only its own processes, not the daemon's or Docker's",
        len(processes) >= 2 and int(processes[0]) <= 6 and processes[1].startswith("sh -c"),
        f"{processes[0] if processes else '?'} processes visible; PID 1 is `{processes[1] if len(processes) > 1 else '?'}...`",
    )
    report.check(
        "C5", "it cannot reach another workload's sandbox or the Docker API over the network",
        sections.get("network", "").split() == ["beta-blocked", "docker-api-unreachable", "docker-tls-unreachable"]
        and reachable_from_host == "beta-says-hi",
        f"{sections.get('network', '').split()}; the same port answers `{reachable_from_host}` from the workspace itself",
    )
    daemon_api = [api_json(line).get("message") for line in sections.get("daemon-api", "").splitlines() if line.strip()]
    report.check(
        "C6", "it cannot use the daemon's own API, although its requests reach coderd with the daemon's token",
        len(daemon_api) == 2 and all("cannot use its workspace daemon" in (m or "") for m in daemon_api), daemon_api,
    )
    direct = sections.get("direct", "").splitlines()
    report.check(
        "C7", "going around the daemon, straight to coderd, with beta's identity in a header gets it nothing",
        len(direct) == 2 and all("401" in line for line in direct), direct,
    )
    view = api_json(sections.get("view", ""))
    report.check(
        "C8", "scopes are not inherited: beta may read workspaces, and a workload beside it without scopes sees none",
        isinstance(view, dict) and view.get("count") == 0 and view.get("workspaces") == [] and "end" in sections,
        f"workspaces visible to mallory: {view.get('count') if isinstance(view, dict) else view}",
    )

    other_token = None
    if OTHER_WORKSPACE:
        other = workspace(OTHER_WORKSPACE)
        other_token = daemon_token(workspace_pod(other)["metadata"]["name"])
    attempts = {
        "no credential": vouch("/api/v2/workspaces", None, beta["id"]),
        "a random agent token": vouch("/api/v2/workspaces", str(uuid.uuid4()), beta["id"]),
        "the daemon's token for an identity that does not exist": vouch("/api/v2/workspaces", token_of_daemon, str(uuid.uuid4())),
        "the daemon's token without an identity": vouch("/api/v2/workspaces", token_of_daemon, None),
    }
    if other_token:
        attempts["another workspace's daemon vouching for beta"] = vouch("/api/v2/workspaces", other_token, beta["id"])
    report.check(
        "C9", "from outside, nothing but beta's own daemon can act as beta",
        set(attempts.values()) == {401}, attempts,
    )
    status, got = http("GET", "/api/v2/users/me", token=token, headers={IDENTITY_HEADER: beta["id"]})
    status2 = vouch("/api/v2/workspaceagents/me/identity", token, beta["id"])
    report.check(
        "C10", "a user who sends the identity header stays that user and does not become the workload",
        status == 200 and got.get("username") == user and status2 == 401,
        f"users/me -> {status} {got.get('username') if isinstance(got, dict) else ''}; identity endpoint -> {status2}",
    )
    status, got = http("GET", "/api/v2/workspaceagents/me/identity", token=token_of_daemon, headers={IDENTITY_HEADER: beta["id"]})
    report.check(
        "C11", "by design: the workspace daemon, and so root in the workspace, can act as the workloads it runs",
        status == 200 and got.get("identity_id") == beta["id"],
        f"{status} identity_id={got.get('identity_id') if isinstance(got, dict) else got} (sent from this host with the daemon's token)",
    )

    # ------------------------------------------------------------------
    report.section("D. Permissions")
    sponsor = {path: ids(http("GET", path, token=token)[1]) for path in ("/api/v2/workspaces", "/api/v2/templates")}
    report.info(f"the sponsor sees {len(sponsor['/api/v2/workspaces'])} workspaces and {len(sponsor['/api/v2/templates'])} templates")
    view_script = f"""
echo "@@ me"; coder exp api users/me 2>/dev/null
echo "@@ workspaces"; coder exp api workspaces 2>/dev/null
echo "@@ templates"; coder exp api templates 2>/dev/null
echo "@@ own"; coder exp api workspaces/{dev['id']} 2>/dev/null
echo "@@ organizations"; coder exp api organizations 2>/dev/null
echo "@@ end"
"""
    none, _ = probe(view_script, name="perm-none")
    seen = {key: ids(api_json(none.get(key, ""))) for key in ("me", "workspaces", "templates", "own", "organizations")}
    not_found = "Resource not found or you do not have access to this resource"
    report.check(
        "D1", "a workload without scopes can see nothing in Coder",
        seen == {"me": not_found, "workspaces": [], "templates": [], "own": not_found, "organizations": []}, seen,
    )
    templates_only, _ = probe(view_script, name="perm-templates", scopes=["template:read"])
    seen = (ids(api_json(templates_only.get("templates", ""))), ids(api_json(templates_only.get("workspaces", ""))))
    report.check(
        "D2", "with template:read it lists its sponsor's templates, and still no workspaces",
        seen == (sponsor["/api/v2/templates"], []) and sponsor["/api/v2/templates"],
        f"templates={len(seen[0]) if isinstance(seen[0], list) else seen[0]} workspaces={seen[1]}",
    )
    reader, _ = probe(view_script, name="perm-reader", scopes=READ_SCOPES)
    seen = (ids(api_json(reader.get("workspaces", ""))), ids(api_json(reader.get("templates", ""))))
    report.check(
        "D3", "with workspace:read and template:read it lists exactly the workspaces its sponsor can see",
        seen == (sponsor["/api/v2/workspaces"], sponsor["/api/v2/templates"]) and dev["id"] in seen[0],
        f"workspaces={len(seen[0]) if isinstance(seen[0], list) else seen[0]} (sponsor sees {len(sponsor['/api/v2/workspaces'])})",
    )
    out = run_workload(["coder", "list"], scopes=READ_SCOPES)
    report.check(
        "D4", "`coder list` works inside such a workload without a login",
        out.returncode == 0 and WORKSPACE in out.stdout, [line.strip()[:70] for line in out.stdout.splitlines() if WORKSPACE in line][:2],
    )
    writer, _ = probe(f"""
echo "@@ autostart"; coder exp api PUT workspaces/{dev['id']}/autostart schedule= >/dev/null 2>&1; echo $?
echo "@@ run"; coder exp api POST workspaces/{dev['id']}/workloads command=true >/dev/null 2>&1; echo $?
echo "@@ remove"; coder exp api DELETE workspaces/{dev['id']}/workloads/{alpha['id']} >/dev/null 2>&1; echo $?
echo "@@ keys"; coder exp api users/me/keys >/dev/null 2>&1; echo $?
echo "@@ end"
""", name="perm-writer", scopes=READ_SCOPES + ["workspace:update"])
    still_there = {w["name"] for w in workloads()}
    report.check(
        "D5", "with workspace:update it can change the workspace, but no workload can run or remove workloads",
        writer.get("autostart") == "0" and all(writer.get(key) not in (None, "0") for key in ("run", "remove", "keys"))
        and "end" in writer and "alpha" in still_there and len(still_there) == 2,
        "exit codes of the API calls: " + ", ".join(
            f"{label}={writer.get(key)}" for key, label in (
                ("autostart", "change autostart"), ("run", "run a workload"),
                ("remove", "remove a workload"), ("keys", "read the sponsor's API keys"))),
    )
    out = run_workload(["true"], scopes=["workspace:fly"])
    report.check("D6", "an unknown scope is refused", out.returncode != 0 and "not a scope" in out.stderr,
                 out.stderr.strip().splitlines()[-2:-1])
    if OTHER_WORKSPACE:
        out = run_workload(["true"], target=OTHER_WORKSPACE)
        report.check(
            "D7", "nobody but the owner can run a workload in a workspace, not even an administrator",
            out.returncode != 0 and "Only the owner" in out.stderr, out.stderr.strip().splitlines()[-1:],
        )
    else:
        report.info("D7 skipped: set OTHER_WORKSPACE to a running workspace from this template that another user owns")

    # ------------------------------------------------------------------
    report.section("E. Revocation and lifecycle")
    out = coder("run", "--remove", "alpha", WORKSPACE)
    status = vouch("/api/v2/workspaceagents/me/identity", token_of_daemon, alpha["id"])
    gone = wait_until(lambda: alpha["sandbox"]["id"][:12] not in " ".join(sandbox_containers(pod_name)), timeout=60)
    report.check(
        "E1", "removing a workload revokes its identity at once, even for its own daemon, and stops its sandbox",
        out.returncode == 0 and status == 401 and bool(gone), f"daemon vouching for alpha -> {status}; sandbox removed={bool(gone)}",
    )
    out = run_workload(["sh", "-c", "sleep 8"], name="shortlived", detach=True)
    short = next(w for w in workloads() if w["name"] == "shortlived")
    before = vouch("/api/v2/workspaceagents/me/identity", token_of_daemon, short["id"])
    dropped = wait_until(lambda: not any(w["name"] == "shortlived" for w in workloads()), timeout=60)
    after = vouch("/api/v2/workspaceagents/me/identity", token_of_daemon, short["id"])
    report.check(
        "E2", "an identity lasts as long as its command: once the command exits, it is revoked",
        before == 200 and bool(dropped) and after == 401, f"while running -> {before}; after it exited -> {after}",
    )
    in_pod(pod_name, f"docker rm --force {beta['sandbox']['id']} >/dev/null")
    dropped = wait_until(lambda: not any(w["name"] == "beta" for w in workloads()), timeout=60)
    status = vouch("/api/v2/workspaceagents/me/identity", token_of_daemon, beta["id"])
    report.check(
        "E3", "killing a sandbox behind Coder's back ends the workload and its identity",
        bool(dropped) and status == 401, f"beta listed={not dropped}; daemon vouching for beta -> {status}",
    )

    run_workload(["sleep", "900"], name="survivor", detach=True)
    survivor = next(w for w in workloads() if w["name"] == "survivor")
    out = coder("stop", WORKSPACE, "--yes", timeout=600)
    pod_gone = wait_until(lambda: workspace_pod(workspace(WORKSPACE)) is None, timeout=300)
    status = vouch("/api/v2/workspaceagents/me/identity", token_of_daemon, survivor["id"])
    listed = workloads()
    report.check(
        "E4", "stopping the workspace ends every workload: the pod, the sandboxes and the identities go together",
        out.returncode == 0 and bool(pod_gone) and status == 401 and listed == [],
        f"pod gone={bool(pod_gone)}; the old daemon vouching for survivor -> {status}; workloads listed={len(listed)}",
    )
    out = coder("start", WORKSPACE, "--yes", timeout=600)
    restarted = wait_until(
        lambda: (lambda ws: ws if [a["status"] for a in agents(ws)] == ["connected"] else None)(workspace(WORKSPACE)),
        timeout=420,
    )
    fresh = {}
    if restarted:
        for _ in range(12):
            again = run_workload(IDENTITY_COMMAND)
            if again.returncode == 0:
                fresh = json.loads(again.stdout)
                break
            # Docker inside the new pod may still be starting.
            time.sleep(5)
    new_token = daemon_token(workspace_pod(workspace(WORKSPACE))["metadata"]["name"]) if restarted else ""
    status = vouch("/api/v2/workspaceagents/me/identity", new_token, survivor["id"])
    report.check(
        "E5", "starting it again brings back the daemon and no workloads; new workloads get new identities",
        out.returncode == 0 and restarted is not None and fresh.get("kind") == "workload"
        and fresh.get("identity_id") not in {None, first.get("identity_id"), survivor["id"]} and status == 401,
        f"new identity {fresh.get('id')}; the new daemon vouching for the old survivor -> {status}",
    )

    # ------------------------------------------------------------------
    report.section("F. Footprint")
    kinds = {}
    for kind in ("deployments", "pods", "secrets", "serviceaccounts", "roles", "rolebindings"):
        items = json.loads(kubectl("get", kind, "-o", "json").stdout)["items"]
        kinds[kind] = sorted(i["metadata"]["name"] for i in items)
    report.check(
        "F1", "the namespace holds only workspaces: no operator, no extra service account, role or secret",
        all(name.startswith("coder-") for name in kinds["deployments"] + kinds["pods"])
        and kinds["secrets"] == [] and kinds["serviceaccounts"] == ["default"]
        and kinds["roles"] == [] and kinds["rolebindings"] == [],
        {kind: len(names) for kind, names in kinds.items()},
    )
    bindings = json.loads(kubectl("get", "clusterrolebindings", "-o", "json", namespaced=False).stdout)["items"]
    mine = [b["metadata"]["name"] for b in bindings if any(s.get("namespace") == NAMESPACE for s in (b.get("subjects") or []))]
    report.check("F2", "nothing in the namespace is bound to a cluster-wide role", mine == [], mine)

    ok = report.finish()
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
