#!/usr/bin/env bash

# Run the SCIM Verify compliance test suite against a running Coder instance.
#
# This script creates a temporary test user, generates a config with the real
# user ID (avoiding scimverify's "AUTO" which targets the first user, typically
# the admin), runs the full test suite, and cleans up.
#
# Usage:
#   ./scripts/scimverify/run.sh [--base-url URL] [--token TOKEN]
#
# Environment variables:
#   SCIM_BASE_URL    Base URL of the SCIM endpoint (default: http://localhost:3000/scim/v2)
#   SCIM_AUTH_TOKEN  Bearer token for SCIM authentication (required)
#
# Prerequisites:
#   - Node.js / npx available on PATH
#   - A running Coder instance with SCIM enabled (CODER_SCIM_AUTH_HEADER set)
#
# The tool outputs TAP-format test results to stdout and writes a HAR file
# capturing all request/response pairs for debugging.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BASE_CONFIG="${SCRIPT_DIR}/config.yaml"
OUTPUT_DIR="${SCRIPT_DIR}/output"

# Defaults
BASE_URL="${SCIM_BASE_URL:-http://localhost:3000/scim/v2}"
AUTH_TOKEN="${SCIM_AUTH_TOKEN:-}"

# Parse CLI args (override env vars)
while [[ $# -gt 0 ]]; do
	case "$1" in
	--base-url)
		BASE_URL="$2"
		shift 2
		;;
	--token)
		AUTH_TOKEN="$2"
		shift 2
		;;
	--help | -h)
		echo "Usage: $0 [--base-url URL] [--token TOKEN]"
		echo ""
		echo "Environment variables:"
		echo "  SCIM_BASE_URL    Base URL (default: http://localhost:3000/scim/v2)"
		echo "  SCIM_AUTH_TOKEN  Bearer token (required)"
		exit 0
		;;
	*)
		echo "Unknown option: $1" >&2
		exit 1
		;;
	esac
done

if [[ -z "${AUTH_TOKEN}" ]]; then
	echo "Error: SCIM auth token is required." >&2
	echo "Set SCIM_AUTH_TOKEN or pass --token TOKEN." >&2
	exit 1
fi

if ! command -v npx &>/dev/null; then
	echo "Error: npx is not installed. Install Node.js to continue." >&2
	exit 1
fi

mkdir -p "${OUTPUT_DIR}"

AUTH_HEADER="Authorization: Bearer ${AUTH_TOKEN}"

# --- Step 1: Create a sacrificial test user for PUT/PATCH/DELETE tests ---
# scimverify's "id: AUTO" resolves to the first user from GET /Users?count=1,
# which is usually the admin. Instead, we pre-create a test user and hardcode
# its UUID into a temporary config.

RAND_SUFFIX="$(head -c 4 /dev/urandom | od -An -tx1 | tr -d ' \n')"
TEST_USERNAME="scimverify-${RAND_SUFFIX}"
TEST_EMAIL="${TEST_USERNAME}@scimverify.test"

echo "=== SCIM Verify ==="
echo "Base URL:  ${BASE_URL}"
echo "Test user: ${TEST_USERNAME}"
echo "HAR out:   ${OUTPUT_DIR}/output.har"
echo ""

echo "Creating test user for PUT/PATCH/DELETE..."
CREATE_RESPONSE=$(curl -s -X POST \
	-H "${AUTH_HEADER}" \
	-H "Content-Type: application/scim+json" \
	"${BASE_URL}/Users" \
	-d "{
		\"schemas\": [\"urn:ietf:params:scim:schemas:core:2.0:User\"],
		\"userName\": \"${TEST_USERNAME}\",
		\"name\": {\"givenName\": \"Verify\", \"familyName\": \"Test\"},
		\"emails\": [{\"value\": \"${TEST_EMAIL}\", \"primary\": true}],
		\"active\": true
	}")

TEST_USER_ID=$(echo "${CREATE_RESPONSE}" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || true)

if [[ -z "${TEST_USER_ID}" ]]; then
	echo "Warning: Failed to create test user. PUT/PATCH/DELETE tests will use AUTO (may target admin)." >&2
	echo "Response: ${CREATE_RESPONSE}" >&2
	echo ""
	# Fall back to the base config (empty PUT/PATCH/DELETE)
	CONFIG_FILE="${BASE_CONFIG}"
else
	echo "Created test user: ${TEST_USERNAME} (${TEST_USER_ID})"
	echo ""

	# --- Step 2: Generate a temporary config with the real user ID ---
	CONFIG_FILE=$(mktemp "${OUTPUT_DIR}/config-XXXXXX.yaml")
	trap 'rm -f "${CONFIG_FILE}"' EXIT

	cat >"${CONFIG_FILE}" <<EOF
# Auto-generated config for scimverify. Test user: ${TEST_USERNAME} (${TEST_USER_ID})
detectSchema: true
detectResourceTypes: true
verifyPagination: false
verifySorting: false
requireAuthentication: true

users:
  enabled: true
  operations:
    - GET
    - POST
    - PUT
    - PATCH
    - DELETE

  post_tests:
    - request:
        {
          "schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
          "userName": "scimtest1",
          "name": { "givenName": "SCIM", "familyName": "TestOne" },
          "emails": [{ "value": "scimtest1@example.com", "primary": true }],
          "active": true,
        }
    - request:
        {
          "schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
          "userName": "scimtest2",
          "name": { "givenName": "SCIM", "familyName": "TestTwo" },
          "emails": [{ "value": "scimtest2@example.com", "primary": true }],
          "active": true,
        }

  put_tests:
    - id: "${TEST_USER_ID}"
      request:
        {
          "schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
          "userName": "${TEST_USERNAME}",
          "name": { "givenName": "Updated", "familyName": "Test" },
          "emails": [{ "value": "${TEST_EMAIL}", "primary": true }],
          "active": true,
        }

  patch_tests:
    - id: "${TEST_USER_ID}"
      request:
        {
          "schemas": ["urn:ietf:params:scim:api:messages:2.0:PatchOp"],
          "Operations":
            [{ "op": "replace", "path": "active", "value": false }],
        }

  delete_tests:
    - id: "${TEST_USER_ID}"

groups:
  enabled: false
EOF
fi

# --- Step 3: Run scimverify ---
npx scimverify \
	--base-url "${BASE_URL}" \
	--auth-header "Bearer ${AUTH_TOKEN}" \
	--config "${CONFIG_FILE}" \
	--har-file "${OUTPUT_DIR}/output.har"
