#!/usr/bin/env bash

set -euo pipefail

if [[ $# -lt 1 ]]; then
	echo "Usage: $0 <coder URL>"
	exit 1
fi

# Allow toggling verbose output
[[ -n ${VERBOSE:-} ]] && set -x

CODER_URL=$1
CONFIG_DIR="${PWD}/.coderv2"
ARCH="$(arch)"
if [[ "$ARCH" == "x86_64" ]]; then
	ARCH="amd64"
fi
PLATFORM="$(uname | tr '[:upper:]' '[:lower:]')"

mkdir -p "${CONFIG_DIR}"
echo "Fetching Coder CLI for first-time setup!"
curl -fsSLk "${CODER_URL}/bin/coder-${PLATFORM}-${ARCH}" -o "${CONFIG_DIR}/coder"
chmod +x "${CONFIG_DIR}/coder"

set +o pipefail
RANDOM_ADMIN_PASSWORD=$(tr </dev/urandom -dc _A-Z-a-z-0-9 | head -c16)
set -o pipefail
CODER_FIRST_USER_EMAIL="admin@coder.com"
CODER_FIRST_USER_USERNAME="coder"
CODER_FIRST_USER_PASSWORD="${RANDOM_ADMIN_PASSWORD}"
CODER_FIRST_USER_TRIAL="false"
echo "Running login command!"
"${CONFIG_DIR}/coder" login "${CODER_URL}" \
	--global-config="${CONFIG_DIR}" \
	--first-user-username="${CODER_FIRST_USER_USERNAME}" \
	--first-user-email="${CODER_FIRST_USER_EMAIL}" \
	--first-user-password="${CODER_FIRST_USER_PASSWORD}" \
	--first-user-trial=false

echo "Writing credentials to ${CONFIG_DIR}/coder.env"
cat <<EOF >"${CONFIG_DIR}/coder.env"
CODER_FIRST_USER_EMAIL=admin@coder.com
CODER_FIRST_USER_USERNAME=coder
CODER_FIRST_USER_PASSWORD="${RANDOM_ADMIN_PASSWORD}"
CODER_FIRST_USER_TRIAL="${CODER_FIRST_USER_TRIAL}"
EOF

echo "Importing kubernetes template"
"${CONFIG_DIR}/coder" templates create --global-config="${CONFIG_DIR}" \
	--directory "${CONFIG_DIR}/templates/kubernetes" --yes kubernetes
