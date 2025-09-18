#!/usr/bin/env bash

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
# shellcheck source=scripts/lib.sh
source "${SCRIPT_DIR}/lib.sh"

CODER_BIN=${CODER_BIN:-"$(which coder)"}
AGENTAPI_SLUG=${AGENTAPI_SLUG:-""}

TEMPDIR=$(mktemp -d)
trap 'rm -rf "${TEMPDIR}"' EXIT

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

ssh_config() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME

	if [[ -n "${OPENSSH_CONFIG_FILE:-}" ]]; then
		echo "Using existing SSH config file: ${OPENSSH_CONFIG_FILE}"
		return
	fi

	OPENSSH_CONFIG_FILE="${TEMPDIR}/coder-ssh.config"
	"${CODER_BIN}" \
		config-ssh \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		--ssh-config-file="${OPENSSH_CONFIG_FILE}" \
		--yes
	export OPENSSH_CONFIG_FILE
}

prompt_ssh() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME PROMPT

	ssh_config

	# Write prompt to a file in the workspace
	cat <<<"${PROMPT}" >"${TEMPDIR}/prompt.txt"
	scp \
		-F "${OPENSSH_CONFIG_FILE}" \
		"${TEMPDIR}/prompt.txt" \
		"${WORKSPACE_NAME}.coder:/tmp/prompt.txt"

	# Execute claude over SSH
	# Note: use of cat to work around claude-code#7357
	ssh \
		-F "${OPENSSH_CONFIG_FILE}" \
		"${WORKSPACE_NAME}.coder" \
		-- \
		"cat /tmp/prompt.txt | \"\${HOME}\"/.local/bin/claude --dangerously-skip-permissions --print --verbose --output-format=stream-json"
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
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME DESTINATION_PREFIX
	ssh_config

	cat >"${TEMPDIR}/archive.sh" <<-EOF
		#!/usr/bin/env bash
		set -x
		set -euo pipefail
		/tmp/coder-script-data/bin/coder-archive-create
		ARCHIVE_NAME=\$(cd && find . -maxdepth 1 -type f -name "*.tar.gz" -print0 | xargs -0 -n 1 basename)
		ARCHIVE_PATH="/home/coder/\${ARCHIVE_NAME}"
		ARCHIVE_DEST="${DESTINATION_PREFIX%%/}/\${ARCHIVE_NAME}"
		if [[ ! -f "\${ARCHIVE_PATH}" ]]; then
			echo "FATAL: Archive not found at expected path: \${ARCHIVE_PATH}"
			exit 1
		fi
		gcloud storage cp "\${ARCHIVE_PATH}" "\${ARCHIVE_DEST}"
		echo "\${ARCHIVE_DEST}"
		exit 0
	EOF

	scp -F "${OPENSSH_CONFIG_FILE}" \
		"${TEMPDIR}/archive.sh" \
		"${WORKSPACE_NAME}.coder:/tmp/archive.sh"

	ARCHIVE_DEST=$(ssh -F "${OPENSSH_CONFIG_FILE}" \
		"${WORKSPACE_NAME}.coder" \
		-- \
		"chmod +x /tmp/archive.sh && /tmp/archive.sh")

	echo "${ARCHIVE_DEST}"

	exit 0
}

commit_push() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME
	ssh_config

	# For multiple commands, upload a script and run it.
	cat >"${TEMPDIR}/commit_push.sh" <<-EOF
		#!/usr/bin/env bash
		set -euo pipefail
		if [[ \$(git branch --show-current) != "${WORKSPACE_NAME}" ]]; then
			git checkout -b ${WORKSPACE_NAME}
		fi

		if [[ -z \$(git status --porcelain) ]]; then
			echo "FATAL: No changes to commit"
			exit 1
		fi

		git add -A
		commit_msg=\$(echo -n 'You are a CLI utility that generates a commit message. Generate a concise git commit message for the currently staged changes. Print ONLY the commit message and nothing else.' | \${HOME}/.local/bin/claude --print)
		if [[ -z "\${commit_msg}" ]]; then
			commit_msg="Default commit message"
		fi
		git commit -am "\${commit_msg}"
		git push origin ${WORKSPACE_NAME}
	EOF

	scp \
		-F "${OPENSSH_CONFIG_FILE}" \
		"${TEMPDIR}/commit_push.sh" \
		"${WORKSPACE_NAME}.coder:/tmp/commit_push.sh"

	ssh \
		-F "${OPENSSH_CONFIG_FILE}" \
		"${WORKSPACE_NAME}.coder" \
		-- \
		"chmod +x /tmp/commit_push.sh && /tmp/commit_push.sh"

	exit $?
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
	commit-push)
		commit_push
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
