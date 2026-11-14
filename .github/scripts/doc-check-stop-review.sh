#!/usr/bin/env bash
# Stops a pull request's doc-check review: drops queued messages on the
# review chat and its sub-agents, interrupts each one that is running, and
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
# The chat phase runs first, so a review gets its interrupts, and has
# usually gone idle, before its workspace stops. A phase's deadline decides
# whether a poll starts, not when its sweep ends, so a slow deployment can
# push the script past the budget.
#
# Needs bash 4.4 or later. A failure prints a warning and exits 0, and the
# review keeps running. Tested by doc-check-stop-review.test.sh.

set -uo pipefail

# The default budget leaves 30 seconds of the 5 minutes GitHub gives an
# always() step after a cancel.
readonly BUDGET_SECONDS="${STOP_BUDGET_SECONDS:-270}"
readonly CHAT_PHASE_SECONDS=$((BUDGET_SECONDS / 3))
readonly POLL_SECONDS=5
readonly REQUEST_SECONDS=30
readonly INTERRUPTS_PER_CHAT=3
# The most deletes in one clear_queue call. It matches the server's
# default limit of 20 queued messages a chat.
readonly DELETES_PER_CLEAR=20

now() {
	date +%s
}

STARTED_AT=$(now)
readonly STARTED_AT
# The second, counted from STARTED_AT, at which the current phase ends.
deadline=0

# State that sweeps keep between polls. chat_interrupts,
# chat_interrupt_failed, and chat_queue_warned are keyed by every chat in a
# review's tree, the other chat_ maps by review chat, and the ws_ maps by
# workspace ID. chat_last_tree holds each review's tree from its last read.
# ws_stop is "sent" once a stop request from this run succeeds or a stop
# build follows a failed one, and "failed" while the last one failed and no
# later read has shown a build in progress.
declare -A chat_interrupts=() chat_interrupt_failed=() chat_queue_warned=()
declare -A chat_sweeps=() chat_read_ok=() chat_last_tree=()
declare -A ws_name=() ws_stop=() ws_read_ok=()

warn() {
	echo "::warning::$*"
}

# Prints the seconds left in the current phase, which can be zero or less.
time_left() {
	echo $((deadline - ($(now) - STARTED_AT)))
}

# Sleeps one poll interval if at least 1 second of the phase is left after
# it, and fails otherwise.
sleep_if_time() {
	(($(time_left) > POLL_SECONDS)) || return 1
	sleep "${POLL_SECONDS}"
}

api() {
	local method="$1" path="$2" max
	shift 2
	max=$(time_left)
	((max > REQUEST_SECONDS)) && max="${REQUEST_SECONDS}"
	# curl --max-time 0 never times out.
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

# Warns about a chat's queue once, and prints later warnings for that chat
# as plain lines. GitHub keeps only the first 10 warning annotations of a
# step, and a busy chat's queue is dropped again on every sweep.
warn_queue() {
	local id="$1"
	shift
	if [[ "${chat_queue_warned["${id}"]:-}" == "true" ]]; then
		echo "$*"
		return 0
	fi
	chat_queue_warned["${id}"]=true
	warn "$*"
}

# Drops every queued message on a chat. Deleting one can promote the next,
# so this reads the queue again after each delete. A delete answers 404
# when chatd promoted the message after the read, so a failed delete warns
# only if the next read still shows that message first.
clear_queue() {
	local id="$1" messages qid failed="" n
	local restarts="When its interruption finishes, the next queued message starts it again."
	for ((n = 0; n <= DELETES_PER_CLEAR; n++)); do
		# queued_messages holds the whole queue on any read without a
		# cursor; limit=1 only keeps the message page small.
		messages=$(api GET "/api/v2/chats/${id}/messages?limit=1") || {
			warn_queue "${id}" "Could not read the queue of chat ${id}. ${restarts}"
			return 0
		}
		qid=$(jq -r '.queued_messages[0].id // empty' <<<"${messages}")
		if [[ -z "${qid}" ]]; then
			return 0
		fi
		if [[ "${qid}" == "${failed}" ]]; then
			warn_queue "${id}" "Could not drop queued message ${qid} on chat ${id}. ${restarts}"
			return 0
		fi
		# The last pass only reads, so a queue that the last delete emptied
		# does not warn.
		if ((n == DELETES_PER_CLEAR)); then
			break
		fi
		if ! api DELETE "/api/v2/chats/${id}/queue/${qid}" >/dev/null; then
			failed="${qid}"
			continue
		fi
		echo "Dropped queued message ${qid} on chat ${id}."
	done
	warn_queue "${id}" "Chat ${id} still has queued messages after ${DELETES_PER_CLEAR} deletes. ${restarts}"
}

# Sweeps a review chat once, and returns 0 when it and its sub-agents are
# idle. Finishing an interruption or leaving a pause promotes the next
# queued message and starts the chat again, so every chat that is not idle
# gets its queue dropped before any interrupt. A chat in error keeps its
# queue, and the next message sent to it runs the oldest queued one first,
# so it gets its queue dropped too, but it is not busy. An interrupted chat
# in requires_action returns to running, so a later sweep interrupts it
# again. A chat that ends its turn after the read answers an interrupt with
# 409, so a failed interrupt does not warn here; the next sweep reads the
# chat again, and report_chat warns if it still reads as running.
sweep_chat() {
	local root="$1" tree id status sent busy=false
	chat_sweeps["${root}"]=$((${chat_sweeps["${root}"]:-0} + 1))
	if ! tree=$(chat_tree "${root}"); then
		chat_read_ok["${root}"]=false
		return 1
	fi
	chat_read_ok["${root}"]=true
	chat_last_tree["${root}"]="${tree}"
	while read -r id status <&3; do
		case "${status}" in
		running | requires_action | interrupting | paused)
			busy=true
			clear_queue "${id}"
			;;
		error) clear_queue "${id}" ;;
		esac
	done 3<<<"${tree}"
	if [[ "${busy}" != "true" ]]; then
		if ((chat_sweeps["${root}"] > 1)); then
			echo "Review chat ${root} and its sub-agents are stopped."
		fi
		return 0
	fi
	while read -r id status <&3; do
		case "${status}" in
		running | requires_action) ;;
		*) continue ;;
		esac
		sent="${chat_interrupts["${id}"]:-0}"
		if ((sent >= INTERRUPTS_PER_CHAT)); then
			continue
		fi
		chat_interrupts["${id}"]=$((sent + 1))
		echo "Interrupting chat ${id} (${status})."
		if api POST "/api/v2/chats/${id}/interrupt" >/dev/null; then
			chat_interrupt_failed["${id}"]=false
		else
			chat_interrupt_failed["${id}"]=true
			echo "Could not interrupt chat ${id}. The next sweep checks it again."
		fi
	done 3<<<"${tree}"
	return 1
}

# Warns about each chat in the review's last read that is running and whose
# last interrupt failed, and once about the review if any other chat in it
# was still busy.
report_chat() {
	local root="$1" id status late=false
	if [[ "${chat_read_ok["${root}"]}" != "true" ]]; then
		warn "Could not read review chat ${root}. It may still be running in Coder."
		return 0
	fi
	while read -r id status; do
		case "${status}" in
		running | requires_action)
			if [[ "${chat_interrupt_failed["${id}"]:-}" == "true" ]]; then
				warn "Could not interrupt chat ${id}. It may still be running in Coder."
			else
				late=true
			fi
			;;
		interrupting | paused) late=true ;;
		esac
	done <<<"${chat_last_tree["${root}"]}"
	if [[ "${late}" == "true" ]]; then
		warn "Review chat ${root} did not stop in time. It may still be running in Coder."
	fi
}

# Sweeps a workspace once, and returns 0 when it is stopped or nothing more
# can be done. A build in progress finishes first; any other state gets a
# stop until a stop request succeeds: a running workspace, or one whose
# last build failed or was canceled and may have left it up. A stop request
# answers 409 when a build started after the read, and one that timed out
# may have been accepted, so a failed request is retried after a new read.
sweep_workspace() {
	local id="$1" workspace name transition status
	# coderd answers 410 Gone for a deleted workspace without include_deleted.
	if ! workspace=$(api GET "/api/v2/workspaces/${id}?include_deleted=true"); then
		ws_read_ok["${id}"]=false
		return 1
	fi
	ws_read_ok["${id}"]=true
	name=$(jq -r '.name' <<<"${workspace}")
	ws_name["${id}"]="${name}"
	transition=$(jq -r '.latest_build.transition' <<<"${workspace}")
	status=$(jq -r '.latest_build.status' <<<"${workspace}")
	case "${transition}/${status}" in
	*/pending | */starting | */stopping | */canceling | */deleting)
		# A failed stop request may still have been accepted, as one that
		# timed out can be, so a stop build after it counts as sent. A start
		# or delete build means the request did not apply, and the sweep
		# after that build decides again.
		if [[ "${ws_stop["${id}"]:-}" == "failed" ]]; then
			if [[ "${transition}" == "stop" ]]; then
				ws_stop["${id}"]=sent
			else
				ws_stop["${id}"]=""
			fi
		fi
		return 1
		;;
	stop/stopped | delete/deleted)
		case "${ws_stop["${id}"]:-}" in
		sent) echo "Stopped workspace ${name}." ;;
		# The last stop request failed, but the workspace stopped or was
		# deleted between two reads, by that request if coderd accepted it
		# before it timed out, or by another build.
		failed) echo "Workspace ${name} is ${status}." ;;
		esac
		return 0
		;;
	*)
		if [[ "${ws_stop["${id}"]:-}" == "sent" ]]; then
			if [[ "${transition}" == "start" ]]; then
				warn "A start build of workspace ${name} began after this run stopped it, and the workspace is now ${status}. A review chat that did not stop may still be running on it."
			else
				warn "The ${transition} build of workspace ${name} ended with status ${status}. The next review may run on this workspace with an expired GitHub token."
			fi
			return 0
		fi
		;;
	esac
	echo "Stopping workspace ${name} (${transition} build ${status})."
	if api POST "/api/v2/workspaces/${id}/builds" \
		-H "Content-Type: application/json" \
		-d '{"transition":"stop"}' >/dev/null; then
		ws_stop["${id}"]=sent
	else
		ws_stop["${id}"]=failed
	fi
	return 1
}

report_workspace() {
	local name="${ws_name["$1"]:-$1}"
	if [[ "${ws_read_ok["$1"]}" != "true" ]]; then
		warn "Could not read workspace ${name}, so this run could not confirm it stopped. It may still be running in Coder."
	elif [[ "${ws_stop["$1"]:-}" == "failed" ]]; then
		warn "Could not stop workspace ${name}. It stops at its autostop deadline instead."
	else
		warn "Workspace ${name} did not finish its build within the ${BUDGET_SECONDS}-second budget."
	fi
}

# Sweeps every item once, then sweeps the items that have not settled again
# while sleep_if_time allows a poll, and reports the ones left.
run_phase() {
	local sweep="$1" report="$2" item
	shift 2
	local -a pending=() unsettled=()
	for item in "$@"; do
		"${sweep}" "${item}" || pending+=("${item}")
	done
	while ((${#pending[@]} > 0)) && sleep_if_time; do
		unsettled=()
		for item in "${pending[@]}"; do
			"${sweep}" "${item}" || unsettled+=("${item}")
		done
		pending=("${unsettled[@]}")
	done
	for item in "${pending[@]}"; do
		"${report}" "${item}"
	done
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
	local pr_ws_name="doc-check-pr-${BASH_REMATCH[4]}"
	local chats workspaces
	local -a roots=() workspace_ids=()

	# Chat phase.
	deadline="${CHAT_PHASE_SECONDS}"
	# The labels coder/agents-chat-action writes on every chat it creates.
	# The list leaves out archived chats. Each run starts a new chat, so
	# stop every match, not only the newest.
	chats=$(api GET /api/v2/chats -G \
		--data-urlencode "label=coder-agents-chat-action:true" \
		--data-urlencode "label=gh-target:${gh_target}" \
		--data-urlencode "label=coder-agents-chat-action-workflow:${WORKFLOW_NAME}") || {
		warn "Could not look up the review chat for ${gh_target}."
		chats='[]'
	}
	mapfile -t roots < <(jq -r '.[].id' <<<"${chats}")
	run_phase sweep_chat report_chat "${roots[@]}"

	# Workspace phase.
	deadline="${BUDGET_SECONDS}"
	# The chats' own workspaces, plus the one named for the pull request.
	# That name has only the number, so look it up only for this
	# repository's pull requests. The name filter matches substrings, so keep
	# only the exact name.
	local this_repo="${GITHUB_REPOSITORY:-}"
	workspaces='{}'
	if [[ "${pr_repo,,}" == "${this_repo,,}" ]]; then
		workspaces=$(api GET /api/v2/workspaces -G \
			--data-urlencode "q=owner:me name:${pr_ws_name}") || {
			warn "Could not look up workspace ${pr_ws_name}."
			workspaces='{}'
		}
	fi
	mapfile -t workspace_ids < <(
		{
			jq -r '.[].workspace_id // empty' <<<"${chats}"
			jq -r --arg name "${pr_ws_name}" '.workspaces[]? | select(.name == $name) | .id' <<<"${workspaces}"
		} | sort -u
	)
	run_phase sweep_workspace report_workspace "${workspace_ids[@]}"
	return 0
}

main "$@"
