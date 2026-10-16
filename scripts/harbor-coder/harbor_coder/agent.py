"""Harbor agent that drives Coder Agents (the coderd chat agent).

Runs in the Harbor host process and talks to coderd over REST. Requires
``CoderEnvironment`` so the chat can be bound to the trial's workspace; the
agent's tool calls then land in the container Harbor's verifier inspects.
"""

from __future__ import annotations

import asyncio
import json
from typing import Any

from harbor.agents.base import BaseAgent
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext

from harbor_coder.client import CoderAPIError, CoderClient


class CoderAgentsAgent(BaseAgent):
    def __init__(self, *args: Any, **kwargs: Any):
        super().__init__(*args, **kwargs)
        self.client = CoderClient()
        self._version: str | None = None
        self._model: str | None = None

    @staticmethod
    def name() -> str:
        return "coder-agents"

    def version(self) -> str | None:
        return self._version

    async def setup(self, environment: BaseEnvironment) -> None:
        self._version = (await self.client.request("GET", "/api/v2/buildinfo")).get("version")
        if not getattr(environment, "workspace_id", None):
            raise RuntimeError("coder-agents requires --env harbor_coder.environment:CoderEnvironment")
        org = await self.client.default_organization()
        models = await self.client.request("GET", f"/api/v2/organizations/{org['id']}/chats/models")
        self._model = next((m["model"] for m in models.get("models") or [] if m.get("is_default")), None)
        if self._model is None:
            raise RuntimeError("no default chat model configured on the coderd under test")
        self.logger.info(f"coder-agents model: {self._model}")

    async def run(self, instruction: str, environment: BaseEnvironment, context: AgentContext) -> None:
        workdir = (
            environment.task_env_config.workdir or getattr(environment, "_dockerfile_workdir", None) or "/"
        )
        org = await self.client.default_organization()
        chat = await self.client.create_chat(
            {
                "organization_id": org["id"],
                "workspace_id": environment.workspace_id,  # type: ignore[attr-defined]
                "title": f"harbor {self.session_id}"[:80],
                "client_type": "api",
                # Context only: the system prompt under test decides behavior.
                "content": [
                    {
                        "type": "text",
                        "text": f"{instruction}\n\n(Working directory: `{workdir}`. Relative paths above refer to it.)",
                    }
                ],
            }
        )
        chat_id = chat["id"]
        context.metadata = {"chat_id": chat_id, "model": self._model}
        status = "running"
        try:
            while (status := (await self.client.chat(chat_id)).get("status")) not in ("waiting", "error"):
                await asyncio.sleep(3)
        except asyncio.CancelledError:
            # Harbor's timeout: stop the agent so it cannot keep mutating the
            # workspace while the verifier runs, and wait for it to actually
            # stop since interrupt is asynchronous server-side.
            await asyncio.shield(self._interrupt_and_settle(chat_id))
            status = "timeout"
            raise
        finally:
            context.metadata["final_status"] = status
            await asyncio.shield(self._collect(chat_id, context))
            try:
                await asyncio.shield(self.client.archive_chat(chat_id))
            except CoderAPIError as e:
                self.logger.warning(f"archive chat {chat_id}: {e}")
        if status == "error":
            raise RuntimeError(f"chat ended in error: {(await self.client.chat(chat_id)).get('last_error')}")

    async def _interrupt_and_settle(self, chat_id: str) -> None:
        await self.client.interrupt_chat(chat_id)
        for _ in range(20):
            if (await self.client.chat(chat_id)).get("status") in ("waiting", "error"):
                return
            await asyncio.sleep(1)

    async def _collect(self, chat_id: str, context: AgentContext) -> None:
        messages = await self.client.chat_messages(chat_id)
        n_in = n_out = n_cache = 0
        for m in messages:
            u = m.get("usage") or {}
            n_in += u.get("input_tokens") or 0
            n_out += u.get("output_tokens") or 0
            n_cache += (u.get("cache_read_tokens") or 0) + (u.get("cache_creation_tokens") or 0)
        # Harbor defines n_input_tokens as including cached tokens.
        context.n_input_tokens = n_in + n_cache
        context.n_cache_tokens = n_cache
        context.n_output_tokens = n_out
        context.metadata["n_tool_calls"] = sum(
            1 for m in messages for p in m.get("content") or [] if p.get("type") == "tool-call"
        )
        # Cost only counts finished gateway interceptions, so it may lag the
        # status change by a moment.
        for _ in range(5):
            cost = await self.client.chat_cost(chat_id)
            if cost.get("request_count", 0) > 0 or n_in == 0:
                context.cost_usd = (cost.get("total_cost_micros") or 0) / 1_000_000
                break
            await asyncio.sleep(2)
        self.logs_dir.mkdir(parents=True, exist_ok=True)
        (self.logs_dir / "chat.json").write_text(json.dumps(messages, indent=1, default=str))
