#!/usr/bin/env bash
# Stops a pull request's doc-check review: interrupts the review chat and
# its sub-agents, drops their queued messages, and stops the review
# workspace.
#
# doc-check.yaml runs this before a review, so a new review never queues
# behind an old one and always starts its workspace fresh, with a GitHub
# token that has not expired. It runs again after the review, so nothing
# keeps generating on the shared model once the job stops watching.
#
# Environment:
#   CODER_URL      Coder deployment URL.
#   CODER_TOKEN    Session token of the bot that owns the review chats.
#   PR_URL         Pull request URL, in its /pull/ or /issues/ form.
#   WORKFLOW_NAME  Workflow name. coder/agents-chat-action labels its chats
#                  with it, along with the pull request.
#
# A failure prints a warning and exits 0. A review this script cannot stop
# keeps running, which is how doc-check behaved before the script existed.
# Tested by doc-check-stop-review.test.sh.

set -uo pipefail

# Polls every 5 seconds: up to 1 minute per interrupt round, and 3
# minutes for a workspace build.
readonly POLL_SECONDS=5
readonly CHAT_POLLS=12
readonly WORKSPACE_POLLS=36

warn() {
	echo "::warning::$*"
}

api() {
	local method="$1" path="$2"
	shift 2
	curl -sS --fail-with-body -X "${method}" \
		-H "Coder-Session-Token: ${CODER_TOKEN}" \
		"$@" "${CODER_URL%/}${path}"
}

# Prints "<chat id> <status>" for the chat and each of its sub-agents.
chat_tree() {
	local chat
	chat=$(api GET "/api/v2/chats/$1") || return 1
	jq -r '(., .children[]?) | "\(.id) \(.status)"' <<<"${chat}"
}

# Deletes queued messages first, because finishing an interruption promotes
# the next queued message and starts the chat again.
clear_queue() {
	local id="$1" messages qid
	messages=$(api GET "/api/v2/chats/${id}/messages?limit=1") || return 0
	for qid in $(jq -r '.queued_messages[]?.id' <<<"${messages}"); do
		api DELETE "/api/v2/chats/${id}/queue/${qid}" >/dev/null 2>&1 &&
			echo "Dropped queued message ${qid} on chat ${id}."
	done
}

# Reports whether any chat in a chat_tree listing is still generating.
any_active() {
	grep -qE ' (running|requires_action|interrupting)$' <<<"$1"
}

# Interrupts the chat and its sub-agents until none is generating. An
# interrupted chat in requires_action returns to running, so this takes
# up to three rounds.
stop_chat() {
	local root="$1" tree id status round poll
	for round in 1 2 3 4; do
		tree=$(chat_tree "${root}") || {
			warn "Could not read review chat ${root}."
			return 0
		}
		while read -r id status <&3; do
			clear_queue "${id}"
		done 3<<<"${tree}"
		if ! any_active "${tree}"; then
			[[ "${round}" != 1 ]] && echo "Review chat ${root} and its sub-agents are stopped."
			return 0
		fi
		if [[ "${round}" == 4 ]]; then
			break
		fi

		while read -r id status <&3; do
			case "${status}" in
			running | requires_action)
				echo "Interrupting chat ${id} (${status})."
				api POST "/api/v2/chats/${id}/interrupt" >/dev/null ||
					warn "Could not interrupt chat ${id}."
				;;
			esac
		done 3<<<"${tree}"

		for ((poll = 0; poll < CHAT_POLLS; poll++)); do
			sleep "${POLL_SECONDS}"
			tree=$(chat_tree "${root}") || continue
			any_active "${tree}" || break
		done
	done
	warn "Review chat ${root} did not stop. It keeps running in Coder."
	return 0
}

# Stops the workspace and waits for the stop to finish, so the next run can
# start it. A build in progress finishes first.
stop_workspace() {
	local id="$1" workspace name transition status poll stop_sent=false
	for ((poll = 0; poll < WORKSPACE_POLLS; poll++)); do
		workspace=$(api GET "/api/v2/workspaces/${id}") || {
			warn "Could not read workspace ${id}."
			return 0
		}
		name=$(jq -r '.name' <<<"${workspace}")
		transition=$(jq -r '.latest_build.transition' <<<"${workspace}")
		status=$(jq -r '.latest_build.status' <<<"${workspace}")
		case "${status}" in
		pending | starting | stopping | canceling | deleting)
			sleep "${POLL_SECONDS}"
			continue
			;;
		esac
		if [[ "${transition}" != "start" || "${status}" != "running" ]]; then
			[[ "${stop_sent}" == "true" ]] && echo "Stopped workspace ${name}."
			return 0
		fi
		if [[ "${stop_sent}" == "true" ]]; then
			warn "Workspace ${name} is running again after the stop."
			return 0
		fi
		echo "Stopping workspace ${name}."
		api POST "/api/v2/workspaces/${id}/builds" \
			-H "Content-Type: application/json" \
			-d '{"transition":"stop"}' >/dev/null || {
			warn "Could not stop workspace ${name}. It stops at its autostop deadline instead."
			return 0
		}
		stop_sent=true
		sleep "${POLL_SECONDS}"
	done
	warn "Workspace ${id} did not finish its build within $((WORKSPACE_POLLS * POLL_SECONDS)) seconds."
	return 0
}

main() {
	# A value written on Windows can end in a carriage return.
	local pr_url="${PR_URL:-}"
	pr_url="${pr_url%$'\r'}"
	if [[ ! "${pr_url}" =~ ^https://github\.com/([^/]+)/([^/]+)/(pull|issues)/([0-9]+)$ ]]; then
		echo "No pull request URL. Nothing to stop."
		return 0
	fi
	local gh_target="${BASH_REMATCH[1]}/${BASH_REMATCH[2]}#${BASH_REMATCH[4]}"
	local ws_name="doc-check-pr-${BASH_REMATCH[4]}"
	local chats workspaces chat_ids workspace_ids id

	# The same labels the action uses to find the pull request's chat. Stop
	# every match, not only the one the action would reuse.
	chats=$(api GET /api/v2/chats -G \
		--data-urlencode "label=coder-agents-chat-action:true" \
		--data-urlencode "label=gh-target:${gh_target}" \
		--data-urlencode "label=coder-agents-chat-action-workflow:${WORKFLOW_NAME}") || {
		warn "Could not look up the review chat for ${gh_target}."
		chats='[]'
	}
	chat_ids=$(jq -r '.[] | select(.archived != true) | .id' <<<"${chats}")
	for id in ${chat_ids}; do
		stop_chat "${id}"
	done

	# The chats' own workspaces, plus the one named for the pull request.
	# The name filter matches substrings, so keep only the exact name.
	workspaces=$(api GET /api/v2/workspaces -G \
		--data-urlencode "q=owner:me name:${ws_name}") || workspaces='{}'
	workspace_ids=$(
		{
			jq -r '.[] | select(.archived != true) | .workspace_id // empty' <<<"${chats}"
			jq -r --arg name "${ws_name}" '.workspaces[]? | select(.name == $name) | .id' <<<"${workspaces}"
		} | sort -u
	)
	for id in ${workspace_ids}; do
		stop_workspace "${id}"
	done
	return 0
}

main "$@"
