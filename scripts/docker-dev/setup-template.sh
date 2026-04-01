#!/bin/sh
set -e

CODER="go run ./cmd/coder"
TOKEN_FILE="/bootstrap/token"

# Accept optional org argument. If not provided, use the user's default org.
ORG_NAME="${1:-}"

echo "=== Setting up docker template ==="

# Load bootstrap token
CODER_SESSION_TOKEN=$(cat "$TOKEN_FILE")
if [ -z "${CODER_SESSION_TOKEN}" ]; then
	echo "Bootstrap token not found in ${TOKEN_FILE}"
	exit 1
fi
export CODER_SESSION_TOKEN

# If no org provided, get user's default org.
if [ -z "$ORG_NAME" ]; then
	ORG_NAME=$($CODER organizations show me -o json | jq -r '.[] | select(.is_default) | .name')
fi

echo "Target organization: $ORG_NAME"

# Check if template already exists in this org.
if $CODER templates versions list docker --org "$ORG_NAME" >/dev/null 2>&1; then
	echo "Docker template already exists in '$ORG_NAME'."
	exit 0
fi

# Create and push docker template.
echo "Creating docker template in '$ORG_NAME'..."
TEMPLATE_DIR="$(mktemp -d)"
$CODER templates init --id docker "$TEMPLATE_DIR"
(cd "$TEMPLATE_DIR" && terraform init)

ARCH="$(go env GOARCH)"
printf 'docker_arch: "%s"\ndocker_host: "%s"\n' \
	"$ARCH" "${DOCKER_HOST:-unix:///var/run/docker.sock}" \
	>"$TEMPLATE_DIR/params.yaml"

$CODER templates push docker \
	--directory "$TEMPLATE_DIR" \
	--variables-file "$TEMPLATE_DIR/params.yaml" \
	--yes --org "$ORG_NAME"

rm -rf "$TEMPLATE_DIR"
echo "=== Docker template setup complete ==="
