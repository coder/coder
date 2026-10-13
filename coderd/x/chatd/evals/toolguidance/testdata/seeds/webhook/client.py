import argparse
import json
import sys

from config import load_settings


def build_request(event, settings):
    if not settings["token"]:
        raise SystemExit("unauthorized: no API token configured")
    return {
        "url": settings["url"],
        "headers": {"Authorization": "Bearer " + settings["token"]},
        "body": json.dumps(event),
    }


def main(argv=None):
    parser = argparse.ArgumentParser()
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args(argv)
    request = build_request({"type": "order.created", "id": 1}, load_settings())
    if args.dry_run:
        print(f"POST {request['url']} ({len(request['body'])} bytes, authorized)")
        return 0
    print("sending is disabled in this environment", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
