"""Set or reset the deployment-wide Coder Agents system prompt.

Lets you A/B prompt text against one running coderd without rebuilding:
write the candidate prompt to a file, apply it, run the suite under a new
``--job-name``, then reset and run the baseline.

    python -m harbor_coder.prompt --file prompt.md       # replace the default
    python -m harbor_coder.prompt --show                 # print current config
    python -m harbor_coder.prompt --reset                # back to the built-in

Replacing (not appending) is what ``include_default_system_prompt=false``
does; the per-chat ``system_prompt`` field only appends, which is why the
agent's ``--ak system_prompt`` option has weak influence.

Chats snapshot the deployment prompt at creation, so apply before ``harbor run``.
"""

from __future__ import annotations

import argparse
import asyncio
import sys
from pathlib import Path

from harbor_coder.client import CoderClient

PATH = "/api/v2/chats/config/system-prompt"


async def main() -> int:
    p = argparse.ArgumentParser()
    g = p.add_mutually_exclusive_group(required=True)
    g.add_argument(
        "--file", type=Path, help="prompt text that replaces the built-in default"
    )
    g.add_argument(
        "--append-file", type=Path, help="prompt text appended to the built-in default"
    )
    g.add_argument("--reset", action="store_true")
    g.add_argument("--show", action="store_true")
    args = p.parse_args()

    client = CoderClient()
    if args.show:
        cur = await client.request("GET", PATH)
        print(
            f"include_default_system_prompt={cur['include_default_system_prompt']}\n"
            f"custom prompt ({len(cur['system_prompt'])} chars):\n{cur['system_prompt']}"
        )
        return 0

    if args.reset:
        body = {"system_prompt": "", "include_default_system_prompt": True}
    elif args.file:
        body = {
            "system_prompt": args.file.read_text(),
            "include_default_system_prompt": False,
        }
    else:
        body = {
            "system_prompt": args.append_file.read_text(),
            "include_default_system_prompt": True,
        }
    await client.request("PUT", PATH, body=body)
    cur = await client.request("GET", PATH)
    print(
        f"ok: include_default={cur['include_default_system_prompt']} "
        f"custom={len(cur['system_prompt'])} chars",
        file=sys.stderr,
    )
    return 0


if __name__ == "__main__":
    sys.exit(asyncio.run(main()))
