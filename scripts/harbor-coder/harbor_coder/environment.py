"""Harbor environment provider that runs each trial inside a Coder workspace.

Per trial: build the task image (or take ``docker_image``), add a layer with
the static ``coder`` binary so the container can run the workspace agent,
``coder create`` a workspace from the ``harbor-task`` template, and route
exec/upload/download through ``coder ssh``. ``stop`` deletes the workspace.

Credentials come from ``CODER_URL`` and ``CODER_SESSION_TOKEN``; the template
name from ``HARBOR_CODER_TEMPLATE`` (default ``harbor-task``).
"""

from __future__ import annotations

import asyncio
import hashlib
import os
import re
import shlex
import shutil
import tempfile
from pathlib import Path
from typing import Any

from harbor.environments.base import BaseEnvironment, ExecResult
from harbor.environments.definition import (
    effective_exec_cwd,
    parse_dockerfile_workdir,
    require_agent_environment_definition,
    should_use_prebuilt_docker_image,
)
from harbor.environments.tar_transfer import extract_dir_from_bytes, pack_dir_to_bytes

from harbor_coder.client import CoderClient

_NAME_SANITIZE = re.compile(r"[^a-z0-9-]+")

_AGENT_LAYER_DOCKERFILE = """\
FROM {base}
COPY coder /usr/local/bin/coder
RUN chmod 0755 /usr/local/bin/coder && mkdir -p /logs /tests && chmod 0777 /logs /tests
"""


class CoderEnvironment(BaseEnvironment):
    def __init__(self, *args: Any, **kwargs: Any):
        super().__init__(*args, **kwargs)
        self.template = os.environ.get("HARBOR_CODER_TEMPLATE", "harbor-task")
        self.coder_binary = shutil.which("coder")
        if not self.coder_binary:
            raise ValueError("coder CLI not found on PATH")
        self.client = CoderClient()
        # Workspace names are DNS labels capped at 32 characters.
        digest = hashlib.sha256(self.session_id.encode()).hexdigest()[:6]
        base = re.sub(r"-{2,}", "-", _NAME_SANITIZE.sub("-", self.session_id.lower().replace("_", "-")))
        self.workspace_name = f"{base.strip('-')[:25].rstrip('-') or 'trial'}-{digest}"
        self.workspace_id: str | None = None
        self._dockerfile_workdir = parse_dockerfile_workdir(self.environment_dir / "Dockerfile")
        # Part of the image tag so a rebuilt coder binary (the other arm of a
        # local A/B) never reuses an image carrying the previous agent.
        with open(self.coder_binary, "rb") as f:
            self._coder_digest = hashlib.file_digest(f, "sha256").hexdigest()[:8]

    @staticmethod
    def type() -> str:
        return "coder"

    @classmethod
    def resource_capabilities(cls):
        return None

    @classmethod
    def preflight(cls) -> None:
        missing = [k for k in ("CODER_URL", "CODER_SESSION_TOKEN") if not os.environ.get(k)]
        if missing:
            raise SystemExit(f"coder environment: missing {', '.join(missing)}")
        for tool in ("coder", "docker"):
            if not shutil.which(tool):
                raise SystemExit(f"coder environment: `{tool}` not found on PATH")

    def _validate_definition(self) -> None:
        require_agent_environment_definition(
            self.environment_dir, docker_image=self.task_env_config.docker_image
        )

    async def _run_local(
        self, *argv: str, input_bytes: bytes | None = None, timeout_sec: float | None = None
    ) -> tuple[bytes, bytes, int]:
        proc = await asyncio.create_subprocess_exec(
            *argv,
            stdin=asyncio.subprocess.PIPE if input_bytes is not None else asyncio.subprocess.DEVNULL,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        try:
            out, err = await asyncio.wait_for(proc.communicate(input_bytes), timeout=timeout_sec)
        except TimeoutError:
            proc.kill()
            await proc.wait()
            raise
        return out, err, proc.returncode or 0

    async def _run_text(self, *argv: str, **kwargs: Any) -> ExecResult:
        out, err, rc = await self._run_local(*argv, **kwargs)
        return ExecResult(stdout=out.decode(errors="replace"), stderr=err.decode(errors="replace"), return_code=rc)

    async def _build_image(self, force_build: bool) -> str:
        name = _NAME_SANITIZE.sub("-", self.environment_name.lower())
        tag = f"harbor-coder/{name}:{self.environment_id[:12]}-{self._coder_digest}"
        if not force_build and (await self._run_text("docker", "image", "inspect", tag)).return_code == 0:
            return tag
        if should_use_prebuilt_docker_image(
            self.environment_dir,
            docker_image=self.task_env_config.docker_image,
            force_build=force_build,
        ):
            base = self.task_env_config.docker_image
        else:
            base = f"harbor-coder/{name}-base:{self.environment_id[:12]}"
            res = await self._run_text(
                "docker", "build", "--platform", "linux/amd64", "-t", base, str(self.environment_dir)
            )
            if res.return_code != 0:
                raise RuntimeError(f"docker build failed:\n{res.stderr}")
        with tempfile.TemporaryDirectory() as ctx:
            shutil.copy2(self.coder_binary, Path(ctx) / "coder")
            (Path(ctx) / "Dockerfile").write_text(_AGENT_LAYER_DOCKERFILE.format(base=base))
            res = await self._run_text("docker", "build", "--platform", "linux/amd64", "-t", tag, ctx)
            if res.return_code != 0:
                raise RuntimeError(f"agent layer build failed:\n{res.stderr}")
        return tag

    async def start(self, force_build: bool) -> None:
        image = await self._build_image(force_build)
        workdir = effective_exec_cwd(None, self.task_env_config.workdir, self._dockerfile_workdir) or "/"
        res = await self._run_text(
            self.coder_binary, "create", self.workspace_name,
            "--template", self.template,
            "--parameter", f"image={image}",
            "--parameter", f"workdir={workdir}",
            "--parameter", f"cpus={self._effective_cpus or 1}",
            "--parameter", f"memory_mb={self._effective_memory_mb or 2048}",
            "--yes",
        )  # fmt: skip
        if res.return_code != 0:
            raise RuntimeError(f"coder create failed:\n{res.stdout}\n{res.stderr}")
        try:
            ws = await self.client.workspace_by_name("me", self.workspace_name)
            self.workspace_id = ws["id"]
            await self.client.wait_for_agent(self.workspace_id, timeout_sec=300)
            # The agent reports connected slightly before SSH is accepted.
            for _ in range(30):
                if (await self._ssh("true", timeout_sec=30)).return_code == 0:
                    break
                await asyncio.sleep(2)
            else:
                raise RuntimeError("coder ssh never succeeded")
        except BaseException:
            # Harbor only calls stop() after a successful start().
            res = await self._run_text(self.coder_binary, "delete", self.workspace_name, "--yes")
            if res.return_code != 0:
                self.logger.warning(f"cleanup: coder delete {self.workspace_name} failed: {res.stderr}")
            raise
        await self.ensure_dirs(self._mount_targets(writable_only=True))
        await self._upload_environment_dir_after_start()

    async def stop(self, delete: bool) -> None:
        if not self.workspace_id:
            return
        verb = "delete" if delete else "stop"
        res = await self._run_text(self.coder_binary, verb, self.workspace_name, "--yes")
        if res.return_code != 0:
            raise RuntimeError(f"coder {verb} {self.workspace_name} failed: {res.stderr}")

    async def _ssh(
        self, remote: str, *, input_bytes: bytes | None = None, timeout_sec: int | None = None
    ) -> ExecResult:
        try:
            # The remote `timeout` wrapper should fire first and report cleanly.
            return await self._run_text(
                self.coder_binary, "ssh", self.workspace_name, "--", remote,
                input_bytes=input_bytes,
                timeout_sec=(timeout_sec + 30) if timeout_sec else None,
            )  # fmt: skip
        except TimeoutError:
            raise RuntimeError(f"Command timed out after {timeout_sec} seconds") from None

    async def exec(
        self,
        command: str,
        cwd: str | None = None,
        env: dict[str, str] | None = None,
        timeout_sec: int | None = None,
        user: str | int | None = None,
    ) -> ExecResult:
        cwd = effective_exec_cwd(cwd, self.task_env_config.workdir, self._dockerfile_workdir)
        parts = []
        if cwd:
            parts.append(f"cd {shlex.quote(cwd)} &&")
        if merged := self._merge_env(env):
            parts.append("env " + " ".join(f"{k}={shlex.quote(v)}" for k, v in merged.items()))
        if timeout_sec:
            parts.append(f"timeout {int(timeout_sec)}")
        parts.append(f"bash -c {shlex.quote(command)}")
        remote = " ".join(parts)
        user = self._resolve_user(user)
        if user not in (None, "root", 0, "0"):
            remote = f"su {shlex.quote(str(user))} -s /bin/bash -c {shlex.quote(remote)}"
        return await self._ssh(remote, timeout_sec=timeout_sec)

    async def upload_file(self, source_path: Path | str, target_path: str) -> None:
        parent = shlex.quote(str(Path(target_path).parent))
        res = await self._ssh(
            f"mkdir -p {parent} && cat > {shlex.quote(target_path)}",
            input_bytes=Path(source_path).read_bytes(),
        )
        if res.return_code != 0:
            raise RuntimeError(f"upload_file {target_path} failed: {res.stderr}")

    async def upload_dir(self, source_dir: Path | str, target_dir: str) -> None:
        q = shlex.quote(target_dir)
        res = await self._ssh(
            f"mkdir -p {q} && tar -xzf - -C {q}",
            input_bytes=pack_dir_to_bytes(source_dir, compress=True).getvalue(),
        )
        if res.return_code != 0:
            raise RuntimeError(f"upload_dir {target_dir} failed: {res.stderr}")

    async def _ssh_bytes(self, remote: str) -> bytes:
        out, err, rc = await self._run_local(self.coder_binary, "ssh", self.workspace_name, "--", remote)
        if rc != 0:
            raise RuntimeError(f"{remote[:40]} failed: {err.decode(errors='replace')}")
        return out

    async def download_file(self, source_path: str, target_path: Path | str) -> None:
        data = await self._ssh_bytes(f"cat {shlex.quote(source_path)}")
        target = Path(target_path)
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(data)

    async def download_dir(self, source_dir: str, target_dir: Path | str) -> None:
        data = await self._ssh_bytes(f"tar -czf - -C {shlex.quote(source_dir)} .")
        Path(target_dir).mkdir(parents=True, exist_ok=True)
        extract_dir_from_bytes(data, target_dir)
