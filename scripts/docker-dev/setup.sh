#!/bin/sh
set -e

CODER="go run ./enterprise/cmd/coder"

# Create first user and log in. The session token is written to
# $HOME/.coderv2/session which is persisted in the coder_dev_home
# volume. On subsequent runs this is a no-op since the session
# already exists.
if [ ! -f "${HOME}/.coderv2/session" ]; then
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

echo "Setup complete."
