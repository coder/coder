"""Bootstrap a fresh coderd for an eval run: first user, an Anthropic provider
routed to Claude Platform on AWS and signed with the host's ambient AWS
credentials, the default chat model, and the Harbor workspace template. Then
send one chat turn through it so a credential or model problem fails here
instead of as N identical trial errors.

Prints ``KEY=VALUE`` lines (CODER_URL, CODER_SESSION_TOKEN,
HARBOR_CODER_TEMPLATE) for the caller to export.

Env: ``AWS_REGION`` and ``CLAUDE_PLATFORM_WORKSPACE_ID`` (required); the AWS
credential itself comes from the environment the way any AWS SDK finds it
(instance profile, env vars, or a role assumed earlier in the job).
"""

from __future__ import annotations

import argparse
import asyncio
import os
import sys
import time
import urllib.request
from pathlib import Path

from harbor_coder.client import CoderClient

FIRST_USER = {"email": "harbor@coder.invalid", "username": "harbor", "name": "Harbor Eval"}


async def _wait_healthy(url: str) -> None:
    deadline = time.time() + 120
    while True:
        try:
            await asyncio.to_thread(lambda: urllib.request.urlopen(f"{url}/healthz", timeout=5).close())
            return
        except Exception as e:  # noqa: BLE001
            if time.time() > deadline:
                raise SystemExit(f"coderd at {url} not healthy: {e!r}")
            await asyncio.sleep(2)


async def _smoke_chat(client: CoderClient, org_id: str) -> None:
    """One real turn through the gateway. This is the only way to verify
    ambient AWS credentials and the model id, since there is no key to curl with."""
    chat = await client.create_chat(
        {
            "organization_id": org_id,
            "client_type": "api",
            "title": "harbor bootstrap smoke",
            "content": [{"type": "text", "text": "Reply with the single word ok."}],
        }
    )
    deadline = time.time() + 180
    while True:
        cur = await client.chat(chat["id"])
        if cur.get("status") == "waiting":
            break
        if cur.get("status") == "error" or time.time() > deadline:
            raise SystemExit(f"smoke chat failed: {cur.get('status')} {cur.get('last_error')}")
        await asyncio.sleep(2)
    await client.archive_chat(chat["id"])
    print("smoke chat through Claude Platform on AWS: ok", file=sys.stderr)


async def main() -> None:
    p = argparse.ArgumentParser()
    p.add_argument("--url", default="http://localhost:3000")
    p.add_argument("--template-dir", required=True, type=Path)
    p.add_argument("--model", default="claude-sonnet-5-5")
    args = p.parse_args()

    region = (
        os.environ.get("AWS_REGION")
        or os.environ.get("AWS_DEFAULT_REGION")
        or sys.exit("AWS_REGION is required")
    )
    workspace_id = os.environ.get("CLAUDE_PLATFORM_WORKSPACE_ID") or sys.exit(
        "CLAUDE_PLATFORM_WORKSPACE_ID is required"
    )

    await _wait_healthy(args.url)
    anon = CoderClient(url=args.url, token="none")
    password = "HarborEval-" + os.urandom(8).hex()
    await anon.request("POST", "/api/v2/users/first", body={**FIRST_USER, "password": password})
    login = await anon.request(
        "POST", "/api/v2/users/login", body={"email": FIRST_USER["email"], "password": password}
    )
    client = CoderClient(url=args.url, token=login["session_token"])
    org = await client.default_organization()

    # No api_keys: the gateway signs requests with its ambient AWS credentials.
    provider = await client.request(
        "POST",
        "/api/v2/ai/providers",
        body={
            "type": "anthropic",
            "name": "anthropic",
            "display_name": "Claude Platform on AWS",
            "enabled": True,
            "base_url": f"https://aws-external-anthropic.{region}.api.aws/",
            "settings": {
                "_type": "claude_platform_aws",
                "_version": 1,
                "region": region,
                "workspace_id": workspace_id,
            },
        },
    )
    await client.request(
        "POST",
        f"/api/v2/organizations/{org['id']}/chats/models",
        body={
            "ai_provider_id": provider["id"],
            "model": args.model,
            "context_limit": 200_000,
            "is_default": True,
        },
    )
    default = await client.default_chat_model(org["id"])
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

    await _smoke_chat(client, org["id"])
    print(f"CODER_URL={args.url}\nCODER_SESSION_TOKEN={client.token}\nHARBOR_CODER_TEMPLATE=harbor-task")


if __name__ == "__main__":
    asyncio.run(main())
