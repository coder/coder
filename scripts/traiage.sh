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
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME TEMPLATE_NAME TEMPLATE_PARAMETERS
	# Check if a workspace already exists
	exists=$("${CODER_BIN}" \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		list \
		--search "owner:me name:${WORKSPACE_NAME}" \
		--output json |
		jq -r '. | length')
	if [[ "${exists}" -eq "1" ]]; then
		echo "Workspace ${WORKSPACE_NAME} already exists."
		exit 0
	fi
	"${CODER_BIN}" \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		create \
		--template "${TEMPLATE_NAME}" \
		--stop-after 30m \
		--parameter "${TEMPLATE_PARAMETERS}" \
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
		--yes \
		>/dev/null 2>&1
	export OPENSSH_CONFIG_FILE
}

prompt_ssh() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME PROMPT

	ssh_config

	# Execute claude over SSH and provide prompt via stdin
	# Note: use of cat to work around claude-code#7357
	ssh \
		-F "${OPENSSH_CONFIG_FILE}" \
		"${WORKSPACE_NAME}.coder" \
		-- \
		"cat | \"\${HOME}\"/.local/bin/claude --dangerously-skip-permissions --print --verbose --output-format=stream-json" \
		<<<"${PROMPT}"
	exit 0
}

prompt_agentapi() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME AGENTAPI_SLUG PROMPT

	wait_agentapi_stable

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

	wait_agentapi_stable
}

wait_agentapi_stable() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME PROMPT
	username=$(curl \
		--fail \
		--header "Coder-Session-Token: ${CODER_SESSION_TOKEN}" \
		--location \
		--show-error \
		--silent \
		"${CODER_URL}/api/v2/users/me" | jq -r '.username')

	for attempt in {1..120}; do
		response=$(curl \
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
		echo "Waiting for AgentAPI to report stable status (attempt ${attempt}/120)"
		sleep 5
	done
}

archive() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME BUCKET_PREFIX
	ssh_config

	# We want the heredoc to be expanded locally and not remotely.
	# shellcheck disable=SC2087
	ARCHIVE_DEST=$(
		ssh -F "${OPENSSH_CONFIG_FILE}" \
			"${WORKSPACE_NAME}.coder" \
			bash <<-EOF
				#!/usr/bin/env bash
				set -euo pipefail
				ARCHIVE_PATH=\$(coder-archive-create)
				ARCHIVE_NAME=\$(basename "\${ARCHIVE_PATH}")
				ARCHIVE_DEST="${BUCKET_PREFIX%%/}/\${ARCHIVE_NAME}"
				if [[ ! -f "\${ARCHIVE_PATH}" ]]; then
					echo "FATAL: Archive not found at expected path: \${ARCHIVE_PATH}"
					exit 1
				fi
				gcloud storage cp "\${ARCHIVE_PATH}" "\${ARCHIVE_DEST}"
				echo "\${ARCHIVE_DEST}"
				exit 0
			EOF
	)

	echo "${ARCHIVE_DEST}"

	exit 0
}

summary() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME
	ssh_config

	# We want the heredoc to be expanded locally and not remotely.
	# shellcheck disable=SC2087
	ssh \
		-F "${OPENSSH_CONFIG_FILE}" \
		"${WORKSPACE_NAME}.coder" \
		-- \
		bash <<-EOF
			#!/usr/bin/env bash
			set -euo pipefail
			summary=\$(echo -n 'You are a CLI utility that generates a human-readable Markdown summary for the currently staged AND unstaged changes. Print ONLY the summary and nothing else.' | \${HOME}/.local/bin/claude --print)
			if [[ -z "\${summary}" ]]; then
				summary="Generating a summary failed. Here is a short overview:\n\$(git diff --stat)"
			fi
			echo "\${summary}"
			exit 0
		EOF
}

commit_push() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME
	ssh_config

	# We want the heredoc to be expanded locally and not remotely.
	# shellcheck disable=SC2087
	ssh \
		-F "${OPENSSH_CONFIG_FILE}" \
		"${WORKSPACE_NAME}.coder" \
		-- \
		bash <<-EOF
			#!/usr/bin/env bash
			set -euo pipefail
			BRANCH="traiage/${WORKSPACE_NAME}"
			if [[ \$(git branch --show-current) != "\${BRANCH}" ]]; then
				git checkout -b "\${BRANCH}"
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
			exit 0
		EOF

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

resume() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN WORKSPACE_NAME BUCKET_PREFIX

	# Note: WORKSPACE_NAME here is really the 'context key'.
	# Files are uploaded to the GCS bucket under this key.
	# This just happens to be the same as the workspace name.

	src="${BUCKET_PREFIX%%/}/${WORKSPACE_NAME}.tar.gz"
	dest="${TEMPDIR}/${WORKSPACE_NAME}.tar.gz"
	gcloud storage cp "${src}" "${dest}"
	if [[ ! -f "${dest}" ]]; then
		echo "FATAL: Failed to download archive from ${src}"
		exit 1
	fi

	resume_dest="${HOME}/workspaces/${WORKSPACE_NAME}"
	mkdir -p "${resume_dest}"
	tar -xzvf "${dest}" -C "${resume_dest}" || exit 1
	echo "Workspace restored to ${resume_dest}"
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
	wait-agentapi-stable)
		wait_agentapi_stable
		;;
	resume)
		resume
		;;
	summary)
		summary
		;;
	*)
		echo "Unknown option: $1"
		usage
		;;
	esac
}

main "$@"
