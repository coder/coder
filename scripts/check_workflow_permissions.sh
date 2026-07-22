#!/usr/bin/env bash

# This script verifies that all GitHub Actions workflows in .github/workflows
# declare explicit top-level permissions and do not grant write permissions at
# the top level (least privilege).

set -euo pipefail

# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

python3 - <<'EOF'
import sys
import glob
import yaml

failed = False

for path in sorted(glob.glob(".github/workflows/*.yaml") + glob.glob(".github/workflows/*.yml")):
    try:
        with open(path, "r", encoding="utf-8") as f:
            data = yaml.safe_load(f)
    except Exception as e:
        print(f"ERROR: Failed to parse {path}: {e}", file=sys.stderr)
        failed = True
        continue

    if not isinstance(data, dict):
        continue

    if "permissions" not in data:
        print(f"ERROR: {path} is missing top-level 'permissions:' declaration", file=sys.stderr)
        failed = True
        continue

    perms = data["permissions"]

    if perms == "write-all":
        print(f"ERROR: {path} has top-level 'permissions: write-all'", file=sys.stderr)
        failed = True
        continue

    if isinstance(perms, dict):
        for k, v in perms.items():
            if v == "write":
                print(f"ERROR: {path} has top-level write permission '{k}: write'", file=sys.stderr)
                failed = True

if failed:
    sys.exit(1)

print("INFO : All GitHub Actions workflows have valid top-level permissions.")
EOF
