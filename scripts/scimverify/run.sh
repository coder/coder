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

if ! command -v jq &>/dev/null; then
	echo "Error: jq is not installed." >&2
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
CREATE_RESPONSE=$(curl -s -w '\n%{http_code}' -X POST \
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
CREATE_STATUS=$(echo "${CREATE_RESPONSE}" | tail -n1)
CREATE_BODY=$(echo "${CREATE_RESPONSE}" | sed '$d')

# Detect SCIM-disabled / unlicensed early so the user gets an actionable
# error instead of a wall of failing tests. The route mounted in
# enterprise/coderd/scimroutes.go returns 404 ("SCIM is disabled...")
# when CODER_SCIM_AUTH_HEADER is unset, and the RequireFeatureMW middleware
# returns 403 ("SCIM is a Premium feature") when no license entitles SCIM.
case "${CREATE_STATUS}" in
404)
	echo "Error: ${BASE_URL}/Users returned 404." >&2
	echo "SCIM appears disabled. Set CODER_SCIM_AUTH_HEADER on the server and restart." >&2
	echo "Response body: ${CREATE_BODY}" >&2
	exit 1
	;;
401 | 403)
	echo "Error: ${BASE_URL}/Users returned ${CREATE_STATUS}." >&2
	echo "Either the bearer token does not match CODER_SCIM_AUTH_HEADER, or no" >&2
	echo "license entitles SCIM. Load one with 'coder licenses add'." >&2
	echo "Response body: ${CREATE_BODY}" >&2
	exit 1
	;;
201 | 200) ;;
*)
	echo "Error: ${BASE_URL}/Users returned ${CREATE_STATUS}; expected 201." >&2
	echo "Response body: ${CREATE_BODY}" >&2
	exit 1
	;;
esac

TEST_USER_ID=$(echo "${CREATE_BODY}" | jq -r '.id // ""')

if [[ -z "${TEST_USER_ID}" ]]; then
	echo "Error: Create returned ${CREATE_STATUS} but no 'id' field." >&2
	echo "Response body: ${CREATE_BODY}" >&2
	exit 1
fi

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
    # Minimal body: only userName + primary email; "active" defaults to true.
    - request:
        {
          "schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
          "userName": "scimtest3",
          "emails": [{ "value": "scimtest3@example.com", "primary": true }],
        }
    # Initial state "active": false. Coder creates the user as dormant and the
    # SCIM response reports active=false.
    - request:
        {
          "schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
          "userName": "scimtest4",
          "name": { "givenName": "SCIM", "familyName": "TestFour" },
          "emails": [{ "value": "scimtest4@example.com", "primary": true }],
          "active": false,
        }
    # Multiple emails; the primary one (second entry) is what Coder picks.
    - request:
        {
          "schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
          "userName": "scimtest5",
          "name": { "givenName": "SCIM", "familyName": "TestFive" },
          "emails":
            [
              { "value": "scimtest5-alt@example.com" },
              { "value": "scimtest5@example.com", "primary": true },
            ],
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
    # Suspend via PUT.
    - id: "${TEST_USER_ID}"
      request:
        {
          "schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
          "userName": "${TEST_USERNAME}",
          "name": { "givenName": "Updated", "familyName": "Test" },
          "emails": [{ "value": "${TEST_EMAIL}", "primary": true }],
          "active": false,
        }

  patch_tests:
    - id: "${TEST_USER_ID}"
      request:
        {
          "schemas": ["urn:ietf:params:scim:api:messages:2.0:PatchOp"],
          "Operations":
            [{ "op": "replace", "path": "active", "value": false }],
        }
    # Reactivate via PATCH using a path-less operation (whole-resource form).
    - id: "${TEST_USER_ID}"
      request:
        {
          "schemas": ["urn:ietf:params:scim:api:messages:2.0:PatchOp"],
          "Operations":
            [{ "op": "replace", "value": { "active": true } }],
        }
    # Suspend again via PATCH with an explicit path so we end up suspended
    # before the DELETE step.
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

# --- Step 3: Run scimverify ---
npx scimverify \
	--base-url "${BASE_URL}" \
	--auth-header "Bearer ${AUTH_TOKEN}" \
	--config "${CONFIG_FILE}" \
	--har-file "${OUTPUT_DIR}/output.har"

# --- Step 4: Verify SCIM recreate-after-delete behavior ---
# Coder does not hard-delete users. POSTing the same SCIM user after a
# DELETE should reactivate the existing row (same ID), not return 409 or
# create a duplicate. The scimverify suite does not test this flow, so we
# verify it directly with curl.
echo ""
echo "=== Recreate-after-delete check ==="
RECREATE_RESPONSE=$(curl -s -w '\n%{http_code}' -X POST \
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
RECREATE_STATUS=$(echo "${RECREATE_RESPONSE}" | tail -n1)
RECREATE_BODY=$(echo "${RECREATE_RESPONSE}" | sed '$d')
RECREATE_ID=$(echo "${RECREATE_BODY}" | jq -r '.id // ""')

if [[ "${RECREATE_STATUS}" == "201" || "${RECREATE_STATUS}" == "200" ]] && [[ "${RECREATE_ID}" == "${TEST_USER_ID}" ]]; then
	echo "ok - POST after DELETE reactivated existing user (${TEST_USER_ID})"
else
	echo "not ok - POST after DELETE should reactivate, got status=${RECREATE_STATUS} id=${RECREATE_ID} (expected ${TEST_USER_ID})" >&2
	echo "  body: ${RECREATE_BODY}" >&2
	exit 1
fi
