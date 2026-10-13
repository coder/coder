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
import re
import time
from typing import Any

from harbor.agents.base import BaseAgent
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext

from harbor_coder.client import CoderAPIError, CoderClient

_TERMINAL = {"waiting", "error"}

# Commands that count as the agent verifying its own work.
_TEST_COMMAND = re.compile(
    r"\b(pytest|go test|npm test|pnpm test|yarn test|cargo test|make test|"
    r"python -m unittest|python3 -m unittest|jest|vitest)\b"
)
_EDIT_TOOLS = {"write_file", "edit_files"}
_EXEC_TOOLS = {"execute"}
_READ_TOOLS = {"read_file"}


def _summarize_behavior(messages: list[dict[str, Any]]) -> dict[str, Any]:
    """Behavioral signals a system prompt change is likely to move, even
    when pass/fail does not."""
    n_turns = n_tool_calls = n_exec = n_edit = n_read = n_test_cmds = 0
    n_tool_errors = 0
    edited_paths: set[str] = set()
    last_assistant_had_tools = False
    last_text = ""
    for m in messages:
        if m.get("role") == "tool":
            for part in m.get("content") or []:
                if part.get("type") == "tool-result" and part.get("is_error"):
                    n_tool_errors += 1
            continue
        if m.get("role") != "assistant":
            continue
        n_turns += 1
        had_tools = False
        for part in m.get("content") or []:
            ptype = part.get("type")
            if ptype == "text" and part.get("text"):
                last_text = part["text"]
            if ptype != "tool-call":
                continue
            had_tools = True
            n_tool_calls += 1
            name = part.get("tool_name") or ""
            args = part.get("args") or {}
            if name in _EXEC_TOOLS:
                n_exec += 1
                if _TEST_COMMAND.search(str(args.get("command", ""))):
                    n_test_cmds += 1
            elif name in _EDIT_TOOLS:
                n_edit += 1
                if name == "write_file" and args.get("path"):
                    edited_paths.add(args["path"])
                for f in args.get("files") or []:
                    if isinstance(f, dict) and f.get("path"):
                        edited_paths.add(f["path"])
            elif name in _READ_TOOLS:
                n_read += 1
        last_assistant_had_tools = had_tools
    # A final turn with no tool calls that asks something means the agent
    # stopped for input instead of finishing. Questions often end in a list
    # of options rather than a trailing "?", so look at the whole message.
    tail = last_text.rstrip().lower()
    asked = (not last_assistant_had_tools) and (
        tail.endswith("?") or "clarif" in tail or tail.count("?") >= 2
    )
    return {
        "n_turns": n_turns,
        "n_tool_calls": n_tool_calls,
        "n_exec": n_exec,
        "n_edit": n_edit,
        "n_read": n_read,
        "n_test_commands": n_test_cmds,
        "n_tool_errors": n_tool_errors,
        "ran_tests": n_test_cmds > 0,
        "asked_question": asked,
        "edited_paths": sorted(edited_paths),
    }


# Context only, no behavioral steering: the system prompt under test must be
# what decides scope, verification, and whether to ask or proceed.
_INSTRUCTION_TEMPLATE = """\
{instruction}

(Working directory: `{workdir}`. Relative paths above refer to it.)
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
        started = time.monotonic()
        context.metadata = {
            "chat_id": chat_id,
            "chat_url": f"{self.client.url}/agents/{chat_id}",
        }
        self.logger.info(f"Created chat {chat_id} bound to workspace {workspace_id}")

        try:
            final = await self._wait_for_turn(chat_id, context)
        finally:
            context.metadata["agent_seconds"] = round(time.monotonic() - started, 1)
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
        last_text: str | None = None
        behavior = _summarize_behavior(messages)
        for m in messages:
            usage = m.get("usage") or {}
            n_in += usage.get("input_tokens") or 0
            n_out += usage.get("output_tokens") or 0
            n_cache += (usage.get("cache_read_tokens") or 0) + (
                usage.get("cache_creation_tokens") or 0
            )
            if m.get("role") == "assistant":
                for part in m.get("content") or []:
                    if part.get("type") == "text" and part.get("text"):
                        last_text = part["text"]

        # Harbor defines n_input_tokens as including cached tokens.
        context.n_input_tokens = n_in + n_cache
        context.n_output_tokens = n_out
        context.n_cache_tokens = n_cache
        context.metadata.update(behavior)
        context.metadata.update(
            {
                "n_messages": len(messages),
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
