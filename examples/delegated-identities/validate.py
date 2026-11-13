#!/usr/bin/env python3
"""Checks delegated identities on a running deployment.

It walks through what a person, an agent and a trusted service do with a
delegated identity, in ordinary workspaces of an ordinary template.

The script needs an administrator's and a member's command line:

    CODER_URL=https://coder.example.com \\
    CODER_SESSION_TOKEN=<administrator's token> CODER_BIN=coder \\
    MEMBER_TOKEN=<member's token> MEMBER_CODER_BIN=<member's coder> \\
    python3 validate.py

It writes what it saw to RUNS_DIR and exits non-zero when a check failed.
"""

import datetime
import json
import os
import pathlib
import subprocess
import sys
import time
import urllib.error
import urllib.request

API = os.environ.get("CODER_URL", "").rstrip("/")
ADMIN_TOKEN = os.environ.get("CODER_SESSION_TOKEN", "")
ADMIN = os.environ.get("CODER_BIN", "coder")
MEMBER_TOKEN = os.environ.get("MEMBER_TOKEN", "")
MEMBER = os.environ.get("MEMBER_CODER_BIN", "")
TEMPLATE = os.environ.get("TEMPLATE", "prebuilt")
PRESET = os.environ.get("PRESET", "agent")
RUNS_DIR = pathlib.Path(os.environ.get("RUNS_DIR", "runs"))
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


def api(token, method, path, body=None):
    """Returns the status and the decoded body of an API request."""
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(API + path, data=data, method=method, headers={
        "Coder-Session-Token": token,
        "Content-Type": "application/json",
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


def wait_for(what, timeout=180, interval=1.0):
    deadline = time.time() + timeout
    while time.time() < deadline:
        got = what()
        if got:
            return got
        time.sleep(interval)
    return None


def workspace_ready(token, workspace_id):
    """Returns the workspace once its daemon is connected."""
    def look():
        status, ws = api(token, "GET", f"/api/v2/workspaces/{workspace_id}")
        if status != 200 or ws["latest_build"]["status"] != "running":
            return None
        agents = [a for r in ws["latest_build"]["resources"] for a in (r.get("agents") or [])]
        return ws if agents and all(a["status"] == "connected" for a in agents) else None
    return wait_for(look)


def delete_workspace(token, workspace_id):
    status, build = api(token, "POST", f"/api/v2/workspaces/{workspace_id}/builds", {"transition": "delete"})
    if status != 201:
        return status, build

    def gone():
        s, ws = api(ADMIN_TOKEN, "GET", f"/api/v2/workspaces/{workspace_id}?include_deleted=true")
        return s == 200 and ws["latest_build"]["job"]["status"] in ("succeeded", "failed")
    wait_for(gone)
    return status, build


def main():
    if not (API and ADMIN_TOKEN and MEMBER_TOKEN and MEMBER):
        print(__doc__)
        return 2
    report = Report()
    say, check = report.say, report.check

    _, me = api(MEMBER_TOKEN, "GET", "/api/v2/users/me")
    _, admin = api(ADMIN_TOKEN, "GET", "/api/v2/users/me")
    org = me["organization_ids"][0]
    _, templates = api(ADMIN_TOKEN, "GET", f"/api/v2/organizations/{org}/templates")
    template = next(t for t in templates if t["name"] == TEMPLATE)
    _, presets = api(ADMIN_TOKEN, "GET", f"/api/v2/templateversions/{template['active_version_id']}/presets")
    preset = next((p["ID"] for p in presets or [] if p["Name"] == PRESET), None)
    say(f"deployment {API}, member {me['username']}, template {TEMPLATE}, run {RUN_ID}")

    def create_request(name):
        req = {"template_id": template["id"], "name": name}
        if preset:
            req["template_version_preset_id"] = preset
        return req

    # A second member, who is above nothing.
    other_name = f"other-{RUN_ID}"
    run([ADMIN, "users", "create", "-u", other_name, "-e", f"{other_name}@example.com", "-p", "SomeSecurePassword!23"])
    other_token = run([ADMIN, "tokens", "create", "--user", other_name, "--name", "validate", "--lifetime", "1h"]).stdout.strip().splitlines()[-1]

    report.section("A person creates a delegated identity")
    agent = f"agent-{RUN_ID}"
    created = run([MEMBER, "identities", "create", agent, "-o", "json"])
    identity = json.loads(created.stdout)[0] if created.returncode == 0 else {}
    check("A1", "creating an identity returns a record under the person, with no scopes and no credential",
          identity.get("parent_id") == me["id"] and identity.get("root_user_id") == me["id"]
          and identity.get("scopes") == [] and "token" not in created.stdout.lower(),
          created.stdout.strip() or created.stderr.strip())
    s1, _ = api(other_token, "GET", f"/api/v2/identities/{identity['id']}")
    s2, _ = api(other_token, "PATCH", f"/api/v2/identities/{identity['id']}", {"scopes": ["all"]})
    s3, _ = api(other_token, "POST", "/api/v2/identities", {"parent_id": identity["id"]})
    s4, _ = api(other_token, "POST", "/api/v2/identities", {"parent_id": me["id"]})
    check("A2", "somebody who is not above the identity cannot see it, change it, or add below it or its parent",
          (s1, s2, s3, s4) == (404, 404, 403, 403), f"get {s1}, update {s2}, create below it {s3}, create below its parent {s4}")

    report.section("A workspace on behalf of the identity")
    # A workspace of the person's own, which the identity must not reach.
    s, own = api(MEMBER_TOKEN, "POST", "/api/v2/users/me/workspaces", create_request(f"mine-{RUN_ID}"))
    s, _ = api(other_token, "POST", f"/api/v2/users/{identity['id']}/workspaces", create_request(f"x-{RUN_ID}"))
    check("B1", "somebody who is not above the identity cannot create a workspace on its behalf", s in (403, 404), s)
    started = time.time()
    made = run([MEMBER, "create", f"{agent}/task-{RUN_ID}", "--template", TEMPLATE, "--preset", PRESET, "-y"])
    s, ws = api(MEMBER_TOKEN, "GET", f"/api/v2/users/{agent}/workspace/task-{RUN_ID}")
    ready = workspace_ready(MEMBER_TOKEN, ws["id"]) if s == 200 else None
    took = time.time() - started
    check("B2", "the unchanged command line creates it: coder create <identity>/<name>",
          made.returncode == 0 and s == 200, (made.stderr.strip().splitlines() or [""])[-1])
    check("B3", "the identity owns the workspace and the person is who asked",
          s == 200 and ws["owner_id"] == identity["id"] and ws["latest_build"]["initiator_id"] == me["id"],
          f"owner {ws.get('owner_name')}, initiator {ws['latest_build'].get('initiator_name')}" if s == 200 else s)
    check("B4", "its daemon connects although the identity may do nothing with the API", ready is not None,
          f"ready {took:.1f}s after the request")
    shown = run([MEMBER, "ssh", f"{agent}/task-{RUN_ID}", "--", "coder", "identities", "show", "me", "-o", "json"], timeout=120)
    inside = json.loads(shown.stdout)[0] if shown.returncode == 0 and shown.stdout.strip().startswith("[") else {}
    check("B5", "the person can enter it, and what runs in it is the identity to Coder",
          inside.get("id") == identity["id"], shown.stdout.strip()[:200] or shown.stderr.strip()[-300:])
    token = run([MEMBER, "ssh", f"{agent}/task-{RUN_ID}", "--", "printenv", "CODER_SESSION_TOKEN"], timeout=120).stdout.strip()

    report.section("What the identity may do")
    s1, _ = api(token, "GET", f"/api/v2/workspaces/{ws['id']}")
    s2, _ = api(token, "GET", "/api/v2/users/me")
    s3, _ = api(token, "POST", "/api/v2/users/me/workspaces", create_request(f"own-{RUN_ID}"))
    check("C1", "with no scopes, the credential in its workspace is worth nothing", (s1, s2) == (404, 403) or (s1 == 404 and s2 >= 400) and s3 >= 400,
          f"its workspace {s1}, its account {s2}, create a workspace {s3}")
    s, _ = api(token, "PATCH", "/api/v2/identities/me", {"scopes": ["all"]})
    check("C2", "the identity cannot raise itself", s in (403, 404), s)
    api(MEMBER_TOKEN, "PATCH", f"/api/v2/identities/{agent}", {"scopes": ["workspace:read", "template:read"]})
    s1, _ = api(token, "GET", f"/api/v2/workspaces/{ws['id']}")
    s2, _ = api(token, "GET", f"/api/v2/workspaces/{own['id']}")
    s3, listed = api(token, "GET", "/api/v2/workspaces")
    names = sorted(w["name"] for w in (listed or {}).get("workspaces", [])) if s3 == 200 else s3
    check("C3", "raising it applies to the credential it already has, and what its parent owns stays out of reach",
          (s1, s2) == (200, 404) and names == [f"task-{RUN_ID}"], f"its workspace {s1}, the person's workspace {s2}, it lists {names}")
    s, _ = api(token, "POST", f"/api/v2/workspaces/{ws['id']}/builds", {"transition": "delete"})
    check("C4", "what it was not given stays refused", s == 403, f"delete its workspace {s}")

    report.section("An identity below an identity")
    api(MEMBER_TOKEN, "PATCH", f"/api/v2/identities/{agent}", {"scopes": [
        "coder:workspaces.create", "delegated_identity:create", "delegated_identity:read", "delegated_identity:use"]})
    s1, refusal = api(token, "POST", "/api/v2/identities", {"name": f"sub-{RUN_ID}", "scopes": ["workspace:delete"]})
    s2, child = api(token, "POST", "/api/v2/identities", {"name": f"sub-{RUN_ID}", "scopes": ["workspace:read"]})
    check("D1", "an identity creates an identity below itself, and can hand on no more than it holds",
          (s1, s2) == (400, 201) and child["parent_id"] == identity["id"] and child["root_user_id"] == me["id"],
          f"more than it holds {s1}: {(refusal or {}).get('message')}; within {s2}")
    s, child_ws = api(token, "POST", f"/api/v2/users/{child['id']}/workspaces", create_request(f"subtask-{RUN_ID}"))
    child_ready = workspace_ready(MEMBER_TOKEN, child_ws["id"]) if s == 201 else None
    check("D2", "it creates a workspace on behalf of the identity below it, which the person can enter as well",
          s == 201 and child_ws["owner_id"] == child["id"] and child_ws["latest_build"]["initiator_id"] == identity["id"]
          and child_ready is not None, f"{s}, owner {child_ws.get('owner_name')}" if s == 201 else f"{s} {child_ws}")
    api(MEMBER_TOKEN, "PATCH", f"/api/v2/identities/{agent}", {"scopes": ["delegated_identity:read"]})
    s, lowered = api(MEMBER_TOKEN, "GET", f"/api/v2/identities/{child['id']}")
    check("D3", "lowering the parent lowers the identity below it at once",
          s == 200 and lowered["scopes"] == ["workspace:read"] and lowered["effective_scopes"] == [],
          f"scopes {lowered.get('scopes')}, effective {lowered.get('effective_scopes')}")

    report.section("A trusted service, as Agent Relay is")
    relay_name = f"relay-{RUN_ID}"
    role = f"relay-{RUN_ID}"
    s, _ = api(ADMIN_TOKEN, "POST", f"/api/v2/organizations/{org}/members/roles", {
        "name": role, "display_name": role, "organization_id": org,
        "site_permissions": [], "user_permissions": [],
        "organization_permissions": [{"resource_type": r, "action": a} for r, a in [
            ("organization_member", "read"),
            ("workspace", "create"), ("workspace", "read"), ("workspace", "start"), ("workspace", "update"), ("workspace", "delete"),
            ("workspace_secret", "create"),
            ("delegated_identity", "create"), ("delegated_identity", "read"),
            ("delegated_identity", "use"), ("delegated_identity", "delete")]],
    })
    made = run([ADMIN, "users", "create", "--service-account", "--username", relay_name])
    api(ADMIN_TOKEN, "PUT", f"/api/v2/organizations/{org}/members/{relay_name}/roles", {"roles": [role]})
    relay_token = run([ADMIN, "tokens", "create", "--user", relay_name, "--name", "validate", "--lifetime", "1h"]).stdout.strip().splitlines()[-1]
    s1, session = api(relay_token, "POST", "/api/v2/identities", {"name": f"session-{RUN_ID}", "parent_id": me["id"]})
    check("E1", "with permission across the organization, a service creates an identity under a member it is not above",
          s in (200, 201) and s1 == 201 and session["parent_id"] == me["id"] and session["created_by"] != me["id"],
          f"role {s}, service account {made.returncode}, identity {s1}")
    s, session_ws = api(relay_token, "POST", f"/api/v2/users/{session['id']}/workspaces", create_request(f"session-{RUN_ID}"))
    session_ready = workspace_ready(MEMBER_TOKEN, session_ws["id"]) if s == 201 else None
    check("E2", "it builds the session's workspace on behalf of the identity, and the member can enter it",
          s == 201 and session_ws["owner_id"] == session["id"] and session_ready is not None,
          f"{s}, owner {session_ws.get('owner_name')}" if s == 201 else f"{s} {session_ws}")
    s1, _ = delete_workspace(relay_token, session_ws["id"])
    s2, _ = api(relay_token, "DELETE", f"/api/v2/identities/{session['id']}")
    check("E3", "when the session ends it deletes the workspace and the identity", (s1, s2) == (201, 204), f"workspace {s1}, identity {s2}")

    report.section("Cleaning up, and what stays")
    s, _ = api(MEMBER_TOKEN, "DELETE", f"/api/v2/identities/{agent}")
    check("F1", "an identity that owns a workspace or has an identity below it cannot be deleted", s == 409, s)
    s1, _ = delete_workspace(MEMBER_TOKEN, child_ws["id"])
    s2, _ = delete_workspace(MEMBER_TOKEN, ws["id"])
    check("F2", "the person deletes the identities' workspaces on their behalf", (s1, s2) == (201, 201), f"{s1}, {s2}")
    s1, _ = api(MEMBER_TOKEN, "DELETE", f"/api/v2/identities/{child['id']}")
    s2, _ = api(MEMBER_TOKEN, "DELETE", f"/api/v2/identities/{agent}")
    s3, _ = api(token, "GET", "/api/v2/identities/me")
    check("F3", "deleting an identity ends its credentials", (s1, s2, s3) == (204, 204, 401), f"{s1}, {s2}, its token {s3}")
    s, audit = api(ADMIN_TOKEN, "GET", "/api/v2/audit?limit=50&q=resource_type:delegated_identity")
    actions = sorted({(a["action"], a["user"]["username"]) for a in audit["audit_logs"] if RUN_ID in a["resource_target"]}) if s == 200 else s
    check("F4", "the audit log names who created, changed and deleted the identities",
          s == 200 and ("create", me["username"]) in actions and ("create", agent) in actions
          and ("create", relay_name) in actions and ("delete", me["username"]) in actions, actions)
    delete_workspace(MEMBER_TOKEN, own["id"])
    _ = admin
    return 0 if report.finish() else 1


if __name__ == "__main__":
    sys.exit(main())
