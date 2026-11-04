#!/usr/bin/env bash
# Stops a pull request's doc-check review: drops queued messages on the
# review chat and its sub-agents, interrupts each one that is active, and
# stops the review workspace.
#
# Environment:
#   CODER_URL             Coder deployment URL.
#   CODER_TOKEN           Session token of the bot that owns the review chats.
#   PR_URL                Pull request URL, in its /pull/ or /issues/ form.
#   WORKFLOW_NAME         Workflow name. coder/agents-chat-action labels its
#                         chats with it, along with the pull request.
#   GITHUB_REPOSITORY     The repository whose review workspaces are named
#                         doc-check-pr-<number>. GitHub Actions sets it.
#   STOP_BUDGET_SECONDS   Optional. The time the script plans to take, 270
#                         by default. The chats get the first third of it,
#                         and the workspaces the rest.
#
# No poll starts after its phase ends, and each request and sleep is cut to
# the time left in its phase. A request made after the end still gets one
# second, so the script can run a few seconds past the budget.
#
# A failure prints a warning and exits 0, and the review keeps running.
# Tested by doc-check-stop-review.test.sh.

set -uo pipefail

# The default budget leaves 30 seconds of the 5 minutes GitHub gives an
# always() step after a cancel.
readonly BUDGET_SECONDS="${STOP_BUDGET_SECONDS:-270}"
readonly POLL_SECONDS=5
readonly REQUEST_SECONDS=30
readonly INTERRUPTS_PER_CHAT=3
# The most deletes in one clear_queue call. It matches the server's
# default limit of 20 queued messages a chat.
readonly DELETES_PER_CLEAR=20
readonly STARTED_AT="${SECONDS}"

# The second, counted from STARTED_AT, at which the current phase ends.
deadline=$((BUDGET_SECONDS / 3))

warn() {
	echo "::warning::$*"
}

# Prints the seconds left in the current phase, which can be zero or less.
time_left() {
	echo $((deadline - (SECONDS - STARTED_AT)))
}

# Reports whether the current phase has time left.
has_time() {
	(($(time_left) > 0))
}

# Sleeps one poll interval, or the time left in the phase if that is less.
nap() {
	local seconds
	seconds=$(time_left)
	((seconds > POLL_SECONDS)) && seconds="${POLL_SECONDS}"
	if ((seconds > 0)); then
		sleep "${seconds}"
	fi
}

api() {
	local method="$1" path="$2" max
	shift 2
	# curl --max-time 0 never times out, so a request made after the
	# deadline gets one second.
	max=$(time_left)
	((max > REQUEST_SECONDS)) && max="${REQUEST_SECONDS}"
	((max < 1)) && max=1
	curl -sS --fail-with-body --max-time "${max}" -X "${method}" \
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
# the next one and starts the chat again. Deleting one can promote the
# next, so this reads the queue again after each delete.
clear_queue() {
	local id="$1" messages qid n
	for ((n = 0; n <= DELETES_PER_CLEAR; n++)); do
		if ! has_time; then
			warn "Ran out of time to drop the queued messages on chat ${id}."
			return 0
		fi
		# queued_messages holds the whole queue on any read without a
		# cursor; limit=1 only keeps the message page small.
		messages=$(api GET "/api/v2/chats/${id}/messages?limit=1") || {
			warn "Could not read the queue of chat ${id}."
			return 0
		}
		qid=$(jq -r '.queued_messages[0].id // empty' <<<"${messages}")
		if [[ -z "${qid}" ]]; then
			return 0
		fi
		if ((n == DELETES_PER_CLEAR)); then
			break
		fi
		if ! api DELETE "/api/v2/chats/${id}/queue/${qid}" >/dev/null; then
			warn "Could not drop queued message ${qid} on chat ${id}."
			return 0
		fi
		echo "Dropped queued message ${qid} on chat ${id}."
	done
	warn "Chat ${id} still has queued messages after ${DELETES_PER_CLEAR} deletes."
}

# Reports whether any chat in a chat_tree listing is running, waiting on a
# tool result (requires_action), or finishing an interruption.
any_active() {
	grep -qE ' (running|requires_action|interrupting)$' <<<"$1"
}

# Interrupts the chat and its sub-agents until none is active. An
# interrupted chat in requires_action returns to running, and dropping a
# paused chat's queue can start it, so every poll interrupts whatever is
# running again, up to INTERRUPTS_PER_CHAT times per chat.
stop_chat() {
	local root="$1" tree id status sent read_ok=true
	local -A interrupts=()
	if ! has_time; then
		warn "Ran out of time to stop review chat ${root}. It may still be running in Coder."
		return 0
	fi
	tree=$(chat_tree "${root}") || {
		warn "Could not read review chat ${root}."
		return 0
	}
	while read -r id status <&3; do
		clear_queue "${id}"
	done 3<<<"${tree}"

	# Sleeping alone reaches the poll limit when the phase ends; slow
	# requests reach the end of the phase first.
	local poll polls=$((deadline / POLL_SECONDS))
	for ((poll = 0; poll <= polls; poll++)); do
		if ((poll > 0)); then
			if ! has_time; then
				break
			fi
			nap
		fi
		if ! tree=$(chat_tree "${root}"); then
			read_ok=false
			continue
		fi
		read_ok=true
		if ! any_active "${tree}"; then
			((${#interrupts[@]} > 0)) &&
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
	if [[ "${read_ok}" == "true" ]]; then
		warn "Review chat ${root} did not stop in time. It may still be running in Coder."
	else
		warn "Could not read review chat ${root}. It may still be running in Coder."
	fi
	return 0
}

# Stops the workspace and waits for the stop to finish, so the next run can
# start it fresh. A build in progress finishes first. The first read runs
# even after the deadline, so every workspace gets a read and, if it needs
# one, a stop.
stop_workspace() {
	local id="$1" name="" workspace transition status stop_sent=false
	local poll polls=$((BUDGET_SECONDS / POLL_SECONDS))
	for ((poll = 0; poll <= polls; poll++)); do
		if ((poll > 0)) && ! has_time; then
			break
		fi
		# A deleted workspace answers 410 Gone without include_deleted.
		workspace=$(api GET "/api/v2/workspaces/${id}?include_deleted=true") || {
			warn "Could not read workspace ${name:-${id}}."
			return 0
		}
		name=$(jq -r '.name' <<<"${workspace}")
		transition=$(jq -r '.latest_build.transition' <<<"${workspace}")
		status=$(jq -r '.latest_build.status' <<<"${workspace}")
		case "${transition}/${status}" in
		*/pending | */starting | */stopping | */canceling | */deleting)
			nap
			continue
			;;
		stop/stopped | delete/deleted)
			[[ "${stop_sent}" == "true" ]] && echo "Stopped workspace ${name}."
			return 0
			;;
		*)
			# Any other state gets a stop: a running workspace, or one whose
			# last build failed or was canceled and may have left it up. A
			# stop this run sent is not retried.
			if [[ "${stop_sent}" == "true" && "${transition}" == "start" ]]; then
				warn "A start build of workspace ${name} began after this run stopped it, and the workspace is now ${status}. The next review may run on it with an expired GitHub token."
				return 0
			fi
			if [[ "${stop_sent}" == "true" ]]; then
				warn "The ${transition} build of workspace ${name} ended with status ${status}. The next review may run on this workspace with an expired GitHub token."
				return 0
			fi
			;;
		esac
		echo "Stopping workspace ${name} (${transition} build ${status})."
		api POST "/api/v2/workspaces/${id}/builds" \
			-H "Content-Type: application/json" \
			-d '{"transition":"stop"}' >/dev/null || {
			warn "Could not stop workspace ${name}. It stops at its autostop deadline instead."
			return 0
		}
		stop_sent=true
		nap
	done
	warn "Workspace ${name:-${id}} did not finish its build within the ${BUDGET_SECONDS}-second budget."
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
	local pr_repo="${BASH_REMATCH[1]}/${BASH_REMATCH[2]}"
	local gh_target="${pr_repo}#${BASH_REMATCH[4]}"
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

	deadline="${BUDGET_SECONDS}"

	# The chats' own workspaces, plus the one named for the pull request.
	# That name has only the number, so look it up only for this
	# repository's pull requests. The name filter matches substrings, so keep
	# only the exact name.
	local this_repo="${GITHUB_REPOSITORY:-}"
	workspaces='{}'
	if [[ "${pr_repo,,}" == "${this_repo,,}" ]]; then
		workspaces=$(api GET /api/v2/workspaces -G \
			--data-urlencode "q=owner:me name:${ws_name}") || {
			warn "Could not look up workspace ${ws_name}."
			workspaces='{}'
		}
	fi
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
