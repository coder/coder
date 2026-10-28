"""Validate one local Coder workspace on an explicitly selected disposable kind cluster.

Run prepare, stop, restart, and delete in order, with the same private state directory.
The delete phase destroys the tested workspace, including its persistent disk.
"""

import argparse
import base64
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import time
import urllib.parse
import urllib.request
import uuid
import xml.etree.ElementTree as ET


parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("phase", choices=["prepare", "stop", "restart", "delete"])
parser.add_argument("--state-dir", type=Path, required=True)
parser.add_argument("--coder-config", type=Path, required=True)
parser.add_argument("--coder-binary", default="coder")
parser.add_argument("--url", required=True)
parser.add_argument("--kubeconfig", type=Path, required=True)
parser.add_argument("--namespace", default="coder-kubevirt-prototype")
parser.add_argument("--workspace", default="kv-workspace")
args = parser.parse_args()
url = urllib.parse.urlsplit(args.url)
if url.scheme not in ("http", "https") or url.hostname not in ("localhost", "127.0.0.1", "::1"):
    parser.error("Use a loopback URL for an isolated local Coder instance.")

os.umask(0o077)
root = args.state_dir.resolve()
root.mkdir(mode=0o700, parents=True, exist_ok=True)
if root.stat().st_mode & 0o077:
    parser.error("The state directory must be private (mode 0700).")
source = Path(__file__).resolve().parent.parent
coder = [args.coder_binary, "--global-config", str(args.coder_config), "--url", args.url]
kube = ["kubectl", "--kubeconfig", str(args.kubeconfig)]
env = {key: value for key, value in os.environ.items() if not key.startswith("CODER_")}
source_names = [
    "main.tf", "bootstrap-agent.sh", "cloud-config.yaml.tftpl", "provisioner-role.yaml",
    "tests/Dockerfile", "tests/compose.yaml", "tests/workloads.sh",
]
phases = ["prepare", "stop", "restart", "delete"]


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def request(path, raw=False):
    token = (args.coder_config / "session").read_text().strip()
    req = urllib.request.Request(args.url.rstrip("/") + "/api/v2" + path,
                                 headers={"Coder-Session-Token": token})
    with urllib.request.urlopen(req, timeout=30) as response:
        body = response.read()
        return body if raw else json.loads(body)


def workspace():
    return request("/users/me/workspace/" + urllib.parse.quote(args.workspace, safe=""))


def command(argv, name, data=None, timeout=1800):
    result = subprocess.run(argv, input=data, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, env=env, timeout=timeout)
    (root / (name + ".log")).write_bytes(result.stdout + result.stderr)
    require(result.returncode == 0, f"{name} exited {result.returncode}; inspect its private log")
    return result.stdout.decode()


def kubectl(*argv):
    result = subprocess.run(kube + list(argv), check=True, capture_output=True,
                            text=True, timeout=60)
    return result.stdout


def resource(kind, name, namespace=None):
    return json.loads(kubectl("-n", namespace or args.namespace, "get", kind, name, "-o", "json"))


def selected(kind, workspace_id):
    return json.loads(kubectl("-n", args.namespace, "get", kind, "-l",
                              "com.coder.workspace.id=" + workspace_id, "-o", "json"))["items"]


def source_hashes():
    return {name: hashlib.sha256((source / name).read_bytes()).hexdigest() for name in source_names}


def poll(check, seconds, description):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        value = check()
        if value:
            return value
        time.sleep(3)
    raise RuntimeError("Timed out waiting for " + description)


def ready():
    ws = workspace()
    build = ws["latest_build"]
    require(build["status"] != "failed", "Workspace build failed; inspect Coder build logs")
    agents = [a for r in build.get("resources", []) for a in (r.get("agents") or [])]
    if (build["status"] == "running" and len(agents) == 1
            and agents[0]["status"] == "connected" and agents[0]["lifecycle_state"] == "ready"):
        return ws, agents[0]
    return None


def provenance(ws):
    version_id = ws["latest_build"]["template_version_id"]
    version = request("/templateversions/" + version_id)
    archive = request("/files/" + version["job"]["file_id"], raw=True)
    with tarfile.open(fileobj=io.BytesIO(archive), mode="r:*") as tar:
        members = {member.name.removeprefix("./"): member for member in tar.getmembers()}
        allowed = set(source_names) | {
            ".terraform.lock.hcl", "README.md", "EVIDENCE.md", "tests/lifecycle.py",
            "evidence/2026-10-28.json",
        }
        unexpected = [name for name, member in members.items() if not member.isdir() and name not in allowed]
        require(not unexpected, "Unexpected uploaded template files: " + ", ".join(sorted(unexpected)))
        for name, digest in source_hashes().items():
            member = members[name]
            require(member.isfile(), "Non-file template member: " + name)
            require(hashlib.sha256(tar.extractfile(member).read()).hexdigest() == digest,
                    "Running template differs from local source: " + name)
    return {"template_version_id": version_id, "template_archive_sha256": hashlib.sha256(archive).hexdigest()}


def observe(ws, agent):
    name = "coder-" + ws["id"]
    vm = resource("vm", name)
    vmi = resource("vmi", name)
    dv = resource("dv", name + "-root")
    pvc = resource("pvc", name + "-root")
    secret = resource("secret", name + "-agent")
    require(vmi["status"]["phase"] == "Running", "VMI is not Running")
    require(dv["status"]["phase"] == "Succeeded", "Disk import did not succeed")
    require(pvc["status"]["phase"] == "Bound", "Root PVC is not Bound")
    require(any(c["type"] == "Ready" and c["status"] == "True" for c in vm["status"]["conditions"]),
            "KubeVirt VM is not Ready")
    for kind in ["virtualmachines", "virtualmachineinstances", "pods", "secrets"]:
        require(selected(kind, ws["id"]), "Ownership selector finds no running " + kind)
    pods = selected("pods", ws["id"])
    launcher = next(p for p in pods if any(c["name"] == "compute" for c in p["spec"]["containers"]))
    xml = kubectl("-n", args.namespace, "exec", launcher["metadata"]["name"], "-c", "compute", "--",
                  "virsh", "-c", "qemu:///session", "dumpxml", args.namespace + "_" + name)
    accelerator = ET.fromstring(xml).attrib["type"]
    digest = hashlib.sha256(base64.b64decode(secret["data"]["agent.env"])).hexdigest()
    guest_digest = command(coder + ["ssh", args.workspace, "--", "sudo sha256sum /run/coder-agent.env"],
                           args.phase + "-guest-credential-digest").split()[0]
    require(digest == guest_digest, "Guest is not using this start's credential payload")
    instance_id = command(coder + ["ssh", args.workspace, "--", "sudo cat /var/lib/cloud/data/instance-id"],
                          args.phase + "-instance-id").strip()
    require(instance_id == ws["id"], "Guest cloud-init identity differs from workspace identity")
    sem = command(coder + ["ssh", args.workspace, "--", "sudo stat -c %Y /var/lib/cloud/instances/"
                          + ws["id"] + "/sem/config_write_files"], args.phase + "-first-boot-semaphore").strip()
    command(coder + ["ssh", args.workspace, "--",
                    "sudo test -f /run/coder/credentials/agent.env && "
                    "sudo -u nobody test ! -r /run/coder/credentials/agent.env && "
                    "sudo -u nobody test ! -r /run/coder-agent.env"], args.phase + "-credential-permissions")
    observation = {
        "workspace_id": ws["id"], "agent_id": agent["id"], "vm_uid": vm["metadata"]["uid"],
        "vmi_uid": vmi["metadata"]["uid"], "datavolume_uid": dv["metadata"]["uid"],
        "pvc_uid": pvc["metadata"]["uid"], "credential_secret_uid": secret["metadata"]["uid"],
        "firmware_uuid": vm["spec"]["template"]["spec"]["domain"]["firmware"]["uuid"],
        "cloud_init_instance_id": instance_id, "first_boot_semaphore_mtime": sem,
        "cpu_model": vmi["spec"]["domain"]["cpu"]["model"], "libvirt_domain_type": accelerator,
        "guest_credentials_match_secret": True, "credentials_unreadable_by_nobody": True,
    }
    return observation, digest


def absent(kind, name):
    return not kubectl("-n", args.namespace, "get", kind, name, "--ignore-not-found", "-o", "name").strip()


def no_compute(workspace_id):
    name = "coder-" + workspace_id
    return (all(absent(kind, item) for kind, item in [
        ("vm", name), ("vmi", name), ("secret", name + "-agent"), ("secret", name + "-cloudinit")])
        and all(not selected(kind, workspace_id) for kind in ["pods", "secrets"]))


context = kubectl("config", "current-context").strip()
require(context.startswith("kind-coder-kv-"), "Refusing a context that is not this disposable kind lab")
cluster_uid = json.loads(kubectl("get", "namespace", "kube-system", "-o", "json"))["metadata"]["uid"]
path = root / "evidence.json"
checker_hash = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
if path.exists():
    evidence = json.loads(path.read_text())
else:
    require(args.phase == "prepare", "Start a fresh evidence run with prepare")
    evidence = {"run_id": uuid.uuid4().hex, "complete": False, "cluster_uid": cluster_uid,
                "namespace": args.namespace, "workspace": args.workspace, "coder_url": args.url,
                "source_sha256": source_hashes(), "checker_sha256": checker_hash, "checks": []}
require(evidence["cluster_uid"] == cluster_uid, "Cluster identity changed")
require((evidence["namespace"], evidence["workspace"], evidence["coder_url"]) ==
        (args.namespace, args.workspace, args.url), "Validation target changed")
require(evidence["source_sha256"] == source_hashes(), "Source changed during validation")
require(evidence["checker_sha256"] == checker_hash, "Lifecycle checker changed during validation")
previous = evidence["checks"]
require(all(c["passed"] for c in previous), "Failed run is immutable; use a fresh fixture and state directory")
require([c["phase"] for c in previous] == phases[:phases.index(args.phase)], "Phases must run once, in order")
if previous:
    require(workspace()["id"] == evidence["before"]["workspace_id"], "Workspace identity changed")
record = {"phase": args.phase, "started_at_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), "passed": False}
evidence["checks"].append(record)
path.write_text(json.dumps(evidence, indent=2) + "\n")
try:
    if args.phase == "prepare":
        ws, agent = poll(ready, 2400, "the actual Coder agent to connect and finish startup")
        record.update(provenance(ws))
        observation, digest = observe(ws, agent)
        evidence["before"] = observation
        (root / "credential-digest-before").write_text(digest)
        command(coder + ["ssh", args.workspace, "--", "test ! -e /home/coder/kubevirt-prototype-tests && "
                         "mkdir -m 700 /home/coder/kubevirt-prototype-tests"], "create-test-directory")
        archive = io.BytesIO()
        with tarfile.open(fileobj=archive, mode="w") as tar:
            for name in ["Dockerfile", "compose.yaml", "workloads.sh"]:
                tar.add(source / "tests" / name, arcname=name)
        command(coder + ["ssh", args.workspace, "--", "tar -xf - -C /home/coder/kubevirt-prototype-tests"],
                "transfer-tests", archive.getvalue())
        record["output"] = command(coder + ["ssh", args.workspace, "--",
                                           "bash /home/coder/kubevirt-prototype-tests/workloads.sh prepare"], "workloads-prepare")
    elif args.phase == "stop":
        before = evidence["before"]
        command(coder + ["stop", args.workspace, "--yes"], "coder-stop")
        poll(lambda: no_compute(before["workspace_id"]), 600, "compute and credentials to disappear")
        pvc = resource("pvc", "coder-" + before["workspace_id"] + "-root")
        require(pvc["metadata"]["uid"] == before["pvc_uid"], "Root disk was replaced or lost")
        require(workspace()["latest_build"]["status"] == "stopped", "Coder workspace is not stopped")
        record.update(compute_and_credentials_absent=True, same_pvc_retained=True)
    elif args.phase == "restart":
        command(coder + ["start", args.workspace, "--yes"], "coder-restart")
        ws, agent = poll(ready, 2400, "the new Coder agent to connect and finish startup")
        record.update(provenance(ws))
        require(record["template_version_id"] == evidence["checks"][0]["template_version_id"], "Template version changed")
        observation, digest = observe(ws, agent)
        before = evidence["before"]
        for key in ["pvc_uid", "datavolume_uid", "firmware_uuid", "cloud_init_instance_id", "first_boot_semaphore_mtime"]:
            require(observation[key] == before[key], key + " did not persist")
        for key in ["vm_uid", "vmi_uid", "agent_id", "credential_secret_uid"]:
            require(observation[key] != before[key], key + " did not rotate")
        require(digest != (root / "credential-digest-before").read_text(), "Agent token did not rotate")
        evidence["after"] = observation
        record["fresh_agent_token"] = True
        record["output"] = command(coder + ["ssh", args.workspace, "--",
                                           "bash /home/coder/kubevirt-prototype-tests/workloads.sh verify"], "workloads-verify")
    elif args.phase == "delete":
        name = "coder-" + evidence["before"]["workspace_id"]
        command(coder + ["delete", args.workspace, "--yes"], "coder-delete")
        poll(lambda: no_compute(evidence["before"]["workspace_id"]), 600, "compute deletion")
        poll(lambda: absent("pvc", name + "-root") and absent("dv", name + "-root"), 600, "storage deletion")
        record["owned_storage_deleted"] = True
    require(evidence["source_sha256"] == source_hashes(), "Source changed during validation")
    record["passed"] = True
    evidence["complete"] = [c["phase"] for c in evidence["checks"]] == phases and all(c["passed"] for c in evidence["checks"])
except Exception as error:
    record["error"] = str(error)
    raise
finally:
    record["finished_at_utc"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    path.write_text(json.dumps(evidence, indent=2) + "\n")
    print(json.dumps({key: value for key, value in record.items() if key != "output"}))
