#!/usr/bin/env bash
# Tests for doc-check-previous-trace.sh against a fake curl on PATH. No CI
# job runs this file.
#
# Run: bash .github/scripts/doc-check-previous-trace.test.sh

set -euo pipefail

SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/doc-check-previous-trace.sh"
WORK=$(mktemp -d)
trap 'rm -rf "${WORK}"' EXIT

# The fake API. State lives in ${STATE}:
#   list.json        the answer to the label lookup
#   messages/<id>    the answer to a read of that chat's messages; a chat
#                    with no file answers with an HTTP error
#   fail-list        when present, the label lookup answers with an HTTP error
#   calls            one line per request
mkdir -p "${WORK}/bin"
cat >"${WORK}/bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
url="" query=() max_time=""
while (($#)); do
	case "$1" in
	-H) shift 2 ;;
	--max-time) max_time="$2"; shift 2 ;;
	--data-urlencode) query+=("$2"); shift 2 ;;
	*) [[ "$1" == http* ]] && url="$1"; shift ;;
	esac
done
path="${url#"${CODER_URL%/}"}"
flag=" max=${max_time}"
[[ -z "${max_time}" ]] && flag=" NO-MAX-TIME"
echo "GET ${path}${query[*]:+ ${query[*]}}${flag}" >>"${STATE}/calls"
fail() {
	echo '{"message":"simulated failure"}'
	exit 22
}
case "${path}" in
/api/v2/chats)
	[[ -e "${STATE}/fail-list" ]] && fail
	cat "${STATE}/list.json"
	;;
/api/v2/chats/*/messages)
	id="${path#/api/v2/chats/}"
	id="${id%/messages}"
	[[ -e "${STATE}/messages/${id}" ]] || fail
	cat "${STATE}/messages/${id}"
	;;
*)
	echo "fake curl: unhandled ${path}" >&2
	exit 1
	;;
esac
EOF
chmod +x "${WORK}/bin/curl"

failures=0
case_name=""

# Starts a case with an empty chat list.
begin() {
	case_name="$1"
	STATE="${WORK}/state-${RANDOM}-${RANDOM}"
	mkdir -p "${STATE}/messages"
	echo '[]' >"${STATE}/list.json"
	: >"${STATE}/calls"
}

# add_chat <id> <created_at> [parent id]: adds a chat to the label lookup.
add_chat() {
	jq --arg id "$1" --arg at "$2" --arg parent "${3:-}" \
		'. + [{id: $id, created_at: $at} + (if $parent == "" then {} else {parent_chat_id: $parent} end)]' \
		"${STATE}/list.json" >"${STATE}/list.json.tmp"
	mv "${STATE}/list.json.tmp" "${STATE}/list.json"
}

# add_messages <id> <role:text>...: sets a chat's messages, newest first.
add_messages() {
	local id="$1" entry
	shift
	local messages='[]'
	for entry in "$@"; do
		messages=$(jq --arg role "${entry%%:*}" --arg text "${entry#*:}" \
			'. + [{role: $role, content: [{type: "reasoning", text: "doc-check-trace:\n  outcome: reasoning"}, {type: "text", text: $text}]}]' \
			<<<"${messages}")
	done
	jq '{messages: ., queued_messages: []}' <<<"${messages}" >"${STATE}/messages/${id}"
}

# A message that ends with a trace in a code fence, as output.md asks.
trace_message() {
	# The backticks are a literal code fence.
	# shellcheck disable=SC2016
	printf 'No findings.\n\n```yaml\ndoc-check-trace:\n  pr: 7\n  head_sha: %s\n  outcome: clean\n```\n' "$1"
}

# Runs the script. A variable that a case does not set reaches the script as
# an empty string, the way GitHub Actions passes an unset value.
run() {
	STATUS=0
	OUT=$(STATE="${STATE}" PATH="${WORK}/bin:${PATH}" \
		CODER_URL="https://coder.example.com/" CODER_TOKEN="token" \
		PR_URL="${PR_URL-https://github.com/coder/coder/issues/7}" \
		WORKFLOW_NAME="AI Documentation Check" \
		TRACE_MAX_CHATS="${TRACE_MAX_CHATS-}" TRACE_MAX_CHARS="${TRACE_MAX_CHARS-}" \
		bash "${SCRIPT}" 2>"${STATE}/stderr") || STATUS=$?
}

fail_case() {
	failures=$((failures + 1))
	echo "FAIL: ${case_name}: $*"
	sed 's/^/    stderr: /' "${STATE}/stderr"
	sed 's/^/    call: /' "${STATE}/calls"
}

expect_out() {
	if [[ "${OUT}" != "$1" ]]; then
		fail_case "stdout was $(printf '%q' "${OUT}"), want $(printf '%q' "$1")"
	fi
	if [[ "${STATUS}" != "0" ]]; then
		fail_case "exit status ${STATUS}, want 0"
	fi
}

expect_stderr() {
	grep -qF -- "$1" "${STATE}/stderr" || fail_case "stderr lacks: $1"
}

expect_calls() {
	local got
	got=$(grep -c "$1" "${STATE}/calls" || true)
	if [[ "${got}" != "$2" ]]; then
		fail_case "${got} calls match '$1', want $2"
	fi
}

readonly TRACE_A=$'doc-check-trace:\n  pr: 7\n  head_sha: aaa\n  outcome: clean'
readonly TRACE_B=$'doc-check-trace:\n  pr: 7\n  head_sha: bbb\n  outcome: clean'

# Optional variables are unset unless a case sets them for one run.
unset TRACE_MAX_CHATS TRACE_MAX_CHARS PR_URL

begin "no earlier chat prints nothing"
run
expect_out ""
expect_stderr "No earlier review chat for coder/coder#7."
expect_calls "messages" 0

begin "the lookup uses the action's three labels and a request limit"
run
expect_calls "GET /api/v2/chats label=coder-agents-chat-action:true label=gh-target:coder/coder#7 label=coder-agents-chat-action-workflow:AI Documentation Check max=30" 1

begin "the newest chat's trace is printed without its code fence"
add_chat old 2026-11-01T00:00:00Z
add_chat new 2026-11-02T00:00:00Z
add_messages old "assistant:$(trace_message bbb)"
add_messages new "assistant:$(trace_message aaa)"
run
expect_out "${TRACE_A}"
expect_stderr "Found the trace of review chat new."
expect_calls "/api/v2/chats/old/messages" 0
expect_calls "/api/v2/chats/new/messages limit=50 max=30" 1

begin "newest means created last, not listed first"
add_chat new 2026-11-02T00:00:00Z
add_chat old 2026-11-01T00:00:00Z
add_chat newest 2026-11-03T00:00:00Z
add_messages old "assistant:$(trace_message bbb)"
add_messages new "assistant:$(trace_message bbb)"
add_messages newest "assistant:$(trace_message aaa)"
run
expect_out "${TRACE_A}"

begin "a cancelled run's chat has no trace, so the one before it is used"
add_chat old 2026-11-01T00:00:00Z
add_chat cancelled 2026-11-02T00:00:00Z
add_messages old "assistant:$(trace_message bbb)"
add_messages cancelled "assistant:Reading the diff." "user:Use the doc-check skill."
run
expect_out "${TRACE_B}"
expect_stderr "Review chat cancelled ended with no trace."

begin "a sub-agent chat is not a review chat"
add_chat root 2026-11-01T00:00:00Z
add_chat child 2026-11-02T00:00:00Z root
add_messages root "assistant:$(trace_message aaa)"
add_messages child "assistant:$(trace_message bbb)"
run
expect_out "${TRACE_A}"
expect_calls "/api/v2/chats/child/messages" 0

begin "the trace in the prompt of the next run is not that run's trace"
add_chat old 2026-11-01T00:00:00Z
add_chat new 2026-11-02T00:00:00Z
add_messages old "assistant:$(trace_message bbb)"
add_messages new "assistant:Starting." "user:Previous run: $(trace_message bbb)"
run
expect_out "${TRACE_B}"
expect_calls "/api/v2/chats/old/messages" 1

begin "the newest message with a trace wins inside one chat"
add_chat chat 2026-11-01T00:00:00Z
add_messages chat "assistant:$(trace_message aaa)" "user:Review again." "assistant:$(trace_message bbb)"
run
expect_out "${TRACE_A}"

begin "the last trace block in one message wins"
add_chat chat 2026-11-01T00:00:00Z
add_messages chat "assistant:$(trace_message bbb)$(trace_message aaa)"
run
expect_out "${TRACE_A}"

begin "text after the closing fence is not part of the trace"
add_chat chat 2026-11-01T00:00:00Z
add_messages chat "assistant:$(trace_message aaa)"$'\nThat is the end of the review.'
run
expect_out "${TRACE_A}"

begin "a trace with no code fence runs to the end of the message"
add_chat chat 2026-11-01T00:00:00Z
add_messages chat "assistant:Done."$'\n\n'"${TRACE_A}"$'\n\n'
run
expect_out "${TRACE_A}"

begin "a mention of the block with no outcome line is not a trace"
add_chat old 2026-11-01T00:00:00Z
add_chat new 2026-11-02T00:00:00Z
add_messages old "assistant:$(trace_message bbb)"
add_messages new "assistant:I will end with the doc-check-trace: block once the personas report."
run
expect_out "${TRACE_B}"

begin "a trace written with CRLF line endings loses its carriage returns"
add_chat chat 2026-11-01T00:00:00Z
add_messages chat "assistant:$(trace_message aaa | sed 's/$/\r/')"
run
expect_out "${TRACE_A}"

begin "a long trace is cut at TRACE_MAX_CHARS"
add_chat chat 2026-11-01T00:00:00Z
add_messages chat "assistant:$(trace_message aaa)"
TRACE_MAX_CHARS=30 run
expect_out "${TRACE_A:0:30}"

begin "only the TRACE_MAX_CHATS newest chats are read"
add_chat c1 2026-11-01T00:00:00Z
add_chat c2 2026-11-02T00:00:00Z
add_chat c3 2026-11-03T00:00:00Z
add_messages c1 "assistant:$(trace_message aaa)"
add_messages c2 "assistant:Stopped."
add_messages c3 "assistant:Stopped."
TRACE_MAX_CHATS=2 run
expect_out ""
expect_stderr "None of the 2 newest review chats for coder/coder#7 ended with a trace."
expect_calls "/api/v2/chats/c1/messages" 0

begin "five chats are read by default"
for n in 1 2 3 4 5 6; do
	add_chat "c${n}" "2026-11-0${n}T00:00:00Z"
	add_messages "c${n}" "assistant:Stopped."
done
run
expect_out ""
expect_calls "messages" 5
expect_calls "/api/v2/chats/c1/messages" 0

begin "a failed label lookup prints nothing and exits 0"
add_chat chat 2026-11-01T00:00:00Z
add_messages chat "assistant:$(trace_message aaa)"
touch "${STATE}/fail-list"
run
expect_out ""
expect_stderr "Could not look up the review chats for coder/coder#7."

begin "a chat whose messages cannot be read is skipped"
add_chat old 2026-11-01T00:00:00Z
add_chat unreadable 2026-11-02T00:00:00Z
add_messages old "assistant:$(trace_message bbb)"
run
expect_out "${TRACE_B}"
expect_stderr "Could not read review chat unreadable."

begin "a messages answer that is not JSON is skipped"
add_chat old 2026-11-01T00:00:00Z
add_chat broken 2026-11-02T00:00:00Z
add_messages old "assistant:$(trace_message bbb)"
echo "<html>502</html>" >"${STATE}/messages/broken"
run
expect_out "${TRACE_B}"

begin "a pull request URL in its /pull/ form with a trailing CR works"
add_chat chat 2026-11-01T00:00:00Z
add_messages chat "assistant:$(trace_message aaa)"
PR_URL=$'https://github.com/coder/coder/pull/7\r' run
expect_out "${TRACE_A}"
expect_calls "label=gh-target:coder/coder#7 " 1

begin "a URL that is not a pull request makes no request"
PR_URL="https://github.com/coder/coder" run
expect_out ""
expect_calls "GET" 0

begin "an empty pull request URL makes no request"
PR_URL="" run
expect_out ""
expect_calls "GET" 0

if ((failures > 0)); then
	echo "${failures} doc-check-previous-trace.sh check(s) failed."
	exit 1
fi
echo "All doc-check-previous-trace.sh cases pass."
