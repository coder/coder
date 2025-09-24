#!/usr/bin/env bash

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
# shellcheck source=scripts/lib.sh
source "${SCRIPT_DIR}/lib.sh"

CODER_BIN=${CODER_BIN:-"$(which coder)"}
APP_SLUG=${APP_SLUG:-""}

TEMPDIR=$(mktemp -d)
trap 'rm -rf "${TEMPDIR}"' EXIT

[[ -n ${VERBOSE:-} ]] && set -x
set -euo pipefail

usage() {
	echo "Usage: $0 <options>"
	exit 1
}

create() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN TASK_NAME TEMPLATE_NAME TEMPLATE_PRESET PROMPT
	# Check if a task already exists
	if "${CODER_BIN}" \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		exp tasks status "${TASK_NAME}" >/dev/null 2>&1; then
		# TODO(Cian): Send PROMPT to the agent in the existing workspace.
		echo "Task \"${TASK_NAME}\" already exists."
		exit 0
	fi

	"${CODER_BIN}" \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		exp tasks create \
		--name "${TASK_NAME}" \
		--template "${TEMPLATE_NAME}" \
		--preset "${TEMPLATE_PRESET}" \
		--org coder \
		--stdin <<<"${PROMPT}"
	exit 0
}

ssh_config() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN TASK_NAME

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

prompt() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN TASK_NAME APP_SLUG PROMPT

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
		"${CODER_URL}/@${username}/${TASK_NAME}/apps/${APP_SLUG}/message" | jq -r '.ok')
	if [[ "${response}" != "true" ]]; then
		echo "Failed to send prompt"
		exit 1
	fi

	# Wait for agentapi to process the response and return the last agent message
	wait

	last_msg=$(curl \
		--header "Content-Type: application/json" \
		--header "Coder-Session-Token: ${CODER_SESSION_TOKEN}" \
		"${CODER_URL}/@${username}/${TASK_NAME}/apps/${APP_SLUG}/messages" |
		jq -r 'last(.messages[] | select(.role=="agent") | [.])')
	echo "${last_msg}"
}

wait() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN TASK_NAME APP_SLUG
	username=$(curl \
		--fail \
		--header "Coder-Session-Token: ${CODER_SESSION_TOKEN}" \
		--location \
		--show-error \
		--silent \
		"${CODER_URL}/api/v2/users/me" | jq -r '.username')

	set +o pipefail
	for attempt in {1..120}; do
		# First wait for task to start
		task_status=$("${CODER_BIN}" \
			--url "${CODER_URL}" \
			--token "${CODER_SESSION_TOKEN}" \
			exp tasks status "${TASK_NAME}" \
			--output json |
			jq -r '.status')
		echo "Task status is ${task_status}"
		if [[ "${task_status}" != "running" ]]; then
			echo "Waiting for task status to be running (attempt ${attempt}/120)"
			sleep 5
			continue
		fi

		# Workspace agent must be healthy
		healthy=$("${CODER_BIN}" \
			--url "${CODER_URL}" \
			--token "${CODER_SESSION_TOKEN}" \
			exp tasks status "${TASK_NAME}" \
			--output json |
			jq -r '.workspace_agent_health.healthy')
		if [[ "${healthy}" == "true" ]]; then
			echo "Workspace agent is healthy"
		else
			echo "Workspace agent not yet healthy (attempt ${attempt}/120)"
			sleep 5
			continue
		fi

		# AgentAPI application should not be 404'ing
		agentapi_app_status_code=$(curl \
			--header "Content-Type: application/json" \
			--header "Coder-Session-Token: ${CODER_SESSION_TOKEN}" \
			--location \
			--request GET \
			--show-error \
			--silent \
			--output /dev/null \
			--write-out '%{http_code}' \
			"${CODER_URL}/@${username}/${TASK_NAME}/apps/${APP_SLUG}/status")
		echo "Workspace app ${APP_SLUG} returned ${agentapi_app_status_code}"
		if [[ "${agentapi_app_status_code}" != "200" ]]; then
			echo "Agentapi not yet running (attempt ${attempt}/120)"
			sleep 5
			continue
		fi

		# AgentAPI must be stable
		agentapi_status=$(curl \
			--header "Content-Type: application/json" \
			--header "Coder-Session-Token: ${CODER_SESSION_TOKEN}" \
			--location \
			--request GET \
			--show-error \
			--silent \
			"${CODER_URL}/@${username}/${TASK_NAME}/apps/${APP_SLUG}/status" | jq -r '.status')
		if [[ "${agentapi_status}" == "stable" ]]; then
			echo "AgentAPI stable"
		else
			echo "Waiting for AgentAPI to report stable status (attempt ${attempt}/120)"
			sleep 5
			continue
		fi
		exit 0
	done
	set -o pipefail
}

archive() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN TASK_NAME BUCKET_PREFIX
	ssh_config

	# We want the heredoc to be expanded locally and not remotely.
	# shellcheck disable=SC2087
	ARCHIVE_DEST=$(
		ssh -F "${OPENSSH_CONFIG_FILE}" \
			"${TASK_NAME}.coder" \
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
	requiredenvs CODER_URL CODER_SESSION_TOKEN TASK_NAME APP_SLUG
	ssh_config

	# We want the heredoc to be expanded locally and not remotely.
	# shellcheck disable=SC2087
	ssh \
		-F "${OPENSSH_CONFIG_FILE}" \
		"${TASK_NAME}.coder" \
		-- \
		bash <<-EOF
			#!/usr/bin/env bash
			set -eu
			summary=\$(echo -n 'You are a CLI utility that generates a human-readable Markdown summary for the currently staged AND unstaged changes. Print ONLY the summary and nothing else.' | \${HOME}/.local/bin/claude --print)
			if [[ -z "\${summary}" ]]; then
				echo "Generating a summary failed."
				echo "Here is a short overview of the changes:"
				echo
				echo ""
				echo "\$(git diff --stat)"
				echo ""
				exit 0
			fi
			echo "\${summary}"
			exit 0
		EOF
}

commit_push() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN TASK_NAME
	ssh_config

	# We want the heredoc to be expanded locally and not remotely.
	# shellcheck disable=SC2087
	ssh \
		-F "${OPENSSH_CONFIG_FILE}" \
		"${TASK_NAME}.coder" \
		-- \
		bash <<-EOF
			#!/usr/bin/env bash
			set -euo pipefail
			BRANCH="traiage/${TASK_NAME}"
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

# TODO(Cian): Update this to delete the task when available.
delete() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN TASK_NAME
	"${CODER_BIN}" \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		delete \
		"${TASK_NAME}" \
		--yes
	exit 0
}

resume() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN TASK_NAME BUCKET_PREFIX

	# Note: TASK_NAME here is really the 'context key'.
	# Files are uploaded to the GCS bucket under this key.
	# This just happens to be the same as the workspace name.

	src="${BUCKET_PREFIX%%/}/${TASK_NAME}.tar.gz"
	dest="${TEMPDIR}/${TASK_NAME}.tar.gz"
	gcloud storage cp "${src}" "${dest}"
	if [[ ! -f "${dest}" ]]; then
		echo "FATAL: Failed to download archive from ${src}"
		exit 1
	fi

	resume_dest="${HOME}/tasks/${TASK_NAME}"
	mkdir -p "${resume_dest}"
	tar -xzvf "${dest}" -C "${resume_dest}" || exit 1
	echo "Task context restored to ${resume_dest}"
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
