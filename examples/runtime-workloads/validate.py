#!/usr/bin/env python3
"""Validates the runtime-workloads proof of concept against a running deployment.

It expects what README.md sets up: coderd with the runtime-workloads
experiment, the workload operator in a namespace, and a running workspace
from the template in this directory.

Settings come from the environment:

  CODER_URL            coderd, as reachable from here (required)
  CODER_SESSION_TOKEN  API token of the workspace owner, who must also be an
                       organization administrator (required)
  POD_CODER_URL        coderd's access URL, as pods reach it. It is also the
                       audience of pod tokens. Defaults to CODER_URL.
  CODER_BIN            the coder CLI built from this branch. Defaults to coder.
  KUBE_CONTEXT         kubeconfig context. Defaults to the current context.
  NAMESPACE            defaults to coder-runtime-workloads
  WORKSPACE            workspace from this template. Defaults to dev.
  PLAIN_WORKSPACE      optional: a running workspace from any ordinary
                       single-agent template, built before the operator was
                       installed. Enables checks A1 to A3.
  OTHER_WORKSPACE      optional: owner/name of a running workspace from this
                       template that another user owns and the token's user
                       can see. Enables check F8.
  RUNS_DIR             where results go. Defaults to ./runs.

The script writes a transcript and results.json to RUNS_DIR/<timestamp>/ and
exits non-zero if any check fails. No credential is written to the output.
Pass --rotation to also wait ten minutes for a pod token to expire.
"""

import base64
import datetime
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

CODER = os.environ.get("CODER_BIN", "coder")
NAMESPACE = os.environ.get("NAMESPACE", "coder-runtime-workloads")
KUBECTL = ["kubectl"] + (["--context", os.environ["KUBE_CONTEXT"]] if os.environ.get("KUBE_CONTEXT") else [])
API = os.environ["CODER_URL"].rstrip("/")
POD_API = os.environ.get("POD_CODER_URL", API).rstrip("/")
WORKSPACE = os.environ.get("WORKSPACE", "dev")
PLAIN_WORKSPACE = os.environ.get("PLAIN_WORKSPACE", "")
OTHER_WORKSPACE = os.environ.get("OTHER_WORKSPACE", "")
RUNS_DIR = pathlib.Path(os.environ.get("RUNS_DIR", "runs"))
OPERATOR_SA = f"system:serviceaccount:{NAMESPACE}:coder-workload-operator"
WORKLOAD_SA = f"system:serviceaccount:{NAMESPACE}:coder-workload"
TOKEN_PATH = "/var/run/secrets/coder.com/identity/token"
POD_TOKEN_TTL = 600


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


def kubectl(*args, timeout=120, stdin=None, namespaced=True):
    cmd = KUBECTL + (["-n", NAMESPACE] if namespaced else []) + list(args)
    return run(cmd, timeout=timeout, stdin=stdin)


def admin_token():
    return os.environ["CODER_SESSION_TOKEN"]


def http(method, path, token=None, body=None, headers=None):
    """Calls coderd. Returns (status, decoded JSON or text)."""
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


def identity_with(token, query="", headers=None):
    return http("GET", "/api/v2/workspaceagents/me/identity" + query, token=token, headers=headers)


def parse_time(value):
    """Parses an RFC 3339 timestamp, with any offset, to UTC."""
    value = value.replace("Z", "+00:00")
    # Trim fractional seconds: Python accepts at most six digits.
    if "." in value:
        head, rest = value.split(".", 1)
        value = head + rest.lstrip("0123456789")
    return datetime.datetime.fromisoformat(value).astimezone(datetime.timezone.utc)


def workspace(name):
    out = coder("list", "--output", "json")
    for ws in json.loads(out.stdout):
        if ws["name"] == name:
            return ws
    raise RuntimeError(f"workspace {name} not found: {out.stderr}")


def agents(ws):
    return [a for r in ws["latest_build"]["resources"] for a in (r.get("agents") or [])]


def workloads():
    out = coder("run", "--list", WORKSPACE, "--output", "json")
    return json.loads(out.stdout or "[]")


def wait_until(predicate, timeout=300, interval=3):
    deadline = time.time() + timeout
    while time.time() < deadline:
        value = predicate()
        if value:
            return value
        time.sleep(interval)
    return None


def pod(name):
    out = kubectl("get", "pod", name, "-o", "json")
    return json.loads(out.stdout) if out.returncode == 0 else None


def pod_token(name):
    out = kubectl("exec", name, "--", "cat", TOKEN_PATH)
    if out.returncode != 0:
        raise RuntimeError(f"read token of {name}: {out.stderr}")
    return out.stdout.strip()


def b64d(part):
    return base64.urlsafe_b64decode(part + "=" * (-len(part) % 4))


def b64e(raw):
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()


def claims(token):
    return json.loads(b64d(token.split(".")[1]))


def tamper(token, mutate):
    """Rewrites a token's claims and keeps its original signature."""
    header, payload, signature = token.split(".")
    data = json.loads(b64d(payload))
    mutate(data)
    return ".".join([header, b64e(json.dumps(data, separators=(",", ":")).encode()), signature])


def forge(token):
    """Re-signs a token's exact header and claims with a key the cluster does not know."""
    header, payload, _ = token.split(".")
    with tempfile.TemporaryDirectory() as tmp:
        key = pathlib.Path(tmp) / "key.pem"
        run(["openssl", "genrsa", "-out", str(key), "2048"])
        signed = subprocess.run(
            ["openssl", "dgst", "-sha256", "-sign", str(key)],
            input=f"{header}.{payload}".encode(), capture_output=True, check=True,
        )
    return ".".join([header, payload, b64e(signed.stdout)])


def create_token(*args):
    out = kubectl("create", "token", "coder-workload", *args)
    if out.returncode != 0:
        raise RuntimeError(f"create token: {out.stderr}")
    return out.stdout.strip()


def kube_api_url():
    out = kubectl("config", "view", "--minify", "-o", "jsonpath={.clusters[0].cluster.server}", namespaced=False)
    return out.stdout.strip()


def can_i(subject, verb, resource, all_namespaces=False):
    # kubectl reads "pods/exec" as the pod named exec, so a subresource has
    # to be passed separately.
    resource, _, subresource = resource.partition("/")
    args = ["auth", "can-i", verb, resource, "--as", subject]
    if subresource:
        args.append(f"--subresource={subresource}")
    out = kubectl(*args, "-A", namespaced=False) if all_namespaces else kubectl(*args)
    return out.stdout.strip()


def run_workload(name, command=None, timeout=420, scopes=()):
    args = ["run", WORKSPACE, "--name", name, "--timeout", "6m"]
    for scope in scopes:
        args += ["--scope", scope]
    if command:
        args += ["--", *command]
    return coder(*args, timeout=timeout)


def workload_pod_names():
    """Names of the pods of this workspace's workloads. Other workspaces may share the namespace."""
    return [w["pod"]["name"] for w in workloads() if w.get("pod")]


def pods_left(names):
    return [name for name in names if pod(name) is not None]


def remove_all_workloads():
    names = workload_pod_names()
    for workload in workloads():
        coder("run", "--remove", workload["name"], WORKSPACE)
    wait_until(lambda: not pods_left(names), timeout=240)


def workload_pods():
    out = kubectl("get", "pods", "-l", "coder.com/workload-operator=poc", "-o", "json")
    return json.loads(out.stdout)["items"]


IMPOSTOR_POD = """
apiVersion: v1
kind: Pod
metadata:
  name: {name}
  labels: {labels}
spec:
  serviceAccountName: coder-workload
  automountServiceAccountToken: false
  terminationGracePeriodSeconds: 1
  containers:
    - name: main
      image: busybox:1.37
      command: ["sleep", "3600"]
"""


def create_impostor(name, labels=None):
    manifest = IMPOSTOR_POD.format(name=name, labels=json.dumps(labels or {"purpose": "impostor"}))
    out = kubectl("apply", "-f", "-", stdin=manifest)
    if out.returncode != 0:
        raise RuntimeError(f"create impostor: {out.stderr}")
    return pod(name)["metadata"]["uid"]


def main():
    report = Report()
    token = admin_token()
    _, me = http("GET", "/api/v2/users/me", token=token)
    user = me["username"]

    report.section("Preconditions")
    status, _ = http("GET", "/healthz")
    report.check("P1", "coderd is up", status == 200, f"GET /healthz -> {status}")
    status, experiments = http("GET", "/api/v2/experiments", token=token)
    report.check("P2", "the runtime-workloads experiment is enabled", "runtime-workloads" in experiments, experiments)
    operator = kubectl("get", "deploy", "coder-workload-operator", "-o", "json")
    operator = json.loads(operator.stdout)
    report.check("P3", "the operator runs in the cluster", operator["status"].get("readyReplicas") == 1)
    remove_all_workloads()
    kubectl("delete", "pod", "impostor", "impostor-bound", "impostor-pending", "--ignore-not-found", "--wait=false")

    # ------------------------------------------------------------------
    report.section("A. Existing behaviour is unchanged")
    if PLAIN_WORKSPACE:
        check_plain_workspace(report, operator)
    else:
        report.info("A1 to A3 skipped: set PLAIN_WORKSPACE to check an ordinary single-agent workspace")

    dev = workspace(WORKSPACE)
    build_before = dev["latest_build"]["id"]
    by_name = {a["name"]: a for a in agents(dev)}
    main_agent, shape_agent = by_name["main"], by_name["workload"]
    report.check(
        "A4", "the human's daemon is connected and the shape, which never connects, does not make the workspace unhealthy",
        main_agent["status"] == "connected" and shape_agent["status"] != "connected" and dev["health"]["healthy"],
        f"main={main_agent['status']} shape={shape_agent['status']} healthy={dev['health']['healthy']}",
    )

    main_pod = kubectl("get", "pods", "-l", f"app.kubernetes.io/instance=coder-{dev['id']}-main", "-o", "json")
    main_pod = json.loads(main_pod.stdout)["items"][0]["metadata"]["name"]
    out = kubectl("exec", main_pod, "--", "sh", "-c",
                  f"CODER_AGENT_URL={POD_API} $(ls -d /tmp/coder.*/coder | head -1) identity --output json")
    human = json.loads(out.stdout)
    report.check(
        "A5", "the human's daemon keeps its agent token and has a declared, not pod-attested, identity",
        human["kind"] == "declared" and human["attestation"] == "agent-token"
        and human["id"] == f"{dev['id']}/agent/{main_agent['id']}" and human["owner_username"] == user,
        f"id={human['id']} kind={human['kind']} attestation={human['attestation']} owner={human['owner_username']}",
    )

    out = coder("exp", "rpty", f"{WORKSPACE}.main", "--", "coder", "whoami", timeout=90)
    report.check(
        "A6", "`coder whoami` in the human's terminal shows no user session and the daemon's workload identity",
        "No user is logged in here" in out.stdout and human["id"] in out.stdout and "template-declared daemon" in out.stdout,
        [line.strip() for line in out.stdout.splitlines() if "identity" in line or "Kind" in line or "logged" in line],
    )

    out = coder("whoami")
    report.check(
        "A7", "`coder whoami` on the human's own machine shows the user and no workload identity",
        f"authenticated as {user}" in out.stdout and "Workload identity" not in out.stdout, out.stdout.strip(),
    )
    out = coder("identity")
    report.check(
        "A8", "`coder identity` outside a workspace says there is no workload identity",
        out.returncode != 0 and "no workload identity" in out.stderr, out.stderr.strip().splitlines()[-1:],
    )

    # ------------------------------------------------------------------
    report.section("B. Runtime provisioning with `coder run`")
    keeper_out = run_workload("keeper")
    keeper_started = time.time()
    report.check("B1", "`coder run dev --name keeper` provisions a daemon without a command",
                 keeper_out.returncode == 0, keeper_out.stderr.strip().splitlines()[-4:])
    keeper_token_first = pod_token("workload-keeper")

    identities = {}
    for name in ("alpha", "beta"):
        out = run_workload(name, ["coder", "identity", "--output", "json"])
        try:
            identities[name] = json.loads(out.stdout)
        except ValueError:
            identities[name] = {}
        ident = identities[name]
        live = pod(f"workload-{name}")
        report.check(
            f"B2-{name}", f"`coder run dev --name {name} -- coder identity` prints that workload's own identity",
            out.returncode == 0 and ident.get("kind") == "runtime" and ident.get("attestation") == "kubernetes-pod"
            and ident.get("agent_name") == name and ident.get("pod", {}).get("uid") == live["metadata"]["uid"]
            and ident.get("id") == f"{dev['id']}/pod/{live['metadata']['uid']}",
            f"id={ident.get('id')} kind={ident.get('kind')} attestation={ident.get('attestation')} "
            f"pod={ident.get('pod', {}).get('name')}" if ident else out.stderr[-400:],
        )
    alpha, beta = identities["alpha"], identities["beta"]

    report.check(
        "B3", "the two workloads and the human's daemon have three different identities",
        len({alpha.get("id"), beta.get("id"), human["id"]}) == 3
        and len({alpha.get("agent_id"), beta.get("agent_id"), human["agent_id"]}) == 3,
        f"alpha={alpha.get('id')}\nbeta={beta.get('id')}\nhuman={human['id']}",
    )
    report.check(
        "B4", "every workload has its own identity record, sponsored by the workspace owner who requested it",
        all(i.get("owner_id") == human["owner_id"] and i.get("sponsor_id") == human["owner_id"]
            and i.get("sponsor_username") == user and i.get("shape") == "workload" for i in (alpha, beta))
        and alpha.get("identity_id") and alpha.get("identity_id") != beta.get("identity_id"),
        f"sponsor={alpha.get('sponsor_username')} alpha={alpha.get('identity_id')} beta={beta.get('identity_id')}",
    )
    dev = workspace(WORKSPACE)
    report.check(
        "B5", "no workspace build ran: the workloads were added to the existing build",
        dev["latest_build"]["id"] == build_before, f"latest build is still {build_before}",
    )
    children = {a["name"]: a for a in agents(dev) if a.get("parent_id") == shape_agent["id"]}
    report.check(
        "B6", "the workloads appear in the workspace as agents cloned from the shape",
        {"keeper", "alpha", "beta"} <= set(children), f"children of the shape agent: {sorted(children)}",
    )

    alpha_pod = pod("workload-alpha")
    spec = alpha_pod["spec"]
    env = {e["name"]: e.get("value") for c in spec["containers"] for e in c.get("env", [])}
    deployment = json.loads(kubectl("get", "deploy", f"coder-{dev['id']}-workload", "-o", "json").stdout)
    selector = deployment["spec"]["selector"]["matchLabels"]
    labels = alpha_pod["metadata"]["labels"]
    owners = alpha_pod["metadata"].get("ownerReferences", [])
    report.check(
        "B7", "the pod is owned by the shape Deployment, which still has zero replicas, and does not match its selector",
        len(owners) == 1 and owners[0]["kind"] == "Deployment" and owners[0]["uid"] == deployment["metadata"]["uid"]
        and deployment["spec"]["replicas"] == 0 and not all(labels.get(k) == v for k, v in selector.items()),
        f"owner={owners[0]['kind']}/{owners[0]['name']} replicas={deployment['spec']['replicas']}",
    )
    shape_container = deployment["spec"]["template"]["spec"]["containers"][0]
    report.check(
        "B8", "the pod is a clone of the shape: same image, command and volumes, plus the identity volume",
        spec["containers"][0]["image"] == shape_container["image"]
        and spec["containers"][0]["command"] == shape_container["command"]
        and [v["name"] for v in spec["volumes"]] == [v["name"] for v in deployment["spec"]["template"]["spec"]["volumes"]] + ["coder-workload-identity"],
        f"image={spec['containers'][0]['image'][:30]}... volumes={[v['name'] for v in spec['volumes']]}",
    )
    report.check(
        "B9", "the pod spec holds no Coder credential: the shape's agent token is removed",
        "CODER_AGENT_TOKEN" not in env and env.get("CODER_AGENT_POD_TOKEN_FILE") == TOKEN_PATH
        and "CODER_AGENT_TOKEN" in {e["name"] for e in shape_container["env"]},
        f"workload pod env names: {sorted(env)}",
    )
    report.check(
        "B10", "the pod runs as the permissionless service account and mounts no Kubernetes API token",
        spec["serviceAccountName"] == "coder-workload" and spec.get("automountServiceAccountToken") is False,
        f"serviceAccountName={spec['serviceAccountName']} automount={spec.get('automountServiceAccountToken')}",
    )
    secrets = json.loads(kubectl("get", "secrets", "-o", "json").stdout)["items"]
    report.check(
        "B11", "no Secret was created for any workload",
        [s["metadata"]["name"] for s in secrets] == ["coder-operator"], [s["metadata"]["name"] for s in secrets],
    )
    status, logs = http("GET", f"/api/v2/workspaceagents/{alpha['agent_id']}/logs", token=token)
    report.check(
        "B12", "the workload ran the shape's own startup script as well as its command",
        any("shape startup script ran in pod workload-alpha" in line["output"] for line in logs),
        [line["output"] for line in logs if "shape startup" in line["output"]],
    )

    # ------------------------------------------------------------------
    report.section("C. Masquerade attempts")
    alpha_token = pod_token("workload-alpha")
    beta_token = pod_token("workload-beta")
    alpha_claims = claims(alpha_token)
    report.info(f"alpha's token: aud={alpha_claims['aud']} lifetime={alpha_claims['exp'] - alpha_claims['iat']}s "
                f"pod={alpha_claims['kubernetes.io']['pod']['name']}")

    status, got = identity_with(
        alpha_token,
        query=f"?agent_id={beta['agent_id']}&workload=beta&pod_uid={beta['pod']['uid']}",
        headers={"X-Coder-Agent-ID": beta["agent_id"], "X-Coder-Workload": "beta", "Coder-Agent-Name": "beta"},
    )
    report.check(
        "C1", "alpha's token with beta's IDs in headers and query still yields alpha",
        status == 200 and got["id"] == alpha["id"], f"{status} id={got.get('id') if isinstance(got, dict) else got}",
    )
    status, got = identity_with(beta_token)
    report.check("C2", "beta's token yields beta", status == 200 and got["id"] == beta["id"], f"{status} id={got.get('id')}")

    def become_beta(data):
        data["kubernetes.io"]["pod"] = {"name": "workload-beta", "uid": beta["pod"]["uid"]}

    status, got = identity_with(tamper(alpha_token, become_beta))
    report.check("C3", "alpha's token rewritten to name beta's pod is refused", status == 401, f"{status} {got}")
    status, got = identity_with(forge(alpha_token))
    report.check("C4", "alpha's exact claims signed with a key the cluster does not publish are refused",
                 status == 401, f"{status} {got}")
    status, got = identity_with(create_token("--audience", POD_API))
    report.check("C5", "a service account token for the right audience that is not bound to a pod is refused",
                 status == 401, f"{status} {got}")
    status, got = identity_with(create_token("--bound-object-kind", "Pod", "--bound-object-name", "workload-alpha"))
    report.check("C6", "a token bound to alpha's pod but issued for the Kubernetes API audience is refused",
                 status == 401, f"{status} {got}")

    impostor_uid = create_impostor("impostor")
    impostor_token = create_token("--audience", POD_API, "--bound-object-kind", "Pod", "--bound-object-name", "impostor")
    status, got = identity_with(impostor_token)
    report.check(
        "C7", "a pod the operator did not create, with the same service account and audience, gets no identity",
        status == 401 and claims(impostor_token)["kubernetes.io"]["pod"]["uid"] == impostor_uid, f"{status} {got}",
    )
    kubectl("delete", "pod", "impostor", "--wait=false")

    bound_uid = create_impostor("impostor-bound", {
        "coder.com/workload-operator": "poc", "coder.com/workload-id": beta["agent_id"], "coder.com/workload-name": "beta",
    })
    bound_token = create_token("--audience", POD_API, "--bound-object-kind", "Pod", "--bound-object-name", "impostor-bound")
    status, got = identity_with(bound_token)
    gone = wait_until(lambda: (pod("impostor-bound") or {}).get("metadata", {}).get("deletionTimestamp") or pod("impostor-bound") is None, timeout=90)
    still_beta = identity_with(pod_token("workload-beta"))
    report.check(
        "C8", "a foreign pod labelled as beta gets no identity, is deleted by the operator, and beta is unaffected",
        status == 401 and gone and still_beta[0] == 200 and still_beta[1]["id"] == beta["id"]
        and pod("workload-beta")["metadata"]["uid"] == beta["pod"]["uid"],
        f"impostor ({bound_uid[:8]}) -> {status}; deleted={bool(gone)}; beta -> {still_beta[0]} {still_beta[1].get('id')}",
    )

    status, got = http("GET", "/api/v2/users/me", token=alpha_token)
    status2, got2 = http("POST", f"/api/v2/workspaces/{dev['id']}/workloads", token=alpha_token, body={"name": "spawned"})
    status3, got3 = http("POST", f"/api/v2/workspaces/{dev['id']}/workloads", token=human_agent_token(main_pod), body={"name": "spawned"})
    report.check(
        "C9", "neither a workload's pod token nor the human daemon's agent token can call `coder run` or read the user",
        status in (403, 404) and status2 in (403, 404) and status3 == 401,
        f"users/me with pod token -> {status}; create workload with pod token -> {status2}; with agent token -> {status3}",
    )

    answers = {
        "get pods": can_i(WORKLOAD_SA, "get", "pods"),
        "list secrets": can_i(WORKLOAD_SA, "list", "secrets"),
        "get deployments": can_i(WORKLOAD_SA, "get", "deployments.apps"),
        "create pods": can_i(WORKLOAD_SA, "create", "pods"),
        "create pods/exec": can_i(WORKLOAD_SA, "create", "pods/exec"),
        "create serviceaccounts/token": can_i(WORKLOAD_SA, "create", "serviceaccounts/token"),
    }
    report.check("C10", "the workload service account has no Kubernetes permissions, so it cannot read another daemon's pod or token",
                 set(answers.values()) == {"no"}, answers)
    out = kubectl("exec", "workload-alpha", "--", "sh", "-c", "ls /var/run/secrets/kubernetes.io 2>&1")
    kube_api = run(["curl", "-sk", "-o", "/dev/null", "-w", "%{http_code}", "-H", f"Authorization: Bearer {alpha_token}",
                    f"{kube_api_url()}/api/v1/namespaces/{NAMESPACE}/pods"])
    report.check(
        "C11", "inside alpha there is no Kubernetes API token, and the Kubernetes API rejects its identity token",
        "No such file" in out.stdout and kube_api.stdout.strip() == "401",
        f"ls /var/run/secrets/kubernetes.io: {out.stdout.strip()}; Kubernetes API with alpha's token -> {kube_api.stdout.strip()}",
    )
    out = kubectl("exec", "workload-alpha", "--", "sh", "-c",
                  "cat /proc/1/environ | tr '\\0' '\\n' | grep -c '^CODER_AGENT_TOKEN=' ; "
                  f"ls /var/run/secrets/coder.com/identity/")
    report.check(
        "C12", "alpha's container environment holds no agent token; its only credential is its own pod token",
        out.stdout.split()[0] == "0" and "token" in out.stdout, out.stdout.strip().replace("\n", " | "),
    )

    # A workload that does not exist yet must not be claimable either.
    kubectl("scale", "deploy", "coder-workload-operator", "--replicas=0")
    wait_until(lambda: not json.loads(kubectl("get", "pods", "-l", "app.kubernetes.io/name=coder-workload-operator", "-o", "json").stdout)["items"], timeout=120)
    out = coder("run", WORKSPACE, "--name", "delta", "--no-wait")
    delta_id = out.stdout.strip()
    pending_uid = create_impostor("impostor-pending", {
        "coder.com/workload-operator": "poc", "coder.com/workload-id": delta_id, "coder.com/workload-name": "delta",
    })
    pending_token = create_token("--audience", POD_API, "--bound-object-kind", "Pod", "--bound-object-name", "impostor-pending")
    status_before, _ = identity_with(pending_token)
    kubectl("scale", "deploy", "coder-workload-operator", "--replicas=1")
    delta = wait_until(lambda: next((w for w in workloads() if w["name"] == "delta" and w["status"] == "running"), None), timeout=420)
    status_after, _ = identity_with(pending_token)
    impostor_gone = wait_until(lambda: pod("impostor-pending") is None or pod("impostor-pending")["metadata"].get("deletionTimestamp"), timeout=90)
    report.check(
        "C13", "a foreign pod created first with a pending workload's labels does not become that workload",
        out.returncode == 0 and status_before == 401 and status_after == 401 and delta is not None
        and delta["pod"]["uid"] != pending_uid and delta["pod"]["name"] == "workload-delta" and impostor_gone,
        f"impostor -> {status_before} before and {status_after} after the operator returned; "
        f"delta runs in {delta['pod']['name'] if delta else None}; impostor deleted={bool(impostor_gone)}",
    )

    # ------------------------------------------------------------------
    report.section("D. Revocation and lifecycle")
    status, got = identity_with(alpha_token)
    report.check(
        "D1", "known limit: alpha's token copied out of its pod works from elsewhere while the pod lives",
        status == 200 and got["id"] == alpha["id"], f"{status} id={got.get('id')} (the request came from this host, not the pod)",
    )
    out = coder("run", "--remove", "alpha", WORKSPACE)
    status, got = identity_with(alpha_token)
    remaining = alpha_claims["exp"] - int(time.time())
    report.check(
        "D2", "removing the workload revokes its identity at once, although the copied token has not expired",
        out.returncode == 0 and status == 401 and remaining > 0, f"{status} {got}; token had {remaining}s left",
    )
    gone = wait_until(lambda: pod("workload-alpha") is None, timeout=180)
    report.check("D3", "the operator then deletes the removed workload's pod", bool(gone))

    kubectl("delete", "pod", "workload-beta", "--wait=false")
    dropped = wait_until(lambda: not any(w["name"] == "beta" for w in workloads()), timeout=120)
    status, got = identity_with(beta_token)
    report.check(
        "D4", "deleting a pod behind Coder's back removes the workload and its identity; it is not recreated",
        bool(dropped) and status == 401, f"beta listed={not dropped}; beta's token -> {status}",
    )
    time.sleep(10)
    report.check("D5", "no replacement pod was created for beta",
                 pod("workload-beta") is None or pod("workload-beta")["metadata"].get("deletionTimestamp") is not None)

    keeper = next(w for w in workloads() if w["name"] == "keeper")
    if "--rotation" in sys.argv:
        # Kubernetes issues no pod token shorter than ten minutes, so this
        # check has to wait that long. It is opt-in for that reason.
        wait = keeper_started + POD_TOKEN_TTL + 60 - time.time()
        if wait > 0:
            report.info(f"waiting {int(wait)}s so that keeper's first token expires and the kubelet rotates it")
            time.sleep(wait)
        keeper_token_now = pod_token("workload-keeper")
        status_old, _ = identity_with(keeper_token_first)
        status_new, _ = identity_with(keeper_token_now)
        keeper = next(w for w in workloads() if w["name"] == "keeper")
        report.check(
            "D6", "a pod token expires after ten minutes; the daemon stays connected on the token the kubelet rotated in",
            status_old == 401 and status_new == 200 and keeper["status"] == "running"
            and claims(keeper_token_now)["iat"] > claims(keeper_token_first)["iat"],
            f"first token -> {status_old}; rotated token -> {status_new}; keeper is {keeper['status']} after {int(time.time() - keeper_started)}s",
        )
    else:
        report.info("D6 (token expiry and rotation) skipped: pass --rotation to wait ten minutes for it")
    out = coder("exp", "rpty", f"{WORKSPACE}.keeper", "--", "coder", "whoami", timeout=90)
    report.check(
        "D7", "`coder whoami` in keeper's terminal shows keeper's pod-attested identity",
        keeper["identity"] in out.stdout and "attested by its Kubernetes pod" in out.stdout,
        [line.strip() for line in out.stdout.splitlines() if "identity" in line or "Kind" in line],
    )

    keeper_token = pod_token("workload-keeper")
    names = workload_pod_names()
    out = coder("stop", WORKSPACE, "--yes", timeout=600)
    pods_gone = wait_until(lambda: not pods_left(names), timeout=300)
    status, got = identity_with(keeper_token)
    listed = workloads()
    report.check(
        "D8", "stopping the workspace removes every runtime daemon: pods, identities and records",
        out.returncode == 0 and bool(pods_gone) and status == 401 and listed == [],
        f"workload pods left={len(pods_left(names))}; keeper's token -> {status}; workloads listed={len(listed)}",
    )
    out = coder("start", WORKSPACE, "--yes", timeout=600)
    restarted = wait_until(
        lambda: (lambda ws: ws if {a["name"]: a["status"] for a in agents(ws)}.get("main") == "connected" else None)(workspace(WORKSPACE)),
        timeout=420,
    )
    report.check(
        "D9", "starting it again restores the declared state: the human's daemon, the shape, and no workloads",
        out.returncode == 0 and restarted is not None and workloads() == [] and not pods_left(names)
        and restarted["health"]["healthy"],
        f"agents={sorted(a['name'] for a in agents(restarted))}" if restarted else out.stderr[-300:],
    )
    wait_until(lambda: json.loads(kubectl("get", "deploy", "-l", "coder.com/workload-shape=true", "-o", "json").stdout)["items"], timeout=60)
    time.sleep(6)
    out = run_workload("after-restart", ["coder", "identity", "--output", "json"])
    for _ in range(10):
        # The operator needs one sync to register the new build's shape.
        if "no workload shape" not in out.stderr:
            break
        time.sleep(5)
        out = run_workload("after-restart", ["coder", "identity", "--output", "json"])
    try:
        fresh = json.loads(out.stdout)
    except ValueError:
        fresh = {}
    report.check(
        "D10", "`coder run` works again on the new build, with a new identity",
        out.returncode == 0 and fresh.get("kind") == "runtime" and fresh.get("id") not in {alpha["id"], beta["id"], keeper["identity"]},
        f"id={fresh.get('id')}" if fresh else out.stderr[-300:],
    )
    coder("run", "--remove", "after-restart", WORKSPACE)

    # ------------------------------------------------------------------
    report.section("E. Operator scope")
    answers = {
        "list pods in its namespace": can_i(OPERATOR_SA, "list", "pods"),
        "create pods in its namespace": can_i(OPERATOR_SA, "create", "pods"),
        "get deployments in its namespace": can_i(OPERATOR_SA, "get", "deployments.apps"),
        "get secrets in its namespace": can_i(OPERATOR_SA, "get", "secrets"),
        "create pods/exec in its namespace": can_i(OPERATOR_SA, "create", "pods/exec"),
        "list pods in all namespaces": can_i(OPERATOR_SA, "list", "pods", all_namespaces=True),
    }
    report.check(
        "E1", "the operator can manage pods and read Deployments in its namespace, and nothing else",
        list(answers.values()) == ["yes", "yes", "yes", "no", "no", "no"], answers,
    )
    bindings = json.loads(kubectl("get", "clusterrolebindings", "-o", "json", namespaced=False).stdout)["items"]
    mine = [b["metadata"]["name"] for b in bindings
            if any(s.get("namespace") == NAMESPACE for s in (b.get("subjects") or []))]
    report.check("E2", "nothing in the POC namespace is bound to a cluster-wide role", mine == [], mine)

    # ------------------------------------------------------------------
    report.section("F. Permissions")
    check_permissions(report, token, dev)

    ok = report.finish()
    kubectl("get", "pods", "-o", "wide")
    return 0 if ok else 1


def summarize(status, body):
    """Reduces an API response to what the checks compare."""
    if status != 200:
        return status
    if isinstance(body, dict) and "workspaces" in body:
        return sorted(w["id"] for w in body["workspaces"])
    if isinstance(body, list):
        return sorted(item["id"] for item in body)
    return status


def check_permissions(report, token, dev):
    """A workload acts as its sponsor, narrowed to the scopes it was given."""
    grants = {
        "perm-none": [],
        "perm-templates": ["template:read"],
        "perm-reader": ["workspace:read", "template:read"],
    }
    tokens, identities = {}, {}
    for name, scopes in grants.items():
        out = run_workload(name, scopes=scopes)
        if out.returncode != 0:
            report.check("F0", f"workload {name} starts", False, out.stderr[-300:])
            return
        tokens[name] = pod_token(f"workload-{name}")
        _, identities[name] = identity_with(tokens[name])
    report.check(
        "F1", "each workload reports the scopes it was given, and none by default",
        all(identities[name].get("scopes", []) == scopes for name, scopes in grants.items())
        and len({identities[name]["identity_id"] for name in grants}) == 3,
        {name: identities[name].get("scopes", []) for name in grants},
    )

    def view(name, path):
        return summarize(*http("GET", path, token=tokens[name]))

    sponsor = {path: summarize(*http("GET", path, token=token)) for path in ("/api/v2/workspaces", "/api/v2/templates")}
    report.info(f"the sponsor sees {len(sponsor['/api/v2/workspaces'])} workspaces and {len(sponsor['/api/v2/templates'])} templates")

    none = {
        "users/me": view("perm-none", "/api/v2/users/me"),
        "workspaces": view("perm-none", "/api/v2/workspaces"),
        "templates": view("perm-none", "/api/v2/templates"),
        "its own workspace": view("perm-none", f"/api/v2/workspaces/{dev['id']}"),
        "organizations": view("perm-none", "/api/v2/organizations"),
        "sponsor's Git SSH key": view("perm-none", "/api/v2/workspaceagents/me/gitsshkey"),
    }
    report.check(
        "F2", "a workload without scopes can see nothing in Coder and gets none of its sponsor's credentials",
        none == {"users/me": 404, "workspaces": [], "templates": [], "its own workspace": 404,
                 "organizations": [], "sponsor's Git SSH key": 403},
        none,
    )
    templates_only = (view("perm-templates", "/api/v2/templates"), view("perm-templates", "/api/v2/workspaces"))
    report.check(
        "F3", "with template:read a workload lists its sponsor's templates, and still no workspaces",
        templates_only == (sponsor["/api/v2/templates"], []) and sponsor["/api/v2/templates"],
        f"templates={len(templates_only[0]) if isinstance(templates_only[0], list) else templates_only[0]} workspaces={templates_only[1]}",
    )
    reader = (view("perm-reader", "/api/v2/workspaces"), view("perm-reader", "/api/v2/templates"))
    report.check(
        "F4", "with workspace:read and template:read a workload lists exactly the workspaces its sponsor can see",
        reader == (sponsor["/api/v2/workspaces"], sponsor["/api/v2/templates"]) and dev["id"] in reader[0],
        f"workspaces={len(reader[0]) if isinstance(reader[0], list) else reader[0]} (sponsor sees {len(sponsor['/api/v2/workspaces'])})",
    )
    out = coder("run", WORKSPACE, "--name", "perm-cli", "--scope", "workspace:read", "--scope", "template:read",
                "--timeout", "6m", "--", "coder", "list", timeout=420)
    report.check(
        "F5", "`coder list` inside such a workload works without a login, using the workload's own identity",
        out.returncode == 0 and WORKSPACE in out.stdout, [line for line in out.stdout.splitlines() if WORKSPACE in line][:2],
    )
    writes = {
        "change the workspace's autostart": http("PUT", f"/api/v2/workspaces/{dev['id']}/autostart", token=tokens["perm-reader"], body={"schedule": ""})[0],
        "run another workload": http("POST", f"/api/v2/workspaces/{dev['id']}/workloads", token=tokens["perm-reader"], body={"name": "spawned"})[0],
        "remove a workload": http("DELETE", f"/api/v2/workspaces/{dev['id']}/workloads/{identities['perm-none']['agent_id']}", token=tokens["perm-reader"])[0],
        "read the sponsor's API keys": http("GET", "/api/v2/users/me/keys", token=tokens["perm-reader"])[0],
    }
    report.check(
        "F6", "read scopes do not allow writes, and a workload cannot run or remove workloads",
        all(status >= 400 for status in writes.values()) and not any(w["name"] == "spawned" for w in workloads()), writes,
    )
    out = coder("run", WORKSPACE, "--scope", "workspace:fly", "--", "true")
    report.check("F7", "an unknown scope is refused", out.returncode != 0 and "not a scope" in out.stderr,
                 out.stderr.strip().splitlines()[-2:-1])
    if OTHER_WORKSPACE:
        out = coder("run", OTHER_WORKSPACE, "--", "true")
        report.check(
            "F8", "nobody but the owner can run a workload in a workspace, not even an administrator",
            out.returncode != 0 and "Only the owner" in out.stderr, out.stderr.strip().splitlines()[-1:],
        )
    else:
        report.info("F8 skipped: set OTHER_WORKSPACE to a running workspace from this template that another user owns")
    coder("run", "--remove", "perm-reader", WORKSPACE)
    status, _ = http("GET", "/api/v2/workspaces", token=tokens["perm-reader"])
    report.check("F9", "removing a workload revokes its access to the user API at once", status == 401, f"workspaces -> {status}")
    for name in ("perm-none", "perm-templates", "perm-cli"):
        coder("run", "--remove", name, WORKSPACE)


def check_plain_workspace(report, operator):
    plain = workspace(PLAIN_WORKSPACE)
    plain_agents = agents(plain)
    report.check(
        "A1", "a single-daemon workspace from an ordinary template is connected and healthy",
        len(plain_agents) == 1 and plain_agents[0]["status"] == "connected" and plain["health"]["healthy"],
        f"agents={[(a['name'], a['status']) for a in plain_agents]} healthy={plain['health']['healthy']}",
    )
    built = parse_time(plain["latest_build"]["created_at"])
    installed = parse_time(operator["metadata"]["creationTimestamp"])
    report.check(
        "A2", "that workspace was built before the operator was installed",
        built < installed, f"workspace build {built:%H:%M:%S} UTC, operator created {installed:%H:%M:%S} UTC",
    )
    out = coder("run", PLAIN_WORKSPACE, "--", "true")
    report.check(
        "A3", "`coder run` on a workspace without a shape is refused with a clear message",
        out.returncode != 0 and "no workload shape" in out.stderr, out.stderr.strip().splitlines()[-2:],
    )


def human_agent_token(main_pod):
    """Reads the human daemon's agent token from its own container, as that daemon could."""
    out = kubectl("exec", main_pod, "--", "sh", "-c", "printf %s \"$CODER_AGENT_TOKEN\"")
    return out.stdout.strip()


if __name__ == "__main__":
    sys.exit(main())
