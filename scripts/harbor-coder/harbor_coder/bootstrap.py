"""Bootstrap a fresh coderd for Harbor evals.

Idempotently: create the first user, log in, register an Anthropic provider
and a default chat model, and push the Harbor workspace template. Prints
``KEY=VALUE`` lines for the caller to export (``CODER_URL``,
``CODER_SESSION_TOKEN``, ``HARBOR_CODER_TEMPLATE``, ``HARBOR_CODER_MODEL_CONFIG_ID``).

Usage:
    python -m harbor_coder.bootstrap --url http://localhost:3000 \
        --template-dir scripts/harbor-coder/template --model claude-haiku-4-5

Model traffic goes through an AI gateway. By default that is dogfood's
(``https://dogfood.cdr.dev/api/v2/ai-gateway/anthropic``), which accepts a
Coder API key, so no provider key is involved: set ``HARBOR_CODER_UPSTREAM_TOKEN``
to a dogfood token. Override the gateway with ``HARBOR_CODER_UPSTREAM_URL``.
"""

from __future__ import annotations

import argparse
import asyncio
import os
import subprocess
import sys
import time
from pathlib import Path

from harbor_coder.client import CoderAPIError, CoderClient

DEFAULT_UPSTREAM_URL = "https://dogfood.cdr.dev/api/v2/ai-gateway/anthropic"

FIRST_USER = {
    "email": "harbor@coder.invalid",
    "username": "harbor",
    "name": "Harbor Eval",
    "password": "HarborEval-" + os.urandom(8).hex(),
}


def _wait_healthy(url: str, timeout: float = 120) -> None:
    import urllib.request

    deadline = time.time() + timeout
    last_err: Exception | None = None
    while time.time() < deadline:
        try:
            with urllib.request.urlopen(f"{url}/healthz", timeout=5) as r:
                if r.status == 200:
                    return
        except Exception as e:  # noqa: BLE001
            last_err = e
        time.sleep(2)
    raise SystemExit(f"coderd at {url} not healthy after {timeout}s: {last_err}")


async def _login(url: str, password_file: Path) -> str:
    anon = CoderClient.__new__(CoderClient)
    anon.url = url.rstrip("/")
    anon.token = "none"

    if password_file.exists():
        FIRST_USER["password"] = password_file.read_text().strip()
    else:
        try:
            await anon.request("POST", "/api/v2/users/first", body=FIRST_USER)
        except CoderAPIError as e:
            if e.status != 409:
                raise
        password_file.parent.mkdir(parents=True, exist_ok=True)
        password_file.write_text(FIRST_USER["password"])
        password_file.chmod(0o600)

    login = await anon.request(
        "POST",
        "/api/v2/users/login",
        body={"email": FIRST_USER["email"], "password": FIRST_USER["password"]},
    )
    return login["session_token"]


async def _ensure_provider(client: CoderClient, api_key: str, base_url: str) -> str:
    try:
        existing = await client.request("GET", "/api/v2/ai/providers/anthropic")
        return existing["id"]
    except CoderAPIError as e:
        if e.status != 404:
            raise
    created = await client.request(
        "POST",
        "/api/v2/ai/providers",
        body={
            "type": "anthropic",
            "name": "anthropic",
            "display_name": "Anthropic",
            "enabled": True,
            "base_url": base_url,
            "api_keys": [api_key],
        },
    )
    return created["id"]


async def _ensure_model(
    client: CoderClient, org_id: str, provider_id: str, model: str
) -> str:
    listing = await client.request(
        "GET", f"/api/v2/organizations/{org_id}/chats/models"
    )
    for m in listing.get("models") or []:
        if m.get("model") == model and m.get("ai_provider_id") == provider_id:
            return m["id"]
    created = await client.request(
        "POST",
        f"/api/v2/organizations/{org_id}/chats/models",
        body={
            "ai_provider_id": provider_id,
            "model": model,
            "context_limit": 200_000,
            "is_default": True,
        },
    )
    return created["id"]


def _push_template(url: str, token: str, name: str, template_dir: Path) -> None:
    env = {**os.environ, "CODER_URL": url, "CODER_SESSION_TOKEN": token}
    subprocess.run(
        [
            "coder",
            "templates",
            "push",
            name,
            "--directory",
            str(template_dir),
            "--yes",
        ],
        check=True,
        env=env,
        stdout=sys.stderr,
    )


async def main() -> None:
    p = argparse.ArgumentParser()
    p.add_argument(
        "--url", default=os.environ.get("CODER_URL", "http://localhost:3000")
    )
    p.add_argument("--template-dir", required=True, type=Path)
    p.add_argument("--template-name", default="harbor-task")
    p.add_argument("--model", default="claude-haiku-4-5")
    p.add_argument(
        "--state-dir",
        type=Path,
        default=Path(os.environ.get("HARBOR_CODER_STATE_DIR", ".harbor-coder")),
    )
    args = p.parse_args()

    upstream_token = os.environ.get("HARBOR_CODER_UPSTREAM_TOKEN")
    if not upstream_token:
        raise SystemExit(
            "HARBOR_CODER_UPSTREAM_TOKEN is required (a Coder API key for the AI gateway)"
        )
    base_url = os.environ.get("HARBOR_CODER_UPSTREAM_URL", DEFAULT_UPSTREAM_URL)

    _wait_healthy(args.url)
    token = await _login(args.url, args.state_dir / "first-user-password")
    client = CoderClient(url=args.url, token=token)
    org = await client.default_organization()

    provider_id = await _ensure_provider(client, upstream_token, base_url)
    model_id = await _ensure_model(client, org["id"], provider_id, args.model)
    _push_template(args.url, token, args.template_name, args.template_dir)

    out = {
        "CODER_URL": args.url,
        "CODER_SESSION_TOKEN": token,
        "HARBOR_CODER_TEMPLATE": args.template_name,
        "HARBOR_CODER_MODEL_CONFIG_ID": model_id,
    }
    for k, v in out.items():
        print(f"{k}={v}")
    print(
        f"bootstrap: org={org['name']} provider={provider_id} model={args.model} "
        f"template={args.template_name}",
        file=sys.stderr,
    )


if __name__ == "__main__":
    asyncio.run(main())
