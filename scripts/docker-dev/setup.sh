#!/bin/sh
set -e

# On failure, keep the container alive so the user can exec in
# to troubleshoot.
on_error() {
  echo ""
  echo "=========================================="
  echo "Setup failed! Container kept alive for troubleshooting."
  echo "Exec into this container with:"
  echo ""
  echo "  docker exec -it \$(docker ps -qf name=setup) sh"
  echo ""
  echo "=========================================="
  sleep infinity
}
trap on_error EXIT

CODER="go run ./cmd/coder"

# Create first user and log in. The session token is written to
# $HOME/.config/coderv2/session which is persisted in the
# coderv2_config volume. On subsequent runs this is a no-op
# since the session already exists.
CODERV2_DIR="${HOME}/.config/coderv2"
if [ ! -f "${CODERV2_DIR}/session" ]; then
  $CODER login http://coderd:3000 \
    --first-user-username=admin \
    --first-user-email=admin@coder.com \
    --first-user-password="SomeSecurePassword!" \
    --first-user-full-name="Admin User" \
    --first-user-trial=false
fi

# Create a regular member user (ignore if exists).
$CODER users create \
  --email=member@coder.com \
  --username=member \
  --full-name="Regular User" \
  --password="SomeSecurePassword!" || true

# Import the docker template if it doesn't already exist.
# Coderd has the Docker socket mounted, so provisioners can
# talk to Docker directly.
if ! $CODER templates versions list docker >/dev/null 2>&1; then
  echo "Importing docker template..."
  TEMPLATE_DIR="$(mktemp -d)"
  $CODER templates init --id docker "$TEMPLATE_DIR"
  (cd "$TEMPLATE_DIR" && terraform init)

  # Use the same DOCKER_HOST that coderd is configured with.
  ARCH="$(go env GOARCH)"
  printf 'docker_arch: "%s"\ndocker_host: "%s"\n' "$ARCH" "${DOCKER_HOST:-unix:///var/run/docker.sock}" \
    > "$TEMPLATE_DIR/params.yaml"

  ORG=$($CODER organizations show me -o json | jq -r '.[] | select(.is_default) | .name')
  $CODER templates push docker \
    --directory "$TEMPLATE_DIR" \
    --variables-file "$TEMPLATE_DIR/params.yaml" \
    --yes --org "$ORG"

  rm -rf "$TEMPLATE_DIR"
fi

# Clear the error trap — setup succeeded.
trap - EXIT
echo "Setup complete."
