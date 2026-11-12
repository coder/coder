#!/usr/bin/env python3
"""Checks a deployment that gives every workload a workspace of its own.

It expects what README.md sets up: coderd with the runtime-workloads
experiment and a cluster to claim sandboxes in, the sandbox template, and a
warm pool. It runs workloads, looks at their pods from the cluster's side,
and plays a pod that tries to pass for another.

Settings, from the environment:

  CODER_URL             coderd, as this machine reaches it
  CODER_SESSION_TOKEN   an administrator's token; the administrator sponsors
                        the workloads of this run
  CODER_BIN             the coder CLI (default: coder)
  TEMPLATE              the sandbox template (default: sandbox)
  NAMESPACE             where sandboxes are claimed (default: coder-sandbox-workloads)
  WARM_POOL             the pool's name (default: coder-workload)
  KUBE_CONTEXT          kubeconfig context for kubectl (default: the current one)
  OTHER_CODER_BIN       optional: a CLI signed in as another, ordinary user
  RUNS_DIR              where to write evidence (default: ./runs)
"""

import base64
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
TOKEN = os.environ.get("CODER_SESSION_TOKEN", "")
CODER = os.environ.get("CODER_BIN", "coder")
OTHER_CODER = os.environ.get("OTHER_CODER_BIN", "")
TEMPLATE = os.environ.get("TEMPLATE", "sandbox")
NAMESPACE = os.environ.get("NAMESPACE", "coder-sandbox-workloads")
WARM_POOL = os.environ.get("WARM_POOL", "coder-workload")
KUBE_CONTEXT = os.environ.get("KUBE_CONTEXT", "")
RUNS_DIR = pathlib.Path(os.environ.get("RUNS_DIR", "runs"))
IDENTITY_HEADER = "Coder-Workload-Identity"
TOKEN_PATH = "/var/run/secrets/coder.com/identity/token"
# The CLI's notices go to stderr, which is part of a workload's output.
IDENTITY_COMMAND = ["sh", "-c", "coder identity --output json 2>/dev/null"]


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

    def info(self, line):
        self.say(f"[info] {line}")

    def finish(self):
        failed = [r for r in self.results if not r["ok"]]
        run = RUNS_DIR / datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
        run.mkdir(parents=True, exist_ok=True)
        self.section("Summary")
        self.say(f"{len(self.results) - len(failed)} of {len(self.results)} checks passed. Evidence: {run}")
        for r in failed:
            self.say(f"  failed: {r['id']} {r['title']}")
        (run / "results.json").write_text(json.dumps(self.results, indent=2) + "\n")
        (run / "transcript.txt").write_text("\n".join(self.lines) + "\n")
        return not failed


def run(argv, stdin=None, timeout=300):
    return subprocess.run(argv, input=stdin, capture_output=True, text=True, timeout=timeout, check=False)


def coder(*args, binary=None, timeout=300, stdin=None):
    return run([binary or CODER, *args], timeout=timeout, stdin=stdin)


def kubectl(*args, namespaced=True, stdin=None):
    argv = ["kubectl"]
    if KUBE_CONTEXT:
        argv += ["--context", KUBE_CONTEXT]
    if namespaced:
        argv += ["-n", NAMESPACE]
    return run([*argv, *args], stdin=stdin)


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


def wait_until(predicate, timeout=120, interval=1):
    deadline = time.time() + timeout
    while time.time() < deadline:
        value = predicate()
        if value:
            return value
        time.sleep(interval)
    return None


def run_workload(command, name=None, scopes=(), detach=False, keep=False, binary=None, extra=(), timeout=300):
    args = ["run", "--template", TEMPLATE]
    if name:
        args += ["--name", name]
    for scope in scopes:
        args += ["--scope", scope]
    if detach:
        args.append("--detach")
    if keep:
        args.append("--keep")
    return coder(*args, *extra, "--", *command, binary=binary, timeout=timeout)


def probe(script, **kwargs):
    """Runs a shell script as a workload. Returns its output split by `@@ marker` lines."""
    out = run_workload(["sh", "-c", script], **kwargs)
    sections, current = {}, None
    for line in out.stdout.splitlines():
        if line.startswith("@@ "):
            current = line[3:].strip()
            sections[current] = []
        elif current is not None:
            sections[current].append(line)
    return {key: "\n".join(lines).strip() for key, lines in sections.items()}, out


def my_workloads(finished=False):
    out = coder("run", "--all" if finished else "--list", "--output", "json")
    return json.loads(out.stdout or "[]")


def workload_named(name, finished=False):
    return next((w for w in my_workloads(finished) if w["name"] == name), None)


def workspace_by_id(workspace_id):
    status, body = http("GET", f"/api/v2/workspaces/{workspace_id}?include_deleted=true", token=TOKEN)
    return body if status == 200 else None


def pod_of(workspace):
    """Returns what the build recorded about the workspace's pod."""
    for resource in workspace["latest_build"]["resources"]:
        if resource["type"] == "agent_sandbox":
            return {m["key"]: m["value"] for m in resource.get("metadata") or []}
    return {}


def pod_token(pod_name):
    return kubectl("exec", pod_name, "--", "cat", TOKEN_PATH).stdout.strip()


def exchange(token, wait=0):
    """Presents a pod token to coderd, as the daemon in the pod does."""
    return http("POST", "/api/v2/workspaceagents/kubernetes-pod-identity", body={"token": token, "wait_seconds": wait})


def claims(token):
    payload = token.split(".")[1]
    return json.loads(base64.urlsafe_b64decode(payload + "=" * (-len(payload) % 4)))


def pool():
    out = kubectl("get", "sandboxwarmpool", WARM_POOL, "-o", "json")
    return json.loads(out.stdout) if out.returncode == 0 else {}


def pool_settled():
    """The pool is full and no pod is coming or going."""
    p = pool()
    busy = [line for line in kubectl("get", "pods", "--no-headers").stdout.splitlines()
            if any(word in line for word in ("Terminating", "ContainerCreating", "Pending", "Init"))]
    return p.get("status", {}).get("readyReplicas") == p.get("spec", {}).get("replicas") and not busy


def settle():
    wait_until(pool_settled, timeout=180)
    # Daemons in new pods need a moment to start waiting for a claim.
    time.sleep(4)


def my_workspaces():
    return [w for w in http("GET", "/api/v2/workspaces?q=owner:me", token=TOKEN)[1]["workspaces"] if w["template_name"] == TEMPLATE]


def cleanup():
    """Removes the caller's workloads and their workspaces."""
    for workload in my_workloads():
        coder("run", "--remove", workload["id"])
    for workspace in my_workspaces():
        coder("delete", workspace["name"], "--yes")
    kubectl("delete", "pod", "stranger", "--ignore-not-found", "--wait=true")


def main():
    if not API or not TOKEN:
        print("CODER_URL and CODER_SESSION_TOKEN are required", file=sys.stderr)
        return 2
    report = Report()
    report.say(f"Sandbox workloads validation against {API}, namespace {NAMESPACE}")

    report.section("Preconditions")
    status, _ = http("GET", "/healthz")
    report.check("P1", "coderd is up", status == 200, f"GET /healthz -> {status}")
    _, experiments = http("GET", "/api/v2/experiments", token=TOKEN)
    report.check("P2", "the runtime-workloads experiment is enabled", "runtime-workloads" in experiments, experiments)
    _, me = http("GET", "/api/v2/users/me", token=TOKEN)
    user = me["username"]
    _, templates = http("GET", "/api/v2/templates", token=TOKEN)
    template = next((t for t in templates if t["name"] == TEMPLATE), {})
    report.check(
        "P3", "the template is built by the sandbox provisioner: no Terraform runs for its workspaces",
        template.get("provisioner") == "sandbox", f"template {TEMPLATE}: provisioner={template.get('provisioner')}",
    )
    cleanup()
    earlier = {w["id"] for w in my_workloads(finished=True)}
    settle()
    size = pool().get("spec", {}).get("replicas")
    report.check("P4", "the warm pool is full", pool_settled(), f"pool {WARM_POOL}: {size} pods ready and waiting")

    # ------------------------------------------------------------------
    report.section("A. A workload is a workspace of its own")
    out = run_workload(IDENTITY_COMMAND)
    first = json.loads(out.stdout) if out.returncode == 0 else {}
    report.check(
        "A1", "`coder run --template sandbox -- coder identity` prints the workload's own identity",
        first.get("kind") == "workload" and first.get("sponsor_username") == user and not first.get("scopes")
        and first.get("attestation") == "kubernetes-pod" and first.get("sandbox", {}).get("runtime") == "pod",
        f"{first.get('id')} kind={first.get('kind')} attestation={first.get('attestation')} sponsor={first.get('sponsor_username')} "
        f"scopes={first.get('scopes')} pod={first.get('sandbox', {}).get('id')}",
    )
    report.check(
        "A2", "the identity rests on the pod: it names the UID the cluster gave the workload's pod",
        first.get("attestation_subject", "").startswith("k8s-pod:") and first.get("workspace_name") == first.get("name"),
        f"attested: {first.get('attestation_subject')}; workspace {first.get('workspace_name')} was made for workload {first.get('name')}",
    )
    out = run_workload(["sh", "-c", "echo to-stdout; echo to-stderr >&2; exit 7"])
    report.check(
        "A3", "the command's output is streamed and its exit code returned",
        out.returncode == 7 and "to-stdout" in out.stdout and "to-stderr" in out.stdout,
        f"exit code {out.returncode}; output {out.stdout.split()}",
    )
    settle()
    timings = []
    for _ in range(5):
        started = time.time()
        timed = run_workload(["true"])
        timings.append(time.time() - started)
        if timed.returncode != 0:
            timings[-1] = float("inf")
        settle()
    report.check(
        "A4", "a workload starts in a new pod, runs and returns in about a second",
        statistics.median(timings) < 3,
        f"`coder run --template {TEMPLATE} -- true`, five runs with a full pool: median {statistics.median(timings):.2f}s, "
        f"range {min(timings):.2f}s to {max(timings):.2f}s",
    )
    typed = "echo typed-$((6*7))\ncoder identity --output json 2>/dev/null | grep '\"name\"'\nexit 4\n"
    out = run([CODER, "run", "-it", "--template", TEMPLATE, "--name", "terminal", "--", "sh"], stdin=typed, timeout=120)
    report.check(
        "A5", "`coder run -it` gives the sponsor a terminal in the workload's pod",
        out.returncode == 4 and "typed-42" in out.stdout and '"name": "terminal"' in out.stdout,
        f"exit code {out.returncode}; saw typed-42: {'typed-42' in out.stdout}",
    )
    settle()

    out_alpha = run_workload(["sleep", "900"], name="alpha", detach=True)
    # beta listens on a port, so that D4 can tell blocked from absent.
    out_beta = run_workload(["nc", "-lk", "-p", "8080", "-e", "echo", "beta-says-hi"], name="beta",
                            scopes=["workspace:read", "template:read"], detach=True)
    alpha, beta = workload_named("alpha"), workload_named("beta")
    ws_alpha, ws_beta = workspace_by_id(alpha["workspace_id"]), workspace_by_id(beta["workspace_id"])
    pod_alpha, pod_beta = pod_of(ws_alpha), pod_of(ws_beta)
    report.check(
        "A6", "two workloads are two workspaces in two pods, each claimed warm",
        out_alpha.returncode == 0 and out_beta.returncode == 0 and ws_alpha["id"] != ws_beta["id"]
        and pod_alpha.get("pod") and pod_alpha.get("pod") != pod_beta.get("pod")
        and pod_alpha.get("launch") == "warm" and pod_beta.get("launch") == "warm"
        and len(ws_alpha["latest_build"]["resources"][0]["agents"]) == 1,
        f"alpha: workspace {ws_alpha['name']}, pod {pod_alpha.get('pod')} ({pod_alpha.get('launch')}); "
        f"beta: workspace {ws_beta['name']}, pod {pod_beta.get('pod')} ({pod_beta.get('launch')})",
    )
    out = coder("ssh", ws_alpha["name"], "--", "sh", "-c", "hostname; ps -o args | grep -c '[s]leep 900'")
    report.check(
        "A7", "a workload's workspace is a workspace: its sponsor can open a shell next to the workload",
        out.returncode == 0 and out.stdout.split()[-2:] == [pod_alpha.get("pod"), "1"],
        f"`coder ssh {ws_alpha['name']}` ran in pod {out.stdout.split()[-2:-1]} and saw the workload's process",
    )

    # ------------------------------------------------------------------
    report.section("B. Who a pod is, is the cluster's word")
    token_alpha, token_beta = pod_token(pod_alpha["pod"]), pod_token(pod_beta["pod"])
    c = claims(token_alpha)
    k8s = c.get("kubernetes.io", {})
    lifetime = c.get("exp", 0) - c.get("iat", 0)
    report.check(
        "B1", "the kubelet gave the pod a token that says which pod it is, is for Coder only, and expires soon",
        k8s.get("pod", {}).get("uid") == pod_alpha.get("pod_uid") and k8s.get("pod", {}).get("name") == pod_alpha.get("pod")
        and len(c.get("aud", [])) == 1 and c["aud"][0] != "https://kubernetes.default.svc.cluster.local" and 0 < lifetime <= 3600,
        f"sub={c.get('sub')} pod={k8s.get('pod')} aud={c.get('aud')} lifetime={lifetime}s",
    )
    can = kubectl("auth", "can-i", "--list", f"--as=system:serviceaccount:{NAMESPACE}:coder-workload").stdout
    # Every account may ask what it may do, and read the API's discovery pages.
    verbs = [line.split()[0] for line in can.splitlines()[1:]
             if line.split() and not line.split()[0].startswith(("selfsubject", "/", "["))]
    mounted = kubectl("exec", pod_alpha["pod"], "--", "ls", "/var/run/secrets/").stdout.split()
    report.check(
        "B2", "the token is good for nothing in the cluster, and the pod has no other",
        verbs == [] and mounted == ["coder.com"],
        f"resources the pod's service account may touch: {verbs}; tokens mounted in the pod: {mounted}",
    )
    status, session = exchange(token_alpha)
    session_alpha = session.get("session_token", "") if isinstance(session, dict) else ""
    _, daemon = http("GET", "/api/v2/workspaceagents/me/identity", token=session_alpha)
    report.check(
        "B3", "coderd asks the cluster about the token and tells the pod's daemon which workspace it serves",
        status == 200 and daemon.get("kind") == "daemon" and daemon.get("workspace_id") == ws_alpha["id"]
        and daemon.get("attestation_subject") == "k8s-pod:" + pod_alpha["pod_uid"],
        f"exchange -> {status}; daemon of workspace {daemon.get('workspace_name')}, attested {daemon.get('attestation_subject')}",
    )
    _, as_alpha = http("GET", "/api/v2/workspaceagents/me/identity", token=session_alpha, headers={IDENTITY_HEADER: alpha["id"]})
    as_beta, _ = http("GET", "/api/v2/workspaceagents/me/identity", token=session_alpha, headers={IDENTITY_HEADER: beta["id"]})
    workspaces_as_beta, _ = http("GET", "/api/v2/workspaces", token=session_alpha, headers={IDENTITY_HEADER: beta["id"]})
    report.check(
        "B4", "alpha's pod is alpha and cannot be beta, whatever it claims",
        as_alpha.get("identity_id") == alpha["id"] and as_beta == 401 and workspaces_as_beta == 401,
        f"alpha's pod as alpha -> {as_alpha.get('name')}; naming beta's identity -> {as_beta}; using beta's scopes -> {workspaces_as_beta}",
    )
    # A pod of the pool that is ready and waiting.
    warm = next(p["metadata"]["name"] for p in json.loads(
        kubectl("get", "pods", "-l", "agents.x-k8s.io/warm-pool-sandbox", "-o", "json").stdout)["items"]
        if not p["metadata"].get("deletionTimestamp")
        and any(c["type"] == "Ready" and c["status"] == "True" for c in p["status"].get("conditions", [])))
    warm_token = pod_token(warm)
    status_warm, body_warm = exchange(warm_token)
    report.check(
        "B5", "a pod that nobody has claimed is nobody: its token is genuine and gets it nothing",
        len(warm_token) > 100 and status_warm == 404,
        f"warm pod {warm} -> {status_warm} {body_warm.get('message') if isinstance(body_warm, dict) else body_warm}",
    )
    header, payload, signature = token_alpha.split(".")
    forged_claims = claims(token_alpha)
    forged_claims["kubernetes.io"]["pod"] = {"name": pod_beta["pod"], "uid": pod_beta["pod_uid"]}
    forged_payload = base64.urlsafe_b64encode(json.dumps(forged_claims).encode()).decode().rstrip("=")
    unbound = kubectl("create", "token", "coder-workload", "--audience", c["aud"][0], "--duration", "10m").stdout.strip()
    for_cluster = kubectl("create", "token", "coder-workload", "--bound-object-kind", "Pod",
                          "--bound-object-name", pod_alpha["pod"], "--duration", "10m").stdout.strip()
    refused = {
        "alpha's token rewritten to name beta's pod": exchange(f"{header}.{forged_payload}.{signature}")[0],
        "a token of the service account that is bound to no pod": exchange(unbound)[0],
        "a token bound to alpha's pod, but for the Kubernetes API": exchange(for_cluster)[0],
        "no token": exchange("")[0],
    }
    report.check(
        "B6", "only a token the cluster issued to a pod, for Coder, is accepted",
        len(unbound) > 100 and len(for_cluster) > 100 and set(refused.values()) <= {400, 401} and refused["no token"] == 400,
        refused,
    )
    stranger = {
        "apiVersion": "v1", "kind": "Pod", "metadata": {"name": "stranger"},
        "spec": {
            "serviceAccountName": "coder-workload", "automountServiceAccountToken": False, "hostUsers": False,
            "terminationGracePeriodSeconds": 1,
            "securityContext": {"runAsUser": 0, "seccompProfile": {"type": "RuntimeDefault"}},
            "containers": [{
                "name": "stranger", "image": "alpine:3.22", "command": ["sleep", "600"],
                "securityContext": {"allowPrivilegeEscalation": False, "capabilities": {"drop": ["ALL"]}},
                "volumeMounts": [{"name": "identity", "mountPath": os.path.dirname(TOKEN_PATH), "readOnly": True}],
            }],
            "volumes": [{"name": "identity", "projected": {"sources": [
                {"serviceAccountToken": {"audience": c["aud"][0], "expirationSeconds": 600, "path": "token"}}]}}],
        },
    }
    kubectl("delete", "pod", "stranger", "--ignore-not-found", "--wait=true")
    kubectl("apply", "-f", "-", stdin=json.dumps(stranger))
    kubectl("wait", "--for=condition=Ready", "pod/stranger", "--timeout=120s")
    stranger_token = pod_token("stranger")
    status_stranger, _ = exchange(stranger_token)
    kubectl("delete", "pod", "stranger", "--wait=false")
    report.check(
        "B7", "a pod somebody else starts with the same service account is nobody, however genuine its token",
        len(stranger_token) > 100 and claims(stranger_token)["kubernetes.io"]["pod"]["name"] == "stranger" and status_stranger == 404,
        f"pod `stranger`, not from any claim, same service account and audience -> {status_stranger}",
    )
    key = {"daemon of a workload's pod": http("GET", "/api/v2/workspaceagents/me/gitsshkey", token=session_alpha)[0],
           "the same session on the user API": http("GET", "/api/v2/users/me", token=session_alpha)[0]}
    report.check(
        "B8", "what a pod is given is the daemon's session, which is not a user's: no Git SSH key, no user API",
        key == {"daemon of a workload's pod": 403, "the same session on the user API": 401}, key,
    )

    # ------------------------------------------------------------------
    report.section("C. Permissions")
    nothing, _ = probe("""
echo "@@ workspaces"; coder exp api workspaces 2>/dev/null
echo "@@ templates"; coder exp api templates 2>/dev/null
echo "@@ own"; coder exp api workspaces/{ws} >/dev/null 2>&1; echo $?
echo "@@ end"
""".replace("{ws}", ws_alpha["id"]))
    report.check(
        "C1", "a workload without scopes can see nothing in Coder",
        json.loads(nothing.get("workspaces") or "{}").get("count") == 0 and json.loads(nothing.get("templates") or "[1]") == []
        and nothing.get("own") not in (None, "0"),
        f"workspaces visible: {json.loads(nothing.get('workspaces') or '{}').get('count')}; templates: {nothing.get('templates')}",
    )
    reader, _ = probe("""
echo "@@ workspaces"; coder exp api workspaces 2>/dev/null
echo "@@ plain"; wget -q -O- $CODER_URL/api/v2/templates 2>&1
echo "@@ run"; coder exp api POST workspaces/{ws}/workloads command=true >/dev/null 2>&1; echo $?
echo "@@ list"; coder exp api workloads >/dev/null 2>&1; echo $?
echo "@@ end"
""".replace("{ws}", ws_alpha["id"]), scopes=["workspace:read", "template:read"])
    seen = sorted(w["name"] for w in json.loads(reader.get("workspaces") or "{}").get("workspaces", []))
    plain = [t["name"] for t in json.loads(reader.get("plain") or "[]")]
    report.check(
        "C2", "with read scopes it sees what its sponsor can see, from any HTTP client and without a token",
        {"alpha", "beta"} <= set(seen) and TEMPLATE in plain,
        f"its sponsor's workspaces, as the workload sees them: {seen}; templates through $CODER_URL with wget: {plain}",
    )
    report.check(
        "C3", "whatever its scopes, a workload cannot run workloads or learn of its sponsor's other workloads",
        reader.get("run") not in (None, "0") and reader.get("list") not in (None, "0"),
        f"exit codes: run a workload={reader.get('run')}, list workloads={reader.get('list')}",
    )
    out = run_workload(["true"], name="greedy", scopes=["coder:all"])
    left = wait_until(lambda: not [w for w in my_workspaces() if w["name"] == "greedy"], timeout=60)
    report.check(
        "C4", "a workload cannot be given all of its sponsor's permissions, and a refused run leaves no workspace",
        out.returncode != 0 and "every permission" in out.stderr and left,
        f"{out.stderr.strip().splitlines()[-2:-1]}; workspace `greedy` removed again: {bool(left)}",
    )
    if OTHER_CODER:
        theirs = run_workload(IDENTITY_COMMAND, binary=OTHER_CODER)
        other = json.loads(theirs.stdout) if theirs.returncode == 0 else {}
        remove = coder("run", "--remove", alpha["id"], binary=OTHER_CODER)
        ssh = coder("ssh", f"{user}/{ws_alpha['name']}", "--", "true", binary=OTHER_CODER)
        listed = json.loads(coder("run", "--all", "--output", "json", binary=OTHER_CODER).stdout or "[]")
        report.check(
            "C5", "another user runs workloads of their own and cannot see, enter or remove this sponsor's",
            other.get("kind") == "workload" and other.get("sponsor_username") not in (None, user)
            and remove.returncode != 0 and ssh.returncode != 0 and all(w["sponsor_username"] != user for w in listed)
            and workload_named("alpha") is not None,
            f"their workload is sponsored by {other.get('sponsor_username')}; removing alpha -> exit {remove.returncode}; "
            f"ssh into alpha's workspace -> exit {ssh.returncode}; workloads they see of {user}'s: 0",
        )
    else:
        report.info("C5 skipped: set OTHER_CODER_BIN to a CLI signed in as another, ordinary user")

    # ------------------------------------------------------------------
    report.section("D. The pod is the sandbox")
    labels = json.loads(kubectl("get", "namespace", NAMESPACE, "-o", "json", namespaced=False).stdout)["metadata"]["labels"]
    spec = json.loads(kubectl("get", "pod", pod_alpha["pod"], "-o", "json").stdout)["spec"]
    container = spec["containers"][0]
    inside, _ = probe("""
echo "@@ id"; id -u; cat /proc/self/uid_map; grep CapEff /proc/self/status
echo "@@ limits"; cat /sys/fs/cgroup/memory.max /sys/fs/cgroup/cpu.max
echo "@@ coder"; wget -q -T 4 -O- $CODER_URL/api/v2/workspaceagents/me/identity 2>&1 | head -c 300; echo
echo "@@ direct"; wget -q -T 4 -O /dev/null {api}/healthz 2>&1 && echo reached || echo unreachable
echo "@@ cluster"; wget -q -T 4 -O /dev/null --no-check-certificate https://kubernetes.default.svc 2>&1 && echo reached || echo unreachable
echo "@@ internet"; wget -q -T 4 -O /dev/null http://1.1.1.1 2>&1 && echo reached || echo unreachable
echo "@@ neighbour"; nc -w 4 {neighbour} 8080 </dev/null 2>&1 | grep -q beta-says-hi && echo reached || echo unreachable
echo "@@ end"
""".replace("{api}", c["aud"][0]).replace("{neighbour}", json.loads(
        kubectl("get", "pod", pod_beta["pod"], "-o", "json").stdout)["status"]["podIP"]))
    ident = inside.get("id", "").split()
    report.check(
        "D1", "the pod meets the Restricted Pod Security Standard: nothing privileged, no capabilities",
        labels.get("pod-security.kubernetes.io/enforce") == "restricted" and spec.get("hostUsers") is False
        and not container["securityContext"].get("privileged") and container["securityContext"]["capabilities"]["drop"] == ["ALL"]
        and "add" not in container["securityContext"]["capabilities"],
        f"namespace enforces {labels.get('pod-security.kubernetes.io/enforce')}; hostUsers={spec.get('hostUsers')}; "
        f"capabilities={container['securityContext']['capabilities']}",
    )
    report.check(
        "D2", "root in the pod is an unprivileged user on the node and has no capabilities in the pod",
        len(ident) >= 6 and ident[0] == "0" and int(ident[2]) > 65535 and ident[-1] == "0000000000000000",
        f"uid in the pod {ident[0] if ident else '?'} is uid {ident[2] if len(ident) > 2 else '?'} on the node; CapEff={ident[-1] if ident else '?'}",
    )
    limits = inside.get("limits", "").split()
    report.check(
        "D3", "the workload has resource limits of its own, which are its pod's",
        len(limits) == 3 and limits[0].isdigit() and limits[1].isdigit()
        and container["resources"]["limits"].keys() >= {"cpu", "memory"},
        f"pod limits {container['resources']['limits']}; seen inside: memory.max={limits[0] if limits else '?'} cpu.max={' '.join(limits[1:])}",
    )
    reach = {key: inside.get(key, "").splitlines()[-1:] for key in ("direct", "cluster", "internet", "neighbour")}
    listening = kubectl("exec", pod_beta["pod"], "--", "nc", "-w", "3", "127.0.0.1", "8080").stdout.strip()
    report.check(
        "D4", "the pool's network policy lets the pod reach Coder and nothing else, not even another workload",
        reach == {"direct": ["reached"], "cluster": ["unreachable"], "internet": ["unreachable"], "neighbour": ["unreachable"]}
        and '"kind":"workload"' in inside.get("coder", "").replace(" ", "") and listening == "beta-says-hi",
        f"coderd: {reach['direct']}; Kubernetes API: {reach['cluster']}; internet: {reach['internet']}; "
        f"beta's port, which answers `{listening}` inside beta's pod: {reach['neighbour']}",
    )

    # ------------------------------------------------------------------
    report.section("E. Lifecycle and the record")
    out = coder("run", "--remove", "beta")
    gone = wait_until(lambda: kubectl("get", "pod", pod_beta["pod"]).returncode != 0, timeout=90)
    status_token, _ = exchange(token_beta)
    record = workload_named("beta", finished=True)
    deleted = wait_until(lambda: (lambda w: w if w and w["latest_build"]["status"] == "deleted" else None)(workspace_by_id(ws_beta["id"])), timeout=60)
    report.check(
        "E1", "removing a workload ends its identity, deletes its workspace and gives its pod back",
        out.returncode == 0 and gone and status_token in (400, 401, 404) and record and record["status"] == "removed" and deleted,
        f"pod {pod_beta['pod']} gone={bool(gone)}; its token now -> {status_token}; record: {record['status'] if record else None}; "
        f"workspace: {deleted['latest_build']['status'] if deleted else 'still there'}",
    )
    run_workload(["sh", "-c", "echo working; sleep 6; echo finished >&2; exit 5"], name="shortlived", detach=True)
    short = workload_named("shortlived")
    short_pod = pod_of(workspace_by_id(short["workspace_id"]))
    record = wait_until(lambda: (lambda w: w if w and w.get("finished_at") else None)(workload_named("shortlived", finished=True)), timeout=60)
    logs = coder("run", "--logs", "shortlived")
    pod_gone = wait_until(lambda: kubectl("get", "pod", short_pod["pod"]).returncode != 0, timeout=90)
    report.check(
        "E2", "when the command ends, the outcome and the output stay on record and the pod is given back",
        record and record["status"] == "exited" and record.get("exit_code") == 5 and logs.returncode == 5
        and logs.stdout.split() == ["working", "finished"] and pod_gone,
        f"record: {record['status'] if record else None} ({record.get('exit_code') if record else None}); "
        f"`coder run --logs` after the workspace is gone: {logs.stdout.split()}, exit {logs.returncode}; pod gone={bool(pod_gone)}",
    )
    session_status = http("GET", "/api/v2/workspaceagents/me/identity", token=session_alpha, headers={IDENTITY_HEADER: alpha["id"]})[0]
    kubectl("delete", "pod", pod_alpha["pod"], "--wait=true")
    lost = wait_until(lambda: http("GET", "/api/v2/workspaceagents/me/identity", token=session_alpha,
                                   headers={IDENTITY_HEADER: alpha["id"]})[0] == 401 or None, timeout=120, interval=3)
    record = wait_until(lambda: (lambda w: w if w and w.get("finished_at") else None)(workload_named("alpha", finished=True)), timeout=120, interval=3)
    deleted = wait_until(lambda: (lambda w: w if w and w["latest_build"]["status"] == "deleted" else None)(workspace_by_id(ws_alpha["id"])), timeout=60)
    report.check(
        "E3", "a pod that is deleted behind Coder's back takes its workload's identity and workspace with it",
        session_status == 200 and lost and record and record["status"] in ("lost", "stopped", "exited") and deleted,
        f"the daemon's session before -> {session_status}; after the pod was deleted -> {401 if lost else 'still accepted'}; "
        f"record: {record['status'] if record else None}; workspace: {deleted['latest_build']['status'] if deleted else 'still there'}",
    )
    settle()
    kept = run_workload(["true"], name="kept", keep=True)
    ws_kept = next((w for w in my_workspaces() if w["name"] == "kept"), None)
    first_pod = pod_of(ws_kept) if ws_kept else {}
    stop = coder("stop", "kept", "--yes")
    stopped = wait_until(lambda: kubectl("get", "pod", first_pod.get("pod", "none")).returncode != 0, timeout=90)
    start = coder("start", "kept", "--yes")
    ws_again = workspace_by_id(ws_kept["id"]) if ws_kept else {}
    second_pod = pod_of(ws_again) if ws_again else {}
    again = coder("ssh", "kept", "--", "hostname")
    report.check(
        "E4", "a kept workspace stops and starts like any workspace: stopping gives the pod back, starting claims another",
        kept.returncode == 0 and stop.returncode == 0 and stopped and start.returncode == 0
        and second_pod.get("pod") not in (None, first_pod.get("pod")) and again.stdout.split()[-1:] == [second_pod.get("pod")],
        f"first pod {first_pod.get('pod')} gone after stop={bool(stopped)}; after start: pod {second_pod.get('pod')}, "
        f"`coder ssh kept -- hostname` -> {again.stdout.split()[-1:]}",
    )
    coder("delete", "kept", "--yes")
    settle()

    count = (size or 3) + 2
    started = time.time()
    with concurrent.futures.ThreadPoolExecutor(max_workers=count) as executor:
        burst = list(executor.map(lambda i: (run_workload(["hostname"], name=f"burst-{i}", timeout=600), time.time() - started), range(count)))
    hosts = {out.stdout.strip() for out, _ in burst if out.returncode == 0}
    took = sorted(round(elapsed, 1) for _, elapsed in burst)
    report.check(
        "E5", "more workloads than the pool holds still all run, each in its own pod: the rest wait for a new pod",
        len(hosts) == count,
        f"{count} at once on a pool of {size}: {len(hosts)} distinct pods; they returned after {took} seconds",
    )
    refilled = wait_until(lambda: pool_settled() and not kubectl("get", "sandboxclaims", "--no-headers").stdout.strip(), timeout=240)
    records = [w for w in my_workloads(finished=True) if w["id"] not in earlier]
    report.check(
        "E6", "afterwards the pool is full again, and every workload of this run is on record",
        refilled and len(records) >= 14 + count and all(w.get("finished_at") for w in records),
        f"pool full={bool(refilled)}; {len(records)} workloads on record, all finished: "
        + ", ".join(sorted({f"{w['status']}" for w in records})),
    )

    # ------------------------------------------------------------------
    report.section("F. The audit log")
    status, page = http("GET", "/api/v2/audit?limit=300&q=resource_type%3Aworkload_identity", token=TOKEN)
    if status != 200:
        report.info(f"skipped: the audit log is not available here (GET /api/v2/audit -> {status}). It needs a license.")
    else:
        ids = {w["id"]: w["name"] for w in records}
        entries = {}
        for entry in page["audit_logs"]:
            name = ids.get((entry.get("additional_fields") or {}).get("workload_identity_id"))
            if name:
                entries.setdefault(name, []).append(entry["action"])
        report.check(
            "F1", "every workload of this run has a start entry and an end entry, and the terminal session is recorded",
            all({"start", "stop"} <= set(entries.get(name, [])) for name in ids.values()) and "connect" in entries.get("terminal", []),
            f"{len(ids)} workloads; entries for terminal: {sorted(entries.get('terminal', []))}",
        )
        _, builds = http("GET", "/api/v2/audit?limit=300&q=resource_type%3Aworkspace_build", token=TOKEN)
        # Names repeat across runs of this script. The workspace's ID does not.
        mine = [e for e in builds["audit_logs"] if (e.get("additional_fields") or {}).get("workspace_id") == short["workspace_id"]]
        report.check(
            "F2", "because a workload is a workspace, its pod's coming and going is in the audit log as workspace builds",
            sorted(e["action"] for e in mine) == ["delete", "start"],
            f"workspace builds of `shortlived`: {sorted((e['action'], e['status_code']) for e in mine)}",
        )

    # ------------------------------------------------------------------
    report.section("G. Footprint")
    kinds = {}
    for kind in ("deployments", "serviceaccounts", "roles", "secrets", "sandboxclaims"):
        items = json.loads(kubectl("get", kind, "-o", "json").stdout)["items"]
        kinds[kind] = sorted(i["metadata"]["name"] for i in items)
    rules = json.loads(kubectl("get", "role", "coder-server", "-o", "json").stdout)["rules"]
    report.check(
        "G1", "the cluster holds the Agent Sandbox controller and the pool; Coder may claim sandboxes and review tokens, no more",
        kinds["deployments"] == ["agent-sandbox-controller"] and kinds["secrets"] == [] and kinds["sandboxclaims"] == []
        and sorted(r["resources"][0] for r in rules) == ["pods", "sandboxclaims", "sandboxes"]
        and all(set(r["verbs"]) <= {"create", "get", "delete"} for r in rules),
        f"{ {kind: names for kind, names in kinds.items()} }; coderd's role: {[(r['resources'], r['verbs']) for r in rules]}",
    )

    ok = report.finish()
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
