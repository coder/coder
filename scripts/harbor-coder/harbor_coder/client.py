"""Minimal coderd REST client shared by the environment and the agent.

Uses only the standard library so the package stays installable next to
Harbor without extra dependencies. Authentication is the ``Coder-Session-Token``
header; ``CODER_URL`` and ``CODER_SESSION_TOKEN`` come from the process
environment, matching the ``coder`` CLI.
"""

from __future__ import annotations

import asyncio
import json
import os
import urllib.error
import urllib.parse
import urllib.request
from typing import Any


class CoderAPIError(RuntimeError):
    def __init__(self, status: int, method: str, path: str, body: str):
        super().__init__(f"{method} {path} -> {status}: {body[:500]}")
        self.status = status


class CoderClient:
    def __init__(self, url: str | None = None, token: str | None = None):
        self.url = (url or os.environ.get("CODER_URL", "")).rstrip("/")
        self.token = token or os.environ.get("CODER_SESSION_TOKEN", "")
        if not self.url:
            raise ValueError("CODER_URL is required")
        if not self.token:
            raise ValueError("CODER_SESSION_TOKEN is required")

    def _request(
        self,
        method: str,
        path: str,
        body: Any | None = None,
        query: dict[str, Any] | None = None,
        timeout: float = 60,
    ) -> Any:
        url = f"{self.url}{path}"
        if query:
            url += "?" + urllib.parse.urlencode({k: v for k, v in query.items() if v is not None})
        data = None
        headers = {"Coder-Session-Token": self.token, "Accept": "application/json"}
        if body is not None:
            data = json.dumps(body).encode()
            headers["Content-Type"] = "application/json"
        req = urllib.request.Request(url, data=data, method=method, headers=headers)
        try:
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                raw = resp.read()
        except urllib.error.HTTPError as e:
            raise CoderAPIError(e.code, method, path, e.read().decode(errors="replace")) from None
        if not raw:
            return None
        return json.loads(raw)

    async def request(self, method: str, path: str, **kwargs: Any) -> Any:
        return await asyncio.to_thread(self._request, method, path, **kwargs)

    # Organizations

    async def default_organization(self) -> dict[str, Any]:
        orgs = await self.request("GET", "/api/v2/organizations")
        for org in orgs:
            if org.get("is_default"):
                return org
        return orgs[0]

    # Workspaces

    async def workspace(self, workspace_id: str) -> dict[str, Any]:
        return await self.request("GET", f"/api/v2/workspaces/{workspace_id}")

    async def workspace_by_name(self, owner: str, name: str) -> dict[str, Any]:
        return await self.request("GET", f"/api/v2/users/{owner}/workspace/{name}")

    async def wait_for_agent(
        self, workspace_id: str, timeout_sec: float, poll_sec: float = 2.0
    ) -> dict[str, Any]:
        """Poll until the workspace's first agent reports ``connected``."""
        loop = asyncio.get_running_loop()
        deadline = loop.time() + timeout_sec
        last = "unknown"
        while True:
            ws = await self.workspace(workspace_id)
            build = ws.get("latest_build", {})
            job = build.get("job", {})
            if job.get("status") in ("failed", "canceled"):
                raise RuntimeError(f"workspace build {job['status']}: {job.get('error')}")
            for res in build.get("resources", []) or []:
                for agent in res.get("agents", []) or []:
                    last = agent.get("status", "unknown")
                    if last == "connected":
                        return agent
            if loop.time() >= deadline:
                raise TimeoutError(
                    f"workspace agent not connected after {timeout_sec}s (last status: {last})"
                )
            await asyncio.sleep(poll_sec)

    # Chats

    async def default_chat_model(self, org_id: str) -> str | None:
        models = await self.request("GET", f"/api/v2/organizations/{org_id}/chats/models")
        return next((m["model"] for m in models.get("models") or [] if m.get("is_default")), None)

    async def create_chat(self, body: dict[str, Any]) -> dict[str, Any]:
        return await self.request("POST", "/api/v2/chats", body=body)

    async def chat(self, chat_id: str) -> dict[str, Any]:
        return await self.request("GET", f"/api/v2/chats/{chat_id}")

    async def chat_messages(self, chat_id: str) -> list[dict[str, Any]]:
        """Return all visible messages in ascending id order."""
        # Without after_id the server pages newest-first, so walk backwards
        # with before_id; after_id would stop after the newest page.
        out: list[dict[str, Any]] = []
        before_id: int | None = None
        while True:
            page = await self.request(
                "GET",
                f"/api/v2/chats/{chat_id}/messages",
                query={"before_id": before_id, "limit": 200},
            )
            msgs = page.get("messages") or []
            out.extend(msgs)
            if not msgs or not page.get("has_more"):
                break
            before_id = min(m["id"] for m in msgs)
        out.sort(key=lambda m: m["id"])
        return out

    async def chat_cost(self, chat_id: str) -> dict[str, Any]:
        return await self.request("GET", f"/api/v2/chats/{chat_id}/cost")

    async def interrupt_chat(self, chat_id: str) -> None:
        await self.request("POST", f"/api/v2/chats/{chat_id}/interrupt")

    async def archive_chat(self, chat_id: str) -> None:
        await self.request("PATCH", f"/api/v2/chats/{chat_id}", body={"archived": True})
