#!/usr/bin/env bash
# Prints the doc-check-trace block that the latest finished doc-check review
# of a pull request ended with, and nothing when there is none.
#
# Each doc-check run starts a new chat, so the workflow passes this trace to
# the new chat in its prompt.
#
# Environment:
#   CODER_URL         Coder deployment URL.
#   CODER_TOKEN       Session token of the bot that owns the review chats.
#   PR_URL            Pull request URL, in its /pull/ or /issues/ form.
#   WORKFLOW_NAME     Workflow name. coder/agents-chat-action labels its
#                     chats with it, along with the pull request.
#   TRACE_MAX_CHATS   Optional. The most review chats to read, newest first,
#                     5 by default. A run that was cancelled or failed ends
#                     with no trace, so the newest chat often has none.
#   TRACE_MAX_CHARS   Optional. The most characters of the trace to print,
#                     8000 by default.
#
# The trace goes to stdout and every other line to stderr. A failure prints
# a line to stderr and exits 0 with no trace, because the review can run
# without it. Tested by doc-check-previous-trace.test.sh.

set -uo pipefail

readonly MAX_CHATS="${TRACE_MAX_CHATS:-5}"
readonly MAX_CHARS="${TRACE_MAX_CHARS:-8000}"
readonly REQUEST_SECONDS=30
# The newest messages of a chat. A finished review ends with its trace, so
# the trace is in the first few.
readonly MESSAGES_PER_CHAT=50

log() {
	echo "$*" >&2
}

api() {
	local path="$1"
	shift
	curl -sS --fail-with-body --max-time "${REQUEST_SECONDS}" -G \
		-H "Coder-Session-Token: ${CODER_TOKEN}" \
		"$@" "${CODER_URL%/}${path}"
}

# Prints the trace in a chat's newest messages, and nothing when no message
# has one. Messages come newest first. The trace runs from the last
# "doc-check-trace:" in a message to the code fence that closes it, or to
# the end of the message. Text that names the block but has no outcome line is
# not a trace: a review that was stopped can mention the block it never
# wrote.
chat_trace() {
	local messages
	messages=$(api "/api/v2/chats/$1/messages" \
		--data-urlencode "limit=${MESSAGES_PER_CHAT}") || return 1
	jq -r --argjson max "${MAX_CHARS}" '
		first(
			.messages[]?
			| select(.role == "assistant")
			| .content[]?
			| select(.type == "text")
			| .text
			| gsub("\r"; "")
			| select(contains("doc-check-trace:"))
			| "doc-check-trace:" + (split("doc-check-trace:") | last)
			| split("\n```") | first
			| select(test("\n\\s*outcome:"))
		) // empty
		| .[0:$max]' <<<"${messages}"
}

main() {
	# A value written on Windows can end in a carriage return.
	local pr_url="${PR_URL:-}"
	pr_url="${pr_url%$'\r'}"
	if [[ ! "${pr_url}" =~ ^https://github\.com/([^/]+)/([^/]+)/(pull|issues)/([0-9]+)$ ]]; then
		log "No pull request URL. No previous trace."
		return 0
	fi
	local gh_target="${BASH_REMATCH[1]}/${BASH_REMATCH[2]}#${BASH_REMATCH[4]}"
	local chats id trace
	local -a roots=()

	# The labels coder/agents-chat-action writes on every chat it creates.
	# The list leaves out archived chats.
	chats=$(api /api/v2/chats \
		--data-urlencode "label=coder-agents-chat-action:true" \
		--data-urlencode "label=gh-target:${gh_target}" \
		--data-urlencode "label=coder-agents-chat-action-workflow:${WORKFLOW_NAME}") || {
		log "Could not look up the review chats for ${gh_target}. No previous trace."
		return 0
	}
	# Review chats only, not their sub-agents, newest first.
	mapfile -t roots < <(jq -r --argjson max "${MAX_CHATS}" '
		[.[] | select(.parent_chat_id == null)]
		| sort_by(.created_at) | reverse | .[0:$max][] | .id' <<<"${chats}")
	if ((${#roots[@]} == 0)); then
		log "No earlier review chat for ${gh_target}."
		return 0
	fi

	for id in "${roots[@]}"; do
		if ! trace=$(chat_trace "${id}"); then
			log "Could not read review chat ${id}. Trying the one before it."
			continue
		fi
		if [[ -n "${trace}" ]]; then
			log "Found the trace of review chat ${id}."
			printf '%s\n' "${trace}"
			return 0
		fi
		log "Review chat ${id} ended with no trace."
	done
	log "None of the ${#roots[@]} newest review chats for ${gh_target} ended with a trace."
	return 0
}

main "$@"
