"""Harbor agent that drives Coder Agents (the coderd chat agent).

This is an "external" Harbor agent: it runs in the Harbor host process and
talks to coderd over REST. It requires ``CoderEnvironment`` so that the chat
can be bound to the trial's workspace; the agent's tool calls then land in the
container that Harbor's verifier inspects.

Configure with ``--ak``: ``system_prompt`` (extra per-chat system text),
``model_config_id``, ``reasoning_effort``, ``poll_sec``.
"""

from __future__ import annotations

import asyncio
import json
import os
from typing import Any

from harbor.agents.base import BaseAgent
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext

from harbor_coder.client import CoderAPIError, CoderClient

_TERMINAL = {"waiting", "error"}

_INSTRUCTION_TEMPLATE = """\
You are being evaluated on a task inside the workspace attached to this chat.
Work only in that workspace. The task's working directory is `{workdir}`; \
relative paths in the task refer to it. Do not ask clarifying questions: make \
reasonable assumptions and complete the task end to end, then stop.

# Task

{instruction}
"""


class CoderAgentsAgent(BaseAgent):
    def __init__(
        self,
        *args: Any,
        system_prompt: str | None = None,
        model_config_id: str | None = None,
        reasoning_effort: str | None = None,
        poll_sec: float = 3.0,
        **kwargs: Any,
    ):
        super().__init__(*args, **kwargs)
        self.system_prompt = system_prompt
        self.model_config_id = model_config_id
        self.reasoning_effort = reasoning_effort
        self.poll_sec = float(poll_sec)
        self.client = CoderClient(
            url=self._get_env("CODER_URL"), token=self._get_env("CODER_SESSION_TOKEN")
        )
        self._build_info: dict[str, Any] | None = None

    @staticmethod
    def name() -> str:
        return "coder-agents"

    def version(self) -> str | None:
        if self._build_info:
            return self._build_info.get("version")
        return None

    @classmethod
    def preflight(cls, kwargs=None, env=None) -> None:
        super().preflight(kwargs, env)
        merged = {**os.environ, **(env or {})}
        missing = [k for k in ("CODER_URL", "CODER_SESSION_TOKEN") if not merged.get(k)]
        if missing:
            raise SystemExit(f"coder-agents: missing {', '.join(missing)}")

    async def setup(self, environment: BaseEnvironment) -> None:
        self._build_info = await self.client.request("GET", "/api/v2/buildinfo")
        if not getattr(environment, "workspace_id", None):
            raise RuntimeError(
                "coder-agents requires the coder environment "
                "(--env harbor_coder.environment:CoderEnvironment)"
            )

    async def run(
        self, instruction: str, environment: BaseEnvironment, context: AgentContext
    ) -> None:
        workspace_id = environment.workspace_id  # type: ignore[attr-defined]
        workdir = (
            getattr(environment.task_env_config, "workdir", None)
            or getattr(environment, "_dockerfile_workdir", None)
            or "/"
        )
        org = await self.client.default_organization()

        body: dict[str, Any] = {
            "organization_id": org["id"],
            "workspace_id": workspace_id,
            "title": f"harbor {self.session_id}"[:80],
            "client_type": "api",
            "labels": {"harbor": "true", "harbor_session": self.session_id or ""},
            "content": [
                {
                    "type": "text",
                    "text": _INSTRUCTION_TEMPLATE.format(
                        workdir=workdir, instruction=instruction
                    ),
                }
            ],
        }
        if self.system_prompt:
            body["system_prompt"] = self.system_prompt
        if self.model_config_id:
            body["model_config_id"] = self.model_config_id
        if self.reasoning_effort:
            body["reasoning_effort"] = self.reasoning_effort

        chat = await self.client.create_chat(body)
        chat_id = chat["id"]
        context.metadata = {
            "chat_id": chat_id,
            "chat_url": f"{self.client.url}/agents/{chat_id}",
        }
        self.logger.info(f"Created chat {chat_id} bound to workspace {workspace_id}")

        try:
            final = await self._wait_for_turn(chat_id, context)
        finally:
            # Always collect what exists so a timeout still yields usage data.
            await self._collect(chat_id, context)
            try:
                await self.client.archive_chat(chat_id)
            except CoderAPIError as e:
                self.logger.warning(f"archive chat failed: {e}")

        if final.get("status") == "error":
            err = final.get("last_error") or {}
            context.metadata["error"] = err
            raise RuntimeError(f"chat ended in error: {err.get('message')}")

    async def _wait_for_turn(
        self, chat_id: str, context: AgentContext
    ) -> dict[str, Any]:
        """Poll until the chat finishes its turn. Harbor enforces the timeout
        around run(); on cancellation we interrupt the chat so the agent stops
        mutating the workspace before the verifier runs."""
        try:
            while True:
                chat = await self.client.chat(chat_id)
                status = chat.get("status")
                if status in _TERMINAL:
                    context.metadata["final_status"] = status
                    return chat
                await asyncio.sleep(self.poll_sec)
        except asyncio.CancelledError:
            context.metadata["final_status"] = "timeout"
            try:
                await asyncio.shield(self.client.interrupt_chat(chat_id))
            except Exception as e:  # noqa: BLE001
                self.logger.warning(f"interrupt chat failed: {e}")
            raise

    async def _collect(self, chat_id: str, context: AgentContext) -> None:
        try:
            messages = await self.client.chat_messages(chat_id)
        except Exception as e:  # noqa: BLE001
            self.logger.warning(f"fetching messages failed: {e}")
            messages = []

        n_in = n_out = n_cache = 0
        tool_calls = 0
        last_text: str | None = None
        for m in messages:
            usage = m.get("usage") or {}
            n_in += usage.get("input_tokens") or 0
            n_out += usage.get("output_tokens") or 0
            n_cache += (usage.get("cache_read_tokens") or 0) + (
                usage.get("cache_creation_tokens") or 0
            )
            if m.get("role") == "assistant":
                for part in m.get("content") or []:
                    if part.get("type") == "tool-call":
                        tool_calls += 1
                    elif part.get("type") == "text" and part.get("text"):
                        last_text = part["text"]

        # Harbor defines n_input_tokens as including cached tokens.
        context.n_input_tokens = n_in + n_cache
        context.n_output_tokens = n_out
        context.n_cache_tokens = n_cache
        context.metadata.update(
            {
                "n_messages": len(messages),
                "n_tool_calls": tool_calls,
                "final_assistant_text": (last_text or "")[:2000],
            }
        )

        # Cost comes from AI Gateway and only counts finished interceptions,
        # so it can trail the status change by a moment.
        for _ in range(5):
            try:
                cost = await self.client.chat_cost(chat_id)
            except Exception as e:  # noqa: BLE001
                self.logger.warning(f"fetching cost failed: {e}")
                break
            if cost.get("request_count", 0) > 0 or n_in == 0:
                context.cost_usd = (cost.get("total_cost_micros") or 0) / 1_000_000
                context.metadata["unpriced_requests"] = cost.get(
                    "unpriced_request_count", 0
                )
                break
            await asyncio.sleep(2)

        self.logs_dir.mkdir(parents=True, exist_ok=True)
        (self.logs_dir / "chat.json").write_text(
            json.dumps(
                {"chat_id": chat_id, "messages": messages}, indent=2, default=str
            )
        )
