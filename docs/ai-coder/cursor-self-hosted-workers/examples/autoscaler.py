#!/usr/bin/env python3
"""Cursor Self-Hosted Pool autoscaler for Coder.

Reads Cursor's fleet summary and pending-request queue, computes a
desired worker count, and prints the Coder workspace API call needed
to converge. Reference implementation; wire it into your own control
loop or run on a cron.

Reads:
- GET https://api.cursor.com/v0/private-workers/summary
  (teamSummary.totalConnected, teamSummary.inUse). Primary signal,
  matches Cursor's documented autoscale pattern.
- GET https://api.cursor.com/v0/private-workers/pending-requests
  (requests[] length = queue depth). Optional, only polled when
  utilization is high. Rate-limited to 600 req/h per team.

Acts on:
- POST {coder_url}/api/v2/users/{bot_user}/workspaces  (scale up)
- POST {coder_url}/api/v2/workspaces/{id}/builds {transition:delete}
  (scale down)

Coder prebuilds give you a baseline warm pool sized in the template
(coder_workspace_preset.prebuilds.instances). This autoscaler manages
burst capacity on top of that baseline by creating ad-hoc workspaces
owned by a system bot user.

Required env vars:
  CURSOR_SA_KEY          team service-account key, agent:* scope
  CODER_URL              e.g. https://dev.coder.com
  CODER_SESSION_TOKEN    admin or template-admin PAT
  TEMPLATE_NAME          e.g. cursor-workers
  BOT_USERNAME           Coder user that owns autoscaled workspaces
  MIN_WARM               minimum baseline workers (default 2)
  MAX_WORKERS            hard cap (default 20)
  BURST_BUFFER           headroom fraction (default 0.5)
  POLL_INTERVAL_SEC      sleep between cycles (default 30)
  DRY_RUN                if "1", log decisions and exit (default "0")
"""

import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

CURSOR_API = "https://api.cursor.com"


def env_int(name: str, default: int) -> int:
    return int(os.environ.get(name, str(default)))


def env_float(name: str, default: float) -> float:
    return float(os.environ.get(name, str(default)))


def http(method: str, url: str, headers=None, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(
        url, method=method, data=data, headers=headers or {})
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            raw = r.read()
            return json.loads(raw) if raw else None
    except urllib.error.HTTPError as e:
        sys.stderr.write(f"{method} {url} -> {e.code} {e.read().decode()}\n")
        raise


def cursor_summary(sa_key: str):
    return http("GET", f"{CURSOR_API}/v0/private-workers/summary",
                headers={"Authorization": f"Bearer {sa_key}"})


def cursor_pending(sa_key: str):
    return http("GET", f"{CURSOR_API}/v0/private-workers/pending-requests",
                headers={"Authorization": f"Bearer {sa_key}"})


def coder_workspaces(coder_url: str, token: str, template_name: str):
    q = urllib.parse.quote(f"template:{template_name}")
    return http("GET", f"{coder_url}/api/v2/workspaces?q={q}",
                headers={"Coder-Session-Token": token})


def coder_create_workspace(coder_url: str, token: str, bot_user: str,
                           template_id: str, name: str):
    return http("POST", f"{coder_url}/api/v2/users/{bot_user}/workspaces",
                headers={"Coder-Session-Token": token,
                         "Content-Type": "application/json"},
                body={"name": name, "template_id": template_id})


def coder_delete_workspace(coder_url: str, token: str, workspace_id: str):
    return http("POST", f"{coder_url}/api/v2/workspaces/{workspace_id}/builds",
                headers={"Coder-Session-Token": token,
                         "Content-Type": "application/json"},
                body={"transition": "delete"})


def decide(summary, pending_count, min_warm, max_workers, burst_buffer):
    team = summary.get("teamSummary", {}) or {}
    total = team.get("totalConnected", 0)
    in_use = team.get("inUse", 0)
    idle = max(total - in_use, 0)

    # Active load is what's running plus what's queued. Headroom keeps
    # a fresh worker available when the next session arrives instead
    # of letting it queue.
    active_load = in_use + pending_count
    desired = max(min_warm, int(active_load + active_load * burst_buffer))
    desired = min(desired, max_workers)
    return {
        "total": total,
        "in_use": in_use,
        "idle": idle,
        "queue": pending_count,
        "desired": desired,
        "delta": desired - total,
    }


def main():
    sa_key = os.environ["CURSOR_SA_KEY"]
    coder_url = os.environ["CODER_URL"].rstrip("/")
    coder_token = os.environ["CODER_SESSION_TOKEN"]
    template_name = os.environ.get("TEMPLATE_NAME", "cursor-workers")
    bot_user = os.environ.get("BOT_USERNAME", "cursor-pool-autoscaler")
    min_warm = env_int("MIN_WARM", 2)
    max_workers = env_int("MAX_WORKERS", 20)
    burst_buffer = env_float("BURST_BUFFER", 0.5)
    poll_seconds = env_int("POLL_INTERVAL_SEC", 30)
    dry_run = os.environ.get("DRY_RUN", "0") == "1"

    while True:
        try:
            summary = cursor_summary(sa_key)
            team = (summary or {}).get("teamSummary", {}) or {}
            total = team.get("totalConnected", 0)
            in_use = team.get("inUse", 0)
            utilization = in_use / total if total else 0.0

            # Poll the queue only when utilization is high. The
            # pending-requests endpoint is rate-limited; the summary
            # endpoint isn't, so we treat utilization as the cheap
            # primary signal.
            pending_count = 0
            if utilization >= 0.8 or total == 0:
                pending = cursor_pending(sa_key)
                pending_count = len(pending.get("requests", []) or [])

            d = decide(summary, pending_count, min_warm, max_workers,
                       burst_buffer)
            d["utilization"] = round(utilization, 2)
            print(json.dumps({"ts": int(time.time()), **d}))

            if d["delta"] > 0:
                print(f"  scale up by {d['delta']}: POST {coder_url}/api/v2/users/{bot_user}/workspaces")
                if not dry_run:
                    # Resolve template id once per cycle.
                    org = http("GET", f"{coder_url}/api/v2/users/{bot_user}",
                               headers={"Coder-Session-Token": coder_token})
                    org_id = org["organization_ids"][0]
                    templates = http(
                        "GET",
                        f"{coder_url}/api/v2/organizations/{org_id}/templates",
                        headers={"Coder-Session-Token": coder_token})
                    template = next(
                        t for t in templates if t["name"] == template_name)
                    for i in range(d["delta"]):
                        name = f"autoscale-{int(time.time())}-{i}"
                        coder_create_workspace(coder_url, coder_token,
                                               bot_user, template["id"], name)
                        print(f"  created {name}")

            elif d["delta"] < 0 and d["idle"] > 0:
                excess = min(-d["delta"], d["idle"])
                print(f"  scale down by {excess}: delete oldest idle "
                      f"bot-owned workspaces beyond MIN_WARM")
                if not dry_run:
                    ws = coder_workspaces(coder_url, coder_token,
                                          template_name)
                    candidates = sorted(
                        (w for w in ws.get("workspaces", [])
                         if w.get("owner_name") == bot_user
                         and w.get("latest_build", {}).get("status") ==
                         "running"),
                        key=lambda w: w.get("last_used_at", ""))
                    for w in candidates[:excess]:
                        coder_delete_workspace(coder_url, coder_token,
                                               w["id"])
                        print(f"  deleted {w['name']}")
            else:
                print("  no action")

        except Exception as e:
            print(f"error: {e}", file=sys.stderr)

        if dry_run:
            return
        time.sleep(poll_seconds)


if __name__ == "__main__":
    main()
