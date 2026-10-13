"""Harbor environment provider that runs each trial inside a Coder workspace.

Lifecycle per trial:

1. Build the task's ``environment/Dockerfile`` locally (or take ``docker_image``
   as-is), then append a layer that adds the ``coder`` binary so the image can
   run the workspace agent without curl or wget being present.
2. Optionally push the image to a registry when the provisioner is not on the
   same Docker host.
3. ``coder create`` a workspace from a template that takes ``image`` and
   ``workdir`` parameters, then wait for its agent to connect.
4. ``exec``/``upload``/``download`` go through ``coder ssh <workspace> -- ...``,
   with tar streams over stdin/stdout for directory transfer.
5. ``stop(delete=True)`` deletes the workspace.

Configure with ``--ek``: ``template`` (required), ``org``, ``registry``,
``coder_binary``, ``image_prefix``. Credentials come from ``CODER_URL`` and
``CODER_SESSION_TOKEN``.
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
from harbor.environments.capabilities import EnvironmentCapabilities
from harbor.environments.definition import (
    effective_exec_cwd,
    parse_dockerfile_workdir,
    require_agent_environment_definition,
    should_use_prebuilt_docker_image,
)
from harbor.environments.tar_transfer import extract_dir_from_bytes, pack_dir_to_bytes

try:
    # Marks a build failure as non-retryable; absent before Harbor 0.25.
    from harbor.environments.definition import SandboxBuildFailedError
except ImportError:  # pragma: no cover

    class SandboxBuildFailedError(RuntimeError):
        pass


from harbor_coder.client import CoderClient

# Coder workspace names are DNS-label-like and capped at 32 characters.
_WORKSPACE_NAME_MAX = 32
_NAME_SANITIZE = re.compile(r"[^a-z0-9-]+")

_AGENT_LAYER_DOCKERFILE = """\
FROM {base}
COPY coder /usr/local/bin/coder
RUN chmod 0755 /usr/local/bin/coder && mkdir -p /logs /tests && chmod 0777 /logs /tests
"""


class CoderEnvironment(BaseEnvironment):
    def __init__(
        self,
        *args: Any,
        template: str | None = None,
        org: str | None = None,
        registry: str | None = None,
        coder_binary: str | None = None,
        image_prefix: str = "harbor-coder",
        agent_connect_timeout_sec: float = 300,
        **kwargs: Any,
    ):
        super().__init__(*args, **kwargs)
        self.template = template or os.environ.get("HARBOR_CODER_TEMPLATE")
        if not self.template:
            raise ValueError(
                "CoderEnvironment needs a template: pass --ek template=<name> "
                "or set HARBOR_CODER_TEMPLATE"
            )
        self.org = org or os.environ.get("HARBOR_CODER_ORG")
        self.registry = registry or os.environ.get("HARBOR_CODER_REGISTRY")
        self.coder_binary = coder_binary or shutil.which("coder")
        if not self.coder_binary:
            raise ValueError(
                "coder CLI not found on PATH; pass --ek coder_binary=/path"
            )
        self.image_prefix = image_prefix
        self.agent_connect_timeout_sec = float(agent_connect_timeout_sec)

        self.client = CoderClient()
        self.workspace_name = self._workspace_name(self.session_id)
        self.workspace_id: str | None = None
        self.workspace_owner: str | None = None
        self.image: str | None = None
        self._dockerfile_workdir = parse_dockerfile_workdir(
            self.environment_dir / "Dockerfile"
        )

    @staticmethod
    def type() -> str:
        return "coder"

    @property
    def capabilities(self) -> EnvironmentCapabilities:
        # The workspace agent needs network access to reach coderd, so
        # no-network and allowlist modes cannot be honored.
        return EnvironmentCapabilities()

    @classmethod
    def resource_capabilities(cls):
        return None

    @classmethod
    def preflight(cls) -> None:
        missing = [
            k for k in ("CODER_URL", "CODER_SESSION_TOKEN") if not os.environ.get(k)
        ]
        if missing:
            raise SystemExit(f"coder environment: missing {', '.join(missing)}")
        if not shutil.which("coder"):
            raise SystemExit("coder environment: `coder` CLI not found on PATH")
        if not shutil.which("docker"):
            raise SystemExit("coder environment: `docker` not found on PATH")

    def _validate_definition(self) -> None:
        require_agent_environment_definition(
            self.environment_dir, docker_image=self.task_env_config.docker_image
        )

    @staticmethod
    def _workspace_name(session_id: str) -> str:
        digest = hashlib.sha256(session_id.encode()).hexdigest()[:6]
        base = _NAME_SANITIZE.sub("-", session_id.lower().replace("_", "-")).strip("-")
        base = re.sub(r"-{2,}", "-", base)
        keep = _WORKSPACE_NAME_MAX - len(digest) - 1
        base = base[:keep].rstrip("-") or "trial"
        return f"{base}-{digest}"

    # Image build

    async def _run_local(
        self, *argv: str, input_bytes: bytes | None = None
    ) -> ExecResult:
        proc = await asyncio.create_subprocess_exec(
            *argv,
            stdin=asyncio.subprocess.PIPE if input_bytes is not None else None,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        out, err = await proc.communicate(input_bytes)
        return ExecResult(
            stdout=out.decode(errors="replace"),
            stderr=err.decode(errors="replace"),
            return_code=proc.returncode or 0,
        )

    async def _build_image(self, force_build: bool) -> str:
        env_hash = self.environment_id[:12]
        name = _NAME_SANITIZE.sub("-", self.environment_name.lower())
        tag = f"{self.image_prefix}/{name}:{env_hash}"
        if self.registry:
            tag = f"{self.registry.rstrip('/')}/{tag}"

        if not force_build:
            exists = await self._run_local("docker", "image", "inspect", tag)
            if exists.return_code == 0:
                self.logger.info(f"Reusing image {tag}")
                return tag
            if self.registry:
                pulled = await self._run_local("docker", "pull", tag)
                if pulled.return_code == 0:
                    return tag

        if should_use_prebuilt_docker_image(
            self.environment_dir,
            docker_image=self.task_env_config.docker_image,
            force_build=force_build,
        ):
            base = self.task_env_config.docker_image
        else:
            base = f"{self.image_prefix}/{name}-base:{env_hash}"
            self.logger.info(f"Building task image {base} from {self.environment_dir}")
            res = await self._run_local(
                "docker",
                "build",
                "--platform",
                "linux/amd64",
                "-t",
                base,
                str(self.environment_dir),
            )
            if res.return_code != 0:
                raise SandboxBuildFailedError(f"docker build failed:\n{res.stderr}")

        with tempfile.TemporaryDirectory() as ctx:
            shutil.copy2(self.coder_binary, Path(ctx) / "coder")
            (Path(ctx) / "Dockerfile").write_text(
                _AGENT_LAYER_DOCKERFILE.format(base=base)
            )
            self.logger.info(f"Adding coder agent layer -> {tag}")
            res = await self._run_local(
                "docker", "build", "--platform", "linux/amd64", "-t", tag, ctx
            )
            if res.return_code != 0:
                raise SandboxBuildFailedError(
                    f"agent layer build failed:\n{res.stderr}"
                )

        if self.registry:
            res = await self._run_local("docker", "push", tag)
            if res.return_code != 0:
                raise SandboxBuildFailedError(f"docker push failed:\n{res.stderr}")
        return tag

    # Lifecycle

    async def start(self, force_build: bool) -> None:
        self.image = await self._build_image(force_build)
        workdir = (
            effective_exec_cwd(
                None, self.task_env_config.workdir, self._dockerfile_workdir
            )
            or "/"
        )

        argv = [
            self.coder_binary,
            "create",
            self.workspace_name,
            "--template",
            self.template,
            "--parameter",
            f"image={self.image}",
            "--parameter",
            f"workdir={workdir}",
            "--parameter",
            f"cpus={self._effective_cpus or 1}",
            "--parameter",
            f"memory_mb={self._effective_memory_mb or 2048}",
            "--yes",
        ]
        if self.org:
            argv += ["--org", self.org]
        self.logger.info(
            f"Creating workspace {self.workspace_name} from {self.template}"
        )
        # Provider downloads during terraform init hit registries that
        # return transient 5xx under concurrency; retry before failing.
        for attempt in range(3):
            res = await self._run_local(*argv)
            if res.return_code == 0:
                break
            transient = "initialize terraform" in (res.stderr + res.stdout)
            if attempt == 2 or not transient:
                raise RuntimeError(
                    f"coder create failed ({res.return_code}):\n{res.stdout}\n{res.stderr}"
                )
            self.logger.warning(
                "coder create hit a transient terraform init error, retrying"
            )
            await self._run_local(
                self.coder_binary, "delete", self.workspace_name, "--yes"
            )
            await asyncio.sleep(5 * (attempt + 1))

        me = await self.client.request("GET", "/api/v2/users/me")
        self.workspace_owner = me["username"]
        ws = await self.client.workspace_by_name("me", self.workspace_name)
        self.workspace_id = ws["id"]
        try:
            await self._wait_ready()
        except BaseException:
            # Harbor only calls stop() after a successful start(), so a
            # workspace whose agent never connects would otherwise leak.
            await self._delete_workspace(warn_only=True)
            raise

        await self.ensure_dirs(self._mount_targets(writable_only=True))
        await self._upload_environment_dir_after_start()

    async def _wait_ready(self) -> None:
        await self.client.wait_for_agent(
            self.workspace_id, self.agent_connect_timeout_sec
        )
        # The agent reports connected slightly before SSH is accepted.
        probe = None
        for _ in range(30):
            probe = await self._ssh("true", timeout_sec=30)
            if probe.return_code == 0:
                return
            await asyncio.sleep(2)
        raise RuntimeError(
            f"coder ssh never succeeded: {probe.stderr if probe else ''}"
        )

    async def _delete_workspace(self, *, warn_only: bool) -> None:
        res = await self._run_local(
            self.coder_binary, "delete", self.workspace_name, "--yes"
        )
        if res.return_code != 0:
            msg = f"coder delete {self.workspace_name} failed: {res.stderr}"
            if warn_only:
                self.logger.warning(msg)
            else:
                raise RuntimeError(msg)

    async def stop(self, delete: bool) -> None:
        if not self.workspace_id:
            return
        if delete:
            await self._delete_workspace(warn_only=False)
        else:
            res = await self._run_local(
                self.coder_binary, "stop", self.workspace_name, "--yes"
            )
            if res.return_code != 0:
                self.logger.warning(f"coder stop failed: {res.stderr}")

    # Exec

    async def _ssh(
        self,
        remote_command: str,
        *,
        input_bytes: bytes | None = None,
        timeout_sec: int | None = None,
    ) -> ExecResult:
        argv = [self.coder_binary, "ssh", self.workspace_name, "--", remote_command]
        proc = await asyncio.create_subprocess_exec(
            *argv,
            stdin=asyncio.subprocess.PIPE
            if input_bytes is not None
            else asyncio.subprocess.DEVNULL,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        try:
            # Give the remote `timeout` a head start so it reports first.
            wait = (timeout_sec + 30) if timeout_sec else None
            out, err = await asyncio.wait_for(
                proc.communicate(input_bytes), timeout=wait
            )
        except TimeoutError:
            proc.kill()
            await proc.wait()
            raise RuntimeError(
                f"Command timed out after {timeout_sec} seconds"
            ) from None
        return ExecResult(
            stdout=out.decode(errors="replace"),
            stderr=err.decode(errors="replace"),
            return_code=proc.returncode or 0,
        )

    def _wrap_command(
        self,
        command: str,
        cwd: str | None,
        env: dict[str, str] | None,
        timeout_sec: int | None,
        user: str | int | None,
    ) -> str:
        parts: list[str] = []
        if cwd:
            parts.append(f"cd {shlex.quote(cwd)} &&")
        if env:
            parts.append(
                "env " + " ".join(f"{k}={shlex.quote(v)}" for k, v in env.items())
            )
        if timeout_sec:
            parts.append(f"timeout {int(timeout_sec)}")
        parts.append(f"bash -c {shlex.quote(command)}")
        inner = " ".join(parts)
        if user not in (None, "root", 0, "0"):
            return f"su {shlex.quote(str(user))} -s /bin/bash -c {shlex.quote(inner)}"
        return inner

    async def exec(
        self,
        command: str,
        cwd: str | None = None,
        env: dict[str, str] | None = None,
        timeout_sec: int | None = None,
        user: str | int | None = None,
    ) -> ExecResult:
        cwd = effective_exec_cwd(
            cwd, self.task_env_config.workdir, self._dockerfile_workdir
        )
        wrapped = self._wrap_command(
            command, cwd, self._merge_env(env), timeout_sec, self._resolve_user(user)
        )
        return await self._ssh(wrapped, timeout_sec=timeout_sec)

    # File transfer

    async def upload_file(self, source_path: Path | str, target_path: str) -> None:
        data = Path(source_path).read_bytes()
        parent = str(Path(target_path).parent)
        res = await self._ssh(
            f"mkdir -p {shlex.quote(parent)} && cat > {shlex.quote(target_path)}",
            input_bytes=data,
        )
        if res.return_code != 0:
            raise RuntimeError(f"upload_file {target_path} failed: {res.stderr}")

    async def upload_dir(self, source_dir: Path | str, target_dir: str) -> None:
        archive = pack_dir_to_bytes(source_dir, compress=True).getvalue()
        res = await self._ssh(
            f"mkdir -p {shlex.quote(target_dir)} && tar -xzf - -C {shlex.quote(target_dir)}",
            input_bytes=archive,
        )
        if res.return_code != 0:
            raise RuntimeError(f"upload_dir {target_dir} failed: {res.stderr}")

    async def download_file(self, source_path: str, target_path: Path | str) -> None:
        res = await self._ssh_bytes(f"cat {shlex.quote(source_path)}")
        target = Path(target_path)
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(res)

    async def download_dir(self, source_dir: str, target_dir: Path | str) -> None:
        data = await self._ssh_bytes(f"tar -czf - -C {shlex.quote(source_dir)} .")
        Path(target_dir).mkdir(parents=True, exist_ok=True)
        extract_dir_from_bytes(data, target_dir)

    async def _ssh_bytes(self, remote_command: str) -> bytes:
        argv = [self.coder_binary, "ssh", self.workspace_name, "--", remote_command]
        proc = await asyncio.create_subprocess_exec(
            *argv,
            stdin=asyncio.subprocess.DEVNULL,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        out, err = await proc.communicate()
        if proc.returncode != 0:
            raise RuntimeError(
                f"remote command failed ({proc.returncode}): {err.decode(errors='replace')}"
            )
        return out
