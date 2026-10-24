"""Bootstrap a fresh coderd for an eval run: first user, AI provider pointed
at dogfood's gateway, default chat model, and the Harbor workspace template.
Prints ``KEY=VALUE`` lines (CODER_URL, CODER_SESSION_TOKEN, HARBOR_CODER_TEMPLATE)
for the caller to export.

Env: ``HARBOR_CODER_UPSTREAM_TOKEN`` (a Coder API key for the gateway, required),
``HARBOR_CODER_UPSTREAM_URL`` (default: dogfood's Anthropic gateway).
"""

from __future__ import annotations

import argparse
import asyncio
import json
import os
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

from harbor_coder.client import CoderAPIError, CoderClient

DEFAULT_UPSTREAM_URL = "https://dogfood.cdr.dev/api/v2/ai-gateway/anthropic"
FIRST_USER = {"email": "harbor@coder.invalid", "username": "harbor", "name": "Harbor Eval"}


def _probe_upstream(token: str, base_url: str) -> None:
    """One-token request so a bad token fails here, not as N identical trial errors."""
    req = urllib.request.Request(
        f"{base_url.rstrip('/')}/v1/messages",
        data=json.dumps(
            {"model": "claude-haiku-4-5", "max_tokens": 1, "messages": [{"role": "user", "content": "hi"}]}
        ).encode(),
        headers={
            "Authorization": f"Bearer {token}",
            "anthropic-version": "2023-06-01",
            "content-type": "application/json",
        },
    )
    try:
        urllib.request.urlopen(req, timeout=60).close()
    except urllib.error.HTTPError as e:
        raise SystemExit(
            f"upstream probe: {base_url} returned {e.code}: {e.read().decode(errors='replace')[:200]}"
        )


async def main() -> None:
    p = argparse.ArgumentParser()
    p.add_argument("--url", default="http://localhost:3000")
    p.add_argument("--template-dir", required=True, type=Path)
    p.add_argument("--model", default="claude-haiku-4-5")
    p.add_argument("--state-dir", type=Path, default=Path(".harbor-coder"))
    args = p.parse_args()

    upstream_token = os.environ.get("HARBOR_CODER_UPSTREAM_TOKEN") or sys.exit(
        "HARBOR_CODER_UPSTREAM_TOKEN is required"
    )
    upstream_url = os.environ.get("HARBOR_CODER_UPSTREAM_URL", DEFAULT_UPSTREAM_URL)
    _probe_upstream(upstream_token, upstream_url)

    deadline = time.time() + 120
    while True:
        try:
            await asyncio.to_thread(lambda: urllib.request.urlopen(f"{args.url}/healthz", timeout=5).close())
            break
        except Exception:  # noqa: BLE001
            if time.time() > deadline:
                raise SystemExit(f"coderd at {args.url} not healthy")
            await asyncio.sleep(2)

    anon = CoderClient(url=args.url, token="none")
    pw_file = args.state_dir / "first-user-password"
    if pw_file.exists():
        password = pw_file.read_text()
    else:
        password = "HarborEval-" + os.urandom(8).hex()
        await anon.request("POST", "/api/v2/users/first", body={**FIRST_USER, "password": password})
        pw_file.parent.mkdir(parents=True, exist_ok=True)
        pw_file.write_text(password)
        pw_file.chmod(0o600)
    login = await anon.request(
        "POST", "/api/v2/users/login", body={"email": FIRST_USER["email"], "password": password}
    )
    client = CoderClient(url=args.url, token=login["session_token"])
    org = await client.default_organization()

    try:
        provider = await client.request("GET", "/api/v2/ai/providers/anthropic")
        # Replace the stored key: the token is short-lived, and a rerun
        # against an existing coderd must not keep a stale one.
        await client.request(
            "PATCH",
            f"/api/v2/ai/providers/{provider['id']}",
            body={"base_url": upstream_url, "api_keys": [{"api_key": upstream_token}]},
        )
    except CoderAPIError as e:
        if e.status != 404:
            raise
        provider = await client.request(
            "POST",
            "/api/v2/ai/providers",
            body={
                "type": "anthropic",
                "name": "anthropic",
                "display_name": "Anthropic",
                "enabled": True,
                "base_url": upstream_url,
                "api_keys": [upstream_token],
            },
        )
    models_path = f"/api/v2/organizations/{org['id']}/chats/models"
    models = (await client.request("GET", models_path)).get("models") or []
    existing = next((m for m in models if m.get("model") == args.model), None)
    if existing is None:
        await client.request(
            "POST",
            models_path,
            body={
                "ai_provider_id": provider["id"],
                "model": args.model,
                "context_limit": 200_000,
                "is_default": True,
            },
        )
    elif not existing.get("is_default"):
        # Creating a model only becomes the default when it is the first one;
        # a rerun with a different model must move the default explicitly.
        await client.request("PATCH", f"{models_path}/{existing['id']}", body={"is_default": True})
    models = (await client.request("GET", models_path)).get("models") or []
    default = next((m["model"] for m in models if m.get("is_default")), None)
    if default != args.model:
        raise SystemExit(f"default chat model is {default!r}, expected {args.model!r}")
    print(f"default chat model: {default}", file=sys.stderr)
    push = await asyncio.create_subprocess_exec(
        "coder", "templates", "push", "harbor-task", "--directory", str(args.template_dir), "--yes",
        env={**os.environ, "CODER_URL": args.url, "CODER_SESSION_TOKEN": client.token},
        stdout=sys.stderr,
    )  # fmt: skip
    if await push.wait() != 0:
        raise SystemExit("coder templates push failed")
    print(f"CODER_URL={args.url}\nCODER_SESSION_TOKEN={client.token}\nHARBOR_CODER_TEMPLATE=harbor-task")


if __name__ == "__main__":
    asyncio.run(main())
