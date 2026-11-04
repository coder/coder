#!/usr/bin/env bash
# Tests for doc-check-stop-review.sh against a fake curl and sleep on PATH.
# No CI job runs this file.
#
# Run: bash .github/scripts/doc-check-stop-review.test.sh

set -euo pipefail

SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/doc-check-stop-review.sh"
WORK=$(mktemp -d)
trap 'rm -rf "${WORK}"' EXIT

mkdir -p "${WORK}/bin"
cat >"${WORK}/bin/sleep" <<'EOF'
#!/usr/bin/env bash
echo "SLEEP" >>"${STATE}/calls"
EOF

# The fake API. State lives in ${STATE}/state.json:
#   chats       id -> {status, workspace_id, children, queued}
#   list        chat IDs the label lookup returns
#   stuck       chat IDs that ignore interrupts
#   workspaces  id -> {name, transition, statuses, after_stop}; each read
#               takes the next status and keeps the last, and a stop
#               applies after_stop when it is set
#   fail        paths that answer with an HTTP error
#   slow        seconds each request takes
# An interrupted chat reads as interrupting twice, and a stopping
# workspace reads as stopping twice, so a case shows whether the script
# waits for them.
cat >"${WORK}/bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
method=GET data="" url="" query=() max_time=""
while (($#)); do
	case "$1" in
	-X) method="$2"; shift 2 ;;
	-H) shift 2 ;;
	--max-time) max_time="$2"; shift 2 ;;
	-d) data="$2"; shift 2 ;;
	--data-urlencode) query+=("$2"); shift 2 ;;
	*) [[ "$1" == http* ]] && url="$1"; shift ;;
	esac
done
path="${url#"${CODER_URL%/}"}"
qs=""
[[ "${path}" == *\?* ]] && qs="${path#*\?}"
path="${path%%\?*}"
flag=" max=${max_time}"
[[ -z "${max_time}" ]] && flag=" NO-MAX-TIME"
echo "${method} ${path}${query[*]:+ ${query[*]}}${data:+ ${data}}${flag}" >>"${STATE}/calls"

st="${STATE}/state.json"
update() {
	jq "$@" "${st}" >"${st}.tmp" && mv "${st}.tmp" "${st}"
}
fail() {
	echo '{"message":"simulated failure"}'
	exit 22
}
if jq -e --arg p "${path}" '.fail | index($p)' "${st}" >/dev/null; then
	fail
fi
slow=$(jq -r '.slow // 0' "${st}")
if [[ "${slow}" != 0 ]]; then
	/bin/sleep "${slow}"
fi

case "${method} ${path}" in
"GET /api/v2/chats")
	jq -c '[.list[] as $id | .chats[$id] + {id: $id}]' "${st}"
	;;
GET\ /api/v2/chats/*/messages)
	id="${path#/api/v2/chats/}"
	id="${id%/messages}"
	jq -c --arg id "${id}" '{messages: [], queued_messages: [.chats[$id].queued[] | {id: .}]}' "${st}"
	;;
DELETE\ /api/v2/chats/*/queue/*)
	rest="${path#/api/v2/chats/}"
	id="${rest%%/*}"
	qid="${rest##*/}"
	jq -e --arg id "${id}" --argjson q "${qid}" '.chats[$id].queued | index($q)' "${st}" >/dev/null || fail
	update --arg id "${id}" --argjson q "${qid}" '.chats[$id].queued -= [$q]'
	# Leaving a pause promotes the next queued message and runs it, or
	# returns to waiting when none is left, as in chatstate.
	update --arg id "${id}" '
		if .chats[$id].status == "paused" then
			if (.chats[$id].queued | length) > 0 then
				.chats[$id].queued |= .[1:] | .chats[$id].status = "running"
			else .chats[$id].status = "waiting" end
		else . end'
	;;
POST\ /api/v2/chats/*/interrupt)
	id="${path#/api/v2/chats/}"
	id="${id%/interrupt}"
	status=$(jq -r --arg id "${id}" '.chats[$id].status' "${st}")
	if jq -e --arg id "${id}" '.stuck | index($id)' "${st}" >/dev/null; then
		echo '{}'
		exit 0
	fi
	case "${status}" in
	running) update --arg id "${id}" '.chats[$id] += {status: "interrupting", reads: 2}' ;;
	# An interrupted requires_action chat gets cancelled tool results and
	# runs again, as in chatstate.
	requires_action) update --arg id "${id}" '.chats[$id].status = "running"' ;;
	*) fail ;;
	esac
	echo '{}'
	;;
GET\ /api/v2/chats/*)
	id="${path#/api/v2/chats/}"
	jq -c --arg id "${id}" '
		def one($id): .chats[$id] + {id: $id};
		one($id) + {children: [.chats[$id].children[] as $c | one($c) | del(.children)]}
	' "${st}"
	update '.chats |= with_entries(
		if .value.status == "interrupting" then
			.value.reads -= 1 | if .value.reads <= 0 then .value.status = "waiting" else . end
		else . end)'
	;;
"GET /api/v2/workspaces")
	name="${query[0]#q=owner:me name:}"
	jq -c --arg n "${name}" '{workspaces: [.workspaces | to_entries[] | select(.value.name | contains($n)) | {id: .key, name: .value.name}]}' "${st}"
	;;
POST\ /api/v2/workspaces/*/builds)
	id="${path#/api/v2/workspaces/}"
	id="${id%/builds}"
	echo "STATE active chats: $(jq '[.chats[] | select(.status | IN("running", "requires_action", "interrupting"))] | length' "${st}")" >>"${STATE}/calls"
	update --arg id "${id}" '.workspaces[$id] += ({transition: "stop", statuses: ["stopping", "stopping", "stopped"]} + (.workspaces[$id].after_stop // {}))'
	echo '{}'
	;;
GET\ /api/v2/workspaces/*)
	id="${path#/api/v2/workspaces/}"
	# A deleted workspace answers 410 Gone unless the read includes deleted
	# workspaces, as in coderd.
	if [[ "$(jq -r --arg id "${id}" '.workspaces[$id].transition' "${st}")" == "delete" && "${qs}" != *include_deleted=true* ]]; then
		fail
	fi
	jq -c --arg id "${id}" '.workspaces[$id] | {id: $id, name, latest_build: {transition, status: .statuses[0]}}' "${st}"
	update --arg id "${id}" 'if (.workspaces[$id].statuses | length) > 1 then .workspaces[$id].statuses |= .[1:] else . end'
	;;
*)
	echo "fake curl: unhandled ${method} ${path}" >&2
	exit 2
	;;
esac
EOF
chmod +x "${WORK}/bin/curl" "${WORK}/bin/sleep"

failures=0
case_name=""
OUT=""
CALLS=""

# run_case NAME PR_URL STATE_JQ [VAR=VALUE...] runs the script against a
# fresh state, with any extra environment. STATE_JQ is a jq expression, so
# cases can leave keys unquoted.
run_case() {
	case_name="$1"
	export STATE="${WORK}/${case_name}"
	mkdir -p "${STATE}"
	: >"${STATE}/calls"
	jq -n "{chats: {}, list: [], stuck: [], workspaces: {}, fail: []} + ($3)" >"${STATE}/state.json"
	local code=0
	OUT=$(env PATH="${WORK}/bin:${PATH}" CODER_URL="https://coder.example.com/" CODER_TOKEN="token" \
		PR_URL="$2" WORKFLOW_NAME="AI Documentation Check" GITHUB_REPOSITORY="coder/coder" \
		"${@:4}" bash "${SCRIPT}" 2>&1) || code=$?
	CALLS=$(cat "${STATE}/calls")
	if [[ "${code}" != 0 ]]; then
		fail_case "exited ${code}"
	fi
}

fail_case() {
	echo "FAIL ${case_name}: $*"
	echo "--- output"
	echo "${OUT}"
	echo "--- calls"
	echo "${CALLS}"
	failures=$((failures + 1))
}

expect_output() {
	grep -qF -- "$1" <<<"${OUT}" || fail_case "output lacks: $1"
}

expect_no_output() {
	! grep -qF -- "$1" <<<"${OUT}" || fail_case "output has: $1"
}

expect_calls() {
	local want="$1" pattern="$2" got
	got=$(grep -cF -- "${pattern}" <<<"${CALLS}" || true)
	[[ "${got}" == "${want}" ]] || fail_case "want ${want} calls matching '${pattern}', got ${got}"
}

expect_before() {
	local a b
	a=$(grep -nF -- "$1" <<<"${CALLS}" | head -1 | cut -d: -f1 || true)
	b=$(grep -nF -- "$2" <<<"${CALLS}" | head -1 | cut -d: -f1 || true)
	[[ -n "${a}" && -n "${b}" && "${a}" -lt "${b}" ]] || fail_case "want '$1' before '$2'"
}

state_of() {
	jq -r "$1" "${STATE}/state.json"
}

# --- Cases ---

run_case no-pr-url "" '{}'
expect_output "Nothing to stop"
[[ -z "${CALLS}" ]] || fail_case "made API calls without a pull request"

run_case idle-review "https://github.com/coder/coder/pull/12" '{
	list: ["r1"],
	chats: {
		r1: {status: "waiting", workspace_id: "w1", children: ["c1"], queued: []},
		c1: {status: "waiting", workspace_id: "w1", children: [], queued: []}
	},
	workspaces: {w1: {name: "doc-check-pr-12", transition: "stop", statuses: ["stopped"]}}
}'
expect_calls 1 "GET /api/v2/chats label=coder-agents-chat-action:true label=gh-target:coder/coder#12 label=coder-agents-chat-action-workflow:AI Documentation Check"
expect_calls 0 "/interrupt"
expect_calls 0 "/builds"
expect_calls 0 "SLEEP"
expect_no_output "::warning::"

run_case running-review "https://github.com/coder/coder/issues/12" '{
	list: ["r1"],
	chats: {
		r1: {status: "running", workspace_id: "w1", children: ["c1", "c2"], queued: [11, 12]},
		c1: {status: "running", workspace_id: "w1", children: [], queued: [21]},
		c2: {status: "waiting", workspace_id: "w1", children: [], queued: []}
	},
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["running"]}}
}'
expect_before "DELETE /api/v2/chats/r1/queue/11" "POST /api/v2/chats/r1/interrupt"
expect_before "DELETE /api/v2/chats/r1/queue/12" "POST /api/v2/chats/r1/interrupt"
expect_before "DELETE /api/v2/chats/c1/queue/21" "POST /api/v2/chats/c1/interrupt"
expect_calls 1 "POST /api/v2/chats/r1/interrupt"
expect_calls 1 "POST /api/v2/chats/c1/interrupt"
expect_calls 0 "POST /api/v2/chats/c2/interrupt"
# The stop waits until no chat is interrupting, then until the build ends.
expect_calls 1 "STATE active chats: 0"
expect_calls 4 "GET /api/v2/workspaces/w1"
expect_calls 1 'POST /api/v2/workspaces/w1/builds {"transition":"stop"}'
expect_output "Review chat r1 and its sub-agents are stopped."
expect_output "Stopped workspace doc-check-pr-12."
expect_no_output "::warning::"
[[ "$(state_of '[.chats[].status] | unique | join(",")')" == "waiting" ]] || fail_case "a chat is still active"
[[ "$(state_of '[.chats[].queued[]] | length')" == 0 ]] || fail_case "queued messages remain"
# A request that hangs would hold the step past its cancel grace period.
expect_calls 0 "NO-MAX-TIME"
# With the default budget, no request waits more than 30 seconds.
[[ "$(grep -oE 'max=[0-9-]+' <<<"${CALLS}" | sort -u)" == "max=30" ]] || fail_case "want every request limited to 30 seconds"

# Every review chat the lookup returns gets stopped.
run_case two-reviews "https://github.com/coder/coder/pull/12" '{
	list: ["r1", "r2"],
	chats: {
		r1: {status: "running", workspace_id: null, children: [], queued: []},
		r2: {status: "running", workspace_id: null, children: [], queued: []}
	}
}'
expect_calls 1 "POST /api/v2/chats/r1/interrupt"
expect_calls 1 "POST /api/v2/chats/r2/interrupt"
expect_no_output "::warning::"
[[ "$(state_of '[.chats[].status] | unique | join(",")')" == "waiting" ]] || fail_case "a review is still active"

# The server queues at most 20 messages a chat by default, and dropping
# all of them is not a failure.
run_case full-queue "https://github.com/coder/coder/pull/12" '{
	list: ["r1"],
	chats: {r1: {status: "waiting", workspace_id: null, children: [], queued: [range(1; 21)]}}
}'
expect_calls 20 "DELETE /api/v2/chats/r1/queue/"
expect_no_output "::warning::"

run_case requires-action "https://github.com/coder/coder/pull/12" '{
	list: ["r1"],
	chats: {
		r1: {status: "waiting", workspace_id: null, children: ["c1"], queued: []},
		c1: {status: "requires_action", workspace_id: null, children: [], queued: []}
	}
}'
expect_calls 2 "POST /api/v2/chats/c1/interrupt"
# The second interrupt comes one poll after the first.
second=$(grep -nF "POST /api/v2/chats/c1/interrupt" <<<"${CALLS}" | sed -n 2p | cut -d: -f1 || true)
first=$(grep -nF "POST /api/v2/chats/c1/interrupt" <<<"${CALLS}" | sed -n 1p | cut -d: -f1 || true)
if [[ -n "${first}" && -n "${second}" ]]; then
	[[ "$(sed -n "${first},${second}p" <<<"${CALLS}" | grep -c SLEEP)" == 1 ]] || fail_case "want one poll between the two interrupts"
fi
expect_output "Review chat r1 and its sub-agents are stopped."
expect_no_output "::warning::"

# Dropping the message under edit promotes the next one, which runs.
run_case paused-chat "https://github.com/coder/coder/pull/12" '{
	list: ["r1"],
	chats: {r1: {status: "paused", workspace_id: null, children: [], queued: [31, 32]}}
}'
expect_before "DELETE /api/v2/chats/r1/queue/31" "POST /api/v2/chats/r1/interrupt"
expect_calls 0 "DELETE /api/v2/chats/r1/queue/32"
expect_calls 1 "POST /api/v2/chats/r1/interrupt"
expect_no_output "::warning::"
[[ "$(state_of '.chats.r1.status')" == "waiting" ]] || fail_case "paused chat kept running after its queue was dropped"

run_case paused-chat-last-message "https://github.com/coder/coder/pull/12" '{
	list: ["r1"],
	chats: {r1: {status: "paused", workspace_id: null, children: [], queued: [31]}}
}'
expect_calls 1 "DELETE /api/v2/chats/r1/queue/31"
expect_calls 0 "/interrupt"
[[ "$(state_of '.chats.r1.status')" == "waiting" ]] || fail_case "paused chat did not return to waiting"

run_case stuck-chat "https://github.com/coder/coder/pull/12" '{
	list: ["r1"],
	stuck: ["r1"],
	chats: {r1: {status: "running", workspace_id: "w1", children: [], queued: []}},
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["running"]}}
}'
expect_calls 3 "POST /api/v2/chats/r1/interrupt"
expect_output "::warning::Review chat r1 did not stop in time. It may still be running in Coder."
# The workspace still stops, which ends the stuck chat's tool calls.
expect_calls 1 "POST /api/v2/workspaces/w1/builds"

# Slow requests end the chat polls at the chat share of the budget, which
# leaves the rest for the workspaces.
run_case chat-budget "https://github.com/coder/coder/pull/12" '{
	slow: 1,
	list: ["r1"],
	stuck: ["r1"],
	chats: {r1: {status: "running", workspace_id: "w1", children: [], queued: []}},
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["running"], after_stop: {statuses: ["stopped"]}}}
}' STOP_BUDGET_SECONDS=15
expect_calls 1 "POST /api/v2/chats/r1/interrupt"
expect_output "::warning::Review chat r1 did not stop in time."
expect_calls 1 "POST /api/v2/workspaces/w1/builds"
expect_output "Stopped workspace doc-check-pr-12."

run_case chat-read-fails "https://github.com/coder/coder/pull/12" '{
	list: ["r1"],
	fail: ["/api/v2/chats/r1"],
	chats: {r1: {status: "running", workspace_id: null, children: [], queued: []}}
}'
expect_output "::warning::Could not read review chat r1."
expect_calls 0 "/interrupt"

run_case build-in-progress "https://github.com/coder/coder/pull/12" '{
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["starting", "starting", "running"]}}
}'
expect_calls 1 "POST /api/v2/workspaces/w1/builds"
expect_before "GET /api/v2/workspaces/w1" "POST /api/v2/workspaces/w1/builds"
expect_output "Stopped workspace doc-check-pr-12."

run_case build-never-ends "https://github.com/coder/coder/pull/12" '{
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["starting"]}}
}'
# The first read, then one read for each 5-second poll of the 270-second
# budget.
expect_calls $((1 + 270 / 5)) "GET /api/v2/workspaces/w1"
expect_calls 0 "POST /api/v2/workspaces/w1/builds"
expect_output "::warning::Workspace doc-check-pr-12 did not finish its build within the 270-second budget."

# Requests are limited to the time left in their phase, and the chats get
# the first third of the budget: 1 second of 5. A request made after the
# budget still gets 1 second, and a workspace reached after the budget still
# gets one read, but no more polls.
run_case budget-caps-requests "https://github.com/coder/coder/pull/12" '{
	slow: 3,
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["starting"]}}
}' STOP_BUDGET_SECONDS=5
expect_calls 1 "GET /api/v2/chats label=coder-agents-chat-action:true label=gh-target:coder/coder#12 label=coder-agents-chat-action-workflow:AI Documentation Check max=1"
expect_calls 1 "GET /api/v2/workspaces/w1"
expect_calls 1 "GET /api/v2/workspaces/w1 max=1"
expect_output "did not finish its build within the 5-second budget."

run_case failed-start "https://github.com/coder/coder/pull/12" '{
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["failed"]}}
}'
expect_calls 1 "POST /api/v2/workspaces/w1/builds"
expect_output "Stopped workspace doc-check-pr-12."

# A stop that failed in an earlier run gets another try.
run_case earlier-failed-stop "https://github.com/coder/coder/pull/12" '{
	workspaces: {w1: {name: "doc-check-pr-12", transition: "stop", statuses: ["failed"]}}
}'
expect_calls 1 "POST /api/v2/workspaces/w1/builds"
expect_output "Stopped workspace doc-check-pr-12."

run_case failed-stop "https://github.com/coder/coder/pull/12" '{
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["running"],
		after_stop: {statuses: ["stopping", "failed"]}}}
}'
expect_calls 1 "POST /api/v2/workspaces/w1/builds"
expect_output "::warning::The stop build of workspace doc-check-pr-12 ended with status failed."
expect_no_output "Stopped workspace"

run_case running-after-stop "https://github.com/coder/coder/pull/12" '{
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["running"],
		after_stop: {transition: "start", statuses: ["running"]}}}
}'
expect_calls 1 "POST /api/v2/workspaces/w1/builds"
expect_output "::warning::A start build of workspace doc-check-pr-12 began after this run stopped it, and the workspace is now running."

# A chat keeps the ID of a workspace that was deleted later.
run_case deleted-workspace "https://github.com/coder/coder/pull/12" '{
	list: ["r1"],
	chats: {r1: {status: "waiting", workspace_id: "w1", children: [], queued: []}},
	workspaces: {w1: {name: "doc-check-pr-12", transition: "delete", statuses: ["deleted"]}}
}'
expect_calls 1 "GET /api/v2/workspaces/w1"
expect_calls 0 "/builds"
expect_no_output "::warning::"

# The workspace name has only the PR number, so a PR in another
# repository must not stop this repository's workspace.
run_case other-repository "https://github.com/coder/other/pull/12" '{
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["running"]}}
}'
expect_calls 1 "label=gh-target:coder/other#12"
expect_calls 0 "GET /api/v2/workspaces"
expect_calls 0 "/builds"

# GitHub repository names are case-insensitive.
run_case mixed-case-repository "https://github.com/Coder/Coder/pull/12" '{
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["running"]}}
}'
expect_calls 1 "POST /api/v2/workspaces/w1/builds"

run_case exact-workspace-name "https://github.com/coder/coder/pull/12" '{
	workspaces: {
		w1: {name: "doc-check-pr-12", transition: "start", statuses: ["running"]},
		w2: {name: "doc-check-pr-123", transition: "start", statuses: ["running"]}
	}
}'
expect_calls 1 "POST /api/v2/workspaces/w1/builds"
expect_calls 0 "POST /api/v2/workspaces/w2/builds"

run_case lookup-fails "https://github.com/coder/coder/pull/12" '{
	fail: ["/api/v2/chats"],
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["running"]}}
}'
expect_output "::warning::Could not look up the review chat for coder/coder#12."
expect_calls 1 "POST /api/v2/workspaces/w1/builds"

run_case workspace-lookup-fails "https://github.com/coder/coder/pull/12" '{
	fail: ["/api/v2/workspaces"],
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["running"]}}
}'
expect_output "::warning::Could not look up workspace doc-check-pr-12."

run_case workspace-read-fails "https://github.com/coder/coder/pull/12" '{
	list: ["r1"],
	fail: ["/api/v2/workspaces/w1"],
	chats: {r1: {status: "waiting", workspace_id: "w1", children: [], queued: []}},
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["running"]}}
}'
expect_output "::warning::Could not read workspace w1."

run_case stop-fails "https://github.com/coder/coder/pull/12" '{
	fail: ["/api/v2/workspaces/w1/builds"],
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["running"]}}
}'
expect_output "::warning::Could not stop workspace doc-check-pr-12."

run_case queue-delete-fails "https://github.com/coder/coder/pull/12" '{
	list: ["r1"],
	fail: ["/api/v2/chats/r1/queue/11"],
	chats: {r1: {status: "running", workspace_id: null, children: [], queued: [11]}}
}'
expect_output "::warning::Could not drop queued message 11 on chat r1."

# A value written on Windows can carry a trailing carriage return.
run_case crlf-pr-url $'https://github.com/coder/coder/pull/12\r' '{
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["running"]}}
}'
expect_calls 1 "label=gh-target:coder/coder#12 "
expect_calls 1 "POST /api/v2/workspaces/w1/builds"

if [[ "${failures}" != 0 ]]; then
	echo "${failures} check(s) failed."
	exit 1
fi
echo "All doc-check-stop-review.sh cases pass."
