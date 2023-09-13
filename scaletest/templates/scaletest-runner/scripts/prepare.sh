#!/bin/bash
set -euo pipefail

[[ $VERBOSE == 1 ]] && set -x

echo "Preparing scaletest workspace environment..."
touch /tmp/.scaletest_preparing

mkdir -p "${SCALETEST_RUN_DIR}"

echo "Installing prerequisites (terraform, envsubst, gcloud, jq and kubectl)..."

wget --quiet -O /tmp/terraform.zip https://releases.hashicorp.com/terraform/1.5.7/terraform_1.5.7_linux_amd64.zip
sudo unzip /tmp/terraform.zip -d /usr/local/bin
terraform --version

wget --quiet -O /tmp/envsubst "https://github.com/a8m/envsubst/releases/download/v1.2.0/envsubst-$(uname -s)-$(uname -m)"
chmod +x /tmp/envsubst
sudo mv /tmp/envsubst /usr/local/bin

echo "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] https://packages.cloud.google.com/apt cloud-sdk main" | sudo tee -a /etc/apt/sources.list.d/google-cloud-sdk.list
curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key --keyring /usr/share/keyrings/cloud.google.gpg add -
sudo apt-get update
sudo apt-get install --yes \
	google-cloud-cli \
	jq \
	kubectl

echo "Cloning coder/coder repo..."

if [[ ! -d coder ]]; then
	git clone https://github.com/coder/coder.git ~/coder
fi
(cd ~/coder && git pull)

echo "Creating coder CLI token (needed for cleanup during shutdown)..."

mkdir -p "${CODER_CONFIG_DIR}"
echo -n "${CODER_URL}" >"${CODER_CONFIG_DIR}/url"

set +x # Avoid logging the token.
# Persist configuration for shutdown script too since the
# owner token is invalidated immediately on workspace stop.
export CODER_SESSION_TOKEN=$CODER_USER_TOKEN
coder tokens delete scaletest_runner >/dev/null 2>&1 || true
# TODO(mafredri): Set TTL? This could interfere with delayed stop though.
token=$(coder tokens create --name scaletest_runner)
unset CODER_SESSION_TOKEN
echo -n "${token}" >"${CODER_CONFIG_DIR}/session"
[[ $VERBOSE == 1 ]] && set -x # Restore logging (if enabled).

echo "Preparation complete!"
