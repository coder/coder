#!/usr/bin/env bash

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
# shellcheck source=scripts/lib.sh
source "${SCRIPT_DIR}/lib.sh"

CODER_BIN=${CODER_BIN:-"$(which coder)"}
AGENTAPI_SLUG=${AGENTAPI_SLUG:-""}

[[ -n ${VERBOSE:-} ]] && set -x
set -euo pipefail

usage() {
	echo "Usage: $0 <options>"
	exit 1
}

create() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME TEMPLATE_NAME
	"${CODER_BIN}" \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		create \
		--template "${TEMPLATE_NAME}" \
		--stop-after 30m \
		--yes \
		"${WORKSPACE_NAME}"
	exit 0
}

prompt() {
	if [[ -z "${AGENTAPI_SLUG}" ]]; then
		prompt_ssh
	else
		prompt_agentapi
	fi
}

prompt_ssh() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME PROMPT

	# For sanity, use OpenSSH with coder config-ssh.
	OPENSSH_CONFIG_FILE=/tmp/coder-ssh.config
	"${CODER_BIN}" \
		config-ssh \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		--ssh-config-file="${OPENSSH_CONFIG_FILE}" \
		--yes
	trap 'rm -f /tmp/coder-ssh.config' EXIT

	# Write prompt to a file in the workspace
	ssh \
		-F /tmp/coder-ssh.config \
	"${WORKSPACE_NAME}.coder" \
		-- \
		"cat > /tmp/prompt.txt" <<<"${PROMPT}"

	# Execute claude over SSH
	# Note: use of cat to work around claude-code#7357
	ssh \
		-F /tmp/coder-ssh.config \
		"${WORKSPACE_NAME}.coder" \
		-- \
		"cat /tmp/prompt.txt | \"\${HOME}\"/.local/bin/claude --print --verbose --output-format=stream-json"
		exit 0
}

prompt_agentapi() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME AGENTAPI_SLUG PROMPT

	wait

	username=$(curl \
		--fail \
		--header "Coder-Session-Token: ${CODER_SESSION_TOKEN}" \
		--location \
		--show-error \
		--silent \
		"${CODER_URL}/api/v2/users/me" | jq -r '.username')

	payload="{
		\"content\": \"${PROMPT}\",
		\"type\": \"user\"
	}"

	response=$(curl \
		--data-raw "${payload}" \
		--fail \
		--header "Content-Type: application/json" \
		--header "Coder-Session-Token: ${CODER_SESSION_TOKEN}" \
		--location \
		--request POST \
		--show-error \
		--silent \
		"${CODER_URL}/@${username}/${WORKSPACE_NAME}/apps/${AGENTAPI_SLUG}/message" | jq -r '.ok')
		if [[ "${response}" != "true" ]]; then
			echo "Failed to send prompt"
			exit 1
		fi

		wait
}

wait() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME PROMPT
	username=$(curl \
		--fail \
		--header "Coder-Session-Token: ${CODER_SESSION_TOKEN}" \
		--location \
		--show-error \
		--silent \
		"${CODER_URL}/api/v2/users/me" | jq -r '.username')

	payload="{
		\"content\": \"${PROMPT}\",
		\"type\": \"user\"
	}"

	for attempt in {1..600}; do
		response=$(curl \
		--data-raw "${payload}" \
		--fail \
		--header "Content-Type: application/json" \
		--header "Coder-Session-Token: ${CODER_SESSION_TOKEN}" \
		--location \
		--request GET \
		--show-error \
		--silent \
		"${CODER_URL}/@${username}/${WORKSPACE_NAME}/apps/agentapi/status" | jq -r '.status')
		if [[ "${response}" == "stable" ]]; then
			echo "AgentAPI stable"
			break
		fi
		echo "Waiting for AgentAPI to report stable status (attempt ${attempt}/600)"
		sleep 1
	done
}

archive() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME
	"${CODER_BIN}" \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		ssh "${WORKSPACE_NAME}" -- /bin/bash -lc "coder-create-archive"
	exit 0
}

delete() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME
	"${CODER_BIN}" \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		delete \
		"${WORKSPACE_NAME}" \
		--yes
	exit 0
}

main() {
	dependencies coder

	if [[ $# -eq 0 ]]; then
		usage
	fi

	case "$1" in
	create)
		create
		;;
	prompt)
		prompt
		;;
	archive)
		archive
		;;
	delete)
		delete
		;;
	wait)
		wait
		;;
	*)
		echo "Unknown option: $1"
		usage
		;;
	esac
}

main "$@"
