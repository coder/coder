#!/bin/sh
set -e

CODER="go run ./cmd/coder"
TOKEN_FILE="/bootstrap/token"
ORG_NAME="second-organization"

echo "=== Multi-Organization Setup ==="

# Load bootstrap token
CODER_SESSION_TOKEN=$(cat "$TOKEN_FILE")
if [ -z "${CODER_SESSION_TOKEN}" ]; then
	echo "Bootstrap token not found in ${TOKEN_FILE}"
	exit 1
fi
export CODER_SESSION_TOKEN

# Create second organization if it doesn't exist.
if ! $CODER organizations show "$ORG_NAME" >/dev/null 2>&1; then
	echo "Creating organization '$ORG_NAME'..."
	$CODER organizations create -y "$ORG_NAME"
else
	echo "Organization '$ORG_NAME' already exists."
fi

# Add member user to the organization.
echo "Adding member user to organization '$ORG_NAME'..."
$CODER organizations members add member --org "$ORG_NAME" 2>/dev/null ||
	echo "Member already in organization or failed to add."

# Import docker template to second org (reuse setup-template.sh).
/scripts/setup-template.sh "$ORG_NAME"

echo "=== Multi-org setup complete ==="
