"""Validates worker.json. Exit status: 0 valid, 2 unreadable, 3 invalid."""

import sys
import json

REQUIRED = ["name", "concurrency", "queue_url"]


def main():
    try:
        with open("worker.json") as f:
            config = json.load(f).get("worker", {})
    except (OSError, ValueError) as err:
        print(f"cannot read worker.json: {err}", file=sys.stderr)
        return 2
    missing = [key for key in REQUIRED if key not in config]
    if missing:
        print("config error: missing " + ", ".join(missing), file=sys.stderr)
        return 3
    print("config ok")
    return 0


if __name__ == "__main__":
    sys.exit(main())
