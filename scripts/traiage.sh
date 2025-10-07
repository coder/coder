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
	requiredenvs CODER_URL CODER_SESSION_TOKEN CODER_USERNAME TASK_NAME TEMPLATE_NAME TEMPLATE_PRESET PROMPT
	# Check if a task already exists
	set +e
	task_json=$("${CODER_BIN}" \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		exp tasks status "${CODER_USERNAME}/${TASK_NAME}" \
		--output json)
	set -e

	if [[ "${TASK_NAME}" == $(jq -r '.name' <<<"${task_json}") ]]; then
		echo "Task \"${CODER_USERNAME}/${TASK_NAME}\" already exists. Sending prompt to existing task."
		prompt
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
		--owner "${CODER_USERNAME}" \
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
	requiredenvs CODER_URL CODER_SESSION_TOKEN TASK_NAME PROMPT

	${CODER_BIN} \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		exp tasks status "${TASK_NAME}" \
		--watch >/dev/null

	${CODER_BIN} \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		exp tasks send "${TASK_NAME}" \
		--stdin \
		<<<"${PROMPT}"

	${CODER_BIN} \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		exp tasks status "${TASK_NAME}" \
		--watch >/dev/null

	last_message
}

last_message() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN TASK_NAME PROMPT

	last_msg_json=$(
		${CODER_BIN} \
			--url "${CODER_URL}" \
			--token "${CODER_SESSION_TOKEN}" \
			exp tasks logs "${TASK_NAME}" \
			--output json
	)
	last_output_msg=$(jq -r 'last(.[] | select(.type=="output")) | .content' <<<"${last_msg_json}")
	# HACK: agentapi currently doesn't split multiple messages, so you can end up with tool
	# call responses in the output.
	last_msg=$(tac <<<"${last_output_msg}" | sed '/^● /q' | tr -d '●' | tac)
	echo "${last_msg}"
}

wait_agentapi_stable() {
	requiredenvs CODER_URL CODER_SESSION_TOKEN TASK_NAME

	${CODER_BIN} \
		--url "${CODER_URL}" \
		--token "${CODER_SESSION_TOKEN}" \
		exp tasks status "${TASK_NAME}" \
		--watch
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
