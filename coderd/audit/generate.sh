#!/usr/bin/env bash

# This script facilitates code generation for auditing types. It outputs code
# that can be copied and pasted into the audit.AuditableResources table. By
# default, every field is ignored. It is your responsiblity to go through each
# field and document why each field should or should not be audited.
#
# Usage:
# ./generate.sh <database type> <database type> ...


set -euo pipefail

cd "$(dirname "$0")" && cd "$(git rev-parse --show-toplevel)"
go run ./scripts/auditgen ./coderd/database "$@"
