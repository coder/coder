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

# Chat error kinds caused by the deployment or the gateway rather than by the
# agent's attempt at the task.
_INFRA_ERROR_KINDS = frozenset({"auth", "config", "usage_limit", "missing_key", "provider_disabled"})


class CoderInfraError(RuntimeError):
    """The chat failed for reasons outside the agent; ``compare`` excludes these trials."""


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
        self._model = await self.client.default_chat_model(org["id"])
        if self._model is None:
            raise RuntimeError("no default chat model configured on the coderd under test")
        self.logger.info(f"coder-agents model: {self._model}")

    async def run(self, instruction: str, environment: BaseEnvironment, context: AgentContext) -> None:
        org = await self.client.default_organization()
        chat = await self.client.create_chat(
            {
                "organization_id": org["id"],
                "workspace_id": environment.workspace_id,  # type: ignore[attr-defined]
                "title": f"harbor {self.session_id}"[:80],
                "client_type": "api",
                # The task text verbatim; the working directory comes from the
                # workspace agent's `dir`, as in a real workspace.
                "content": [{"type": "text", "text": instruction}],
            }
        )
        chat_id = chat["id"]
        context.metadata = {"chat_id": chat_id, "model": self._model, "queued_for_capacity": False}
        status = "running"
        last_error: dict[str, Any] | None = None
        try:
            while True:
                c = await self.client.chat(chat_id)
                status = c.get("status")
                # Queued time counts against Harbor's timeout; recording it
                # makes a concurrency above chatd's cap visible in results.
                if c.get("queued_for_capacity"):
                    context.metadata["queued_for_capacity"] = True
                if status in ("waiting", "error"):
                    last_error = c.get("last_error")
                    break
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
            err = last_error or {}
            if err.get("retryable") or err.get("kind") in _INFRA_ERROR_KINDS:
                raise CoderInfraError(f"chat ended in infrastructure error: {err}")
            raise RuntimeError(f"chat ended in error: {err}")

    async def _interrupt_and_settle(self, chat_id: str) -> None:
        await self.client.interrupt_chat(chat_id)
        status = None
        for _ in range(20):
            status = (await self.client.chat(chat_id)).get("status")
            if status in ("waiting", "error"):
                return
            await asyncio.sleep(1)
        # The verifier may now race a still-running tool call, and archive
        # will fail because chatd only archives idle chats.
        self.logger.warning(f"chat {chat_id} still {status!r} 20s after interrupt")

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
