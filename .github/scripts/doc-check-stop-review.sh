#!/usr/bin/env bash
# Stops a pull request's doc-check review: drops queued messages on the
# review chat and its sub-agents, interrupts each one that is active, and
# stops the review workspace.
#
# Environment:
#   CODER_URL      Coder deployment URL.
#   CODER_TOKEN    Session token of the bot that owns the review chats.
#   PR_URL         Pull request URL, in its /pull/ or /issues/ form.
#   WORKFLOW_NAME  Workflow name. coder/agents-chat-action labels its chats
#                  with it, along with the pull request.
#
# A failure prints a warning and exits 0, and the review keeps running.
# Tested by doc-check-stop-review.test.sh.

set -uo pipefail

# The bounds keep a run under the 5 minutes GitHub gives an always() step
# after a cancel: 90 seconds for the chats and 3 minutes for a workspace
# build, polling every 5 seconds.
readonly POLL_SECONDS=5
readonly CHAT_POLLS=18
readonly WORKSPACE_POLLS=36
readonly INTERRUPTS_PER_CHAT=3
readonly REQUEST_SECONDS=30

warn() {
	echo "::warning::$*"
}

api() {
	local method="$1" path="$2"
	shift 2
	curl -sS --fail-with-body --max-time "${REQUEST_SECONDS}" -X "${method}" \
		-H "Coder-Session-Token: ${CODER_TOKEN}" \
		"$@" "${CODER_URL%/}${path}"
}

# Prints "<chat id> <status>" for the chat and each of its sub-agents.
chat_tree() {
	local chat
	chat=$(api GET "/api/v2/chats/$1") || return 1
	jq -r '(., .children[]?) | "\(.id) \(.status)"' <<<"${chat}"
}

# Drops every queued message, because finishing an interruption promotes
# the next one and starts the chat again.
clear_queue() {
	local id="$1" messages qid
	# queued_messages holds the whole queue on any read without a cursor;
	# limit=1 only keeps the message page small.
	messages=$(api GET "/api/v2/chats/${id}/messages?limit=1") || {
		warn "Could not read the queue of chat ${id}."
		return 0
	}
	for qid in $(jq -r '.queued_messages[]?.id' <<<"${messages}"); do
		if api DELETE "/api/v2/chats/${id}/queue/${qid}" >/dev/null; then
			echo "Dropped queued message ${qid} on chat ${id}."
		else
			warn "Could not drop queued message ${qid} on chat ${id}."
		fi
	done
}

# Reports whether any chat in a chat_tree listing is running, waiting on a
# tool result (requires_action), or finishing an interruption.
any_active() {
	grep -qE ' (running|requires_action|interrupting)$' <<<"$1"
}

# Interrupts the chat and its sub-agents until none is active. An
# interrupted chat in requires_action returns to running, and deleting a
# paused chat's queue can start it, so every poll interrupts whatever is
# running again, up to INTERRUPTS_PER_CHAT times per chat.
stop_chat() {
	local root="$1" tree id status poll sent
	local -A interrupts=()
	tree=$(chat_tree "${root}") || {
		warn "Could not read review chat ${root}."
		return 0
	}
	while read -r id status <&3; do
		clear_queue "${id}"
	done 3<<<"${tree}"

	for ((poll = 0; poll <= CHAT_POLLS; poll++)); do
		if ((poll > 0)); then
			sleep "${POLL_SECONDS}"
		fi
		tree=$(chat_tree "${root}") || continue
		if ! any_active "${tree}"; then
			((poll > 0 || ${#interrupts[@]} > 0)) &&
				echo "Review chat ${root} and its sub-agents are stopped."
			return 0
		fi
		while read -r id status <&3; do
			case "${status}" in
			running | requires_action) ;;
			*) continue ;;
			esac
			sent="${interrupts["${id}"]:-0}"
			if ((sent >= INTERRUPTS_PER_CHAT)); then
				continue
			fi
			interrupts["${id}"]=$((sent + 1))
			if ((sent > 0)); then
				clear_queue "${id}"
			fi
			echo "Interrupting chat ${id} (${status})."
			api POST "/api/v2/chats/${id}/interrupt" >/dev/null ||
				warn "Could not interrupt chat ${id}."
		done 3<<<"${tree}"
	done
	warn "Review chat ${root} did not stop. It keeps running in Coder."
	return 0
}

# Stops the workspace and waits for the stop to finish, so the next run can
# start it fresh. A build in progress finishes first.
stop_workspace() {
	local id="$1" name="$1" workspace transition status poll stop_sent=false
	for ((poll = 0; poll < WORKSPACE_POLLS; poll++)); do
		workspace=$(api GET "/api/v2/workspaces/${id}") || {
			warn "Could not read workspace ${name}."
			return 0
		}
		name=$(jq -r '.name' <<<"${workspace}")
		transition=$(jq -r '.latest_build.transition' <<<"${workspace}")
		status=$(jq -r '.latest_build.status' <<<"${workspace}")
		case "${transition}/${status}" in
		*/pending | */starting | */stopping | */canceling | */deleting)
			sleep "${POLL_SECONDS}"
			continue
			;;
		stop/stopped | delete/deleted)
			[[ "${stop_sent}" == "true" ]] && echo "Stopped workspace ${name}."
			return 0
			;;
		start/*)
			# A start that failed or was canceled can leave resources up,
			# so it gets a stop too.
			if [[ "${stop_sent}" == "true" ]]; then
				warn "Workspace ${name} is ${status} again after the stop."
				return 0
			fi
			;;
		*)
			warn "Workspace ${name} ended its ${transition} build ${status}."
			return 0
			;;
		esac
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
	warn "Workspace ${name} did not finish its build within $((WORKSPACE_POLLS * POLL_SECONDS)) seconds."
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
	local chats workspaces workspace_ids id

	# The same labels the action uses to find the pull request's chat. The
	# list leaves out archived chats. Stop every match, not only the one the
	# action would reuse.
	chats=$(api GET /api/v2/chats -G \
		--data-urlencode "label=coder-agents-chat-action:true" \
		--data-urlencode "label=gh-target:${gh_target}" \
		--data-urlencode "label=coder-agents-chat-action-workflow:${WORKFLOW_NAME}") || {
		warn "Could not look up the review chat for ${gh_target}."
		chats='[]'
	}
	for id in $(jq -r '.[].id' <<<"${chats}"); do
		stop_chat "${id}"
	done

	# The chats' own workspaces, plus the one named for the pull request.
	# The name filter matches substrings, so keep only the exact name.
	workspaces=$(api GET /api/v2/workspaces -G \
		--data-urlencode "q=owner:me name:${ws_name}") || {
		warn "Could not look up workspace ${ws_name}."
		workspaces='{}'
	}
	workspace_ids=$(
		{
			jq -r '.[].workspace_id // empty' <<<"${chats}"
			jq -r --arg name "${ws_name}" '.workspaces[]? | select(.name == $name) | .id' <<<"${workspaces}"
		} | sort -u
	)
	for id in ${workspace_ids}; do
		stop_workspace "${id}"
	done
	return 0
}

main "$@"
