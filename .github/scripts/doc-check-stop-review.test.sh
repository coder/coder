#!/usr/bin/env bash
# Tests for doc-check-stop-review.sh. A fake curl on PATH serves a small
# simulated Coder API from a JSON state file and logs every call, and a
# fake sleep returns at once, so each case runs the real script in well
# under a second.
#
# Run: bash .github/scripts/doc-check-stop-review.test.sh

set -euo pipefail

SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/doc-check-stop-review.sh"
WORK=$(mktemp -d)
trap 'rm -rf "${WORK}"' EXIT

mkdir -p "${WORK}/bin"
cat >"${WORK}/bin/sleep" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

# The fake API. State lives in ${STATE}/state.json:
#   chats       id -> {status, archived, workspace_id, children, queued}
#   list        chat IDs the label lookup returns
#   stuck       chat IDs that ignore interrupts
#   workspaces  id -> {name, transition, statuses}; each read takes the
#               next status and keeps the last
#   fail        paths that answer with an HTTP error
cat >"${WORK}/bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
method=GET data="" url="" query=()
while (($#)); do
	case "$1" in
	-X) method="$2"; shift 2 ;;
	-H) shift 2 ;;
	-d) data="$2"; shift 2 ;;
	--data-urlencode) query+=("$2"); shift 2 ;;
	*) [[ "$1" == http* ]] && url="$1"; shift ;;
	esac
done
path="${url#"${CODER_URL%/}"}"
path="${path%%\?*}"
echo "${method} ${path}${query[*]:+ ${query[*]}}${data:+ ${data}}" >>"${STATE}/calls"

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

chat_json() {
	jq -c --arg id "$1" '
		def one($id): .chats[$id] + {id: $id};
		one($id) + {children: [.chats[$id].children[] as $c | one($c) | del(.children)]}
	' "${st}"
}

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
	running) update --arg id "${id}" '.chats[$id].status = "interrupting"' ;;
	# An interrupted requires_action chat gets cancelled tool results and
	# runs again, as in chatstate.
	requires_action) update --arg id "${id}" '.chats[$id].status = "running"' ;;
	*) fail ;;
	esac
	echo '{}'
	;;
GET\ /api/v2/chats/*)
	id="${path#/api/v2/chats/}"
	chat_json "${id}"
	# The worker finishes an interruption after one read.
	update '.chats |= with_entries(if .value.status == "interrupting" then .value.status = "waiting" else . end)'
	;;
"GET /api/v2/workspaces")
	name="${query[0]#q=owner:me name:}"
	jq -c --arg n "${name}" '{workspaces: [.workspaces | to_entries[] | select(.value.name | contains($n)) | {id: .key, name: .value.name}]}' "${st}"
	;;
POST\ /api/v2/workspaces/*/builds)
	id="${path#/api/v2/workspaces/}"
	id="${id%/builds}"
	update --arg id "${id}" '.workspaces[$id] += {transition: "stop", statuses: ["stopping", "stopped"]}'
	echo '{}'
	;;
GET\ /api/v2/workspaces/*)
	id="${path#/api/v2/workspaces/}"
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

# run_case NAME PR_URL STATE_JSON runs the script against a fresh state.
run_case() {
	case_name="$1"
	export STATE="${WORK}/${case_name}"
	mkdir -p "${STATE}"
	: >"${STATE}/calls"
	# The state is a jq expression, so cases can leave keys unquoted.
	jq -n "{chats: {}, list: [], stuck: [], workspaces: {}, fail: []} + ($3)" >"${STATE}/state.json"
	local code=0
	OUT=$(PATH="${WORK}/bin:${PATH}" CODER_URL="https://coder.example.com/" CODER_TOKEN="token" \
		PR_URL="$2" WORKFLOW_NAME="AI Documentation Check" bash "${SCRIPT}" 2>&1) || code=$?
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
		r1: {status: "waiting", archived: false, workspace_id: "w1", children: ["c1"], queued: []},
		c1: {status: "waiting", archived: false, workspace_id: "w1", children: [], queued: []}
	},
	workspaces: {w1: {name: "doc-check-pr-12", transition: "stop", statuses: ["stopped"]}}
}'
expect_calls 1 "GET /api/v2/chats label=coder-agents-chat-action:true label=gh-target:coder/coder#12 label=coder-agents-chat-action-workflow:AI Documentation Check"
expect_calls 0 "/interrupt"
expect_calls 0 "/builds"
expect_no_output "::warning::"

run_case running-review "https://github.com/coder/coder/issues/12" '{
	list: ["r1"],
	chats: {
		r1: {status: "running", archived: false, workspace_id: "w1", children: ["c1", "c2"], queued: [11, 12]},
		c1: {status: "running", archived: false, workspace_id: "w1", children: [], queued: [21]},
		c2: {status: "waiting", archived: false, workspace_id: "w1", children: [], queued: []}
	},
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["running"]}}
}'
expect_before "DELETE /api/v2/chats/r1/queue/11" "POST /api/v2/chats/r1/interrupt"
expect_before "DELETE /api/v2/chats/r1/queue/12" "POST /api/v2/chats/r1/interrupt"
expect_before "DELETE /api/v2/chats/c1/queue/21" "POST /api/v2/chats/c1/interrupt"
expect_calls 1 "POST /api/v2/chats/r1/interrupt"
expect_calls 1 "POST /api/v2/chats/c1/interrupt"
expect_calls 0 "POST /api/v2/chats/c2/interrupt"
expect_before "POST /api/v2/chats/c1/interrupt" "POST /api/v2/workspaces/w1/builds"
expect_calls 1 'POST /api/v2/workspaces/w1/builds {"transition":"stop"}'
expect_output "Review chat r1 and its sub-agents are stopped."
expect_output "Stopped workspace doc-check-pr-12."
expect_no_output "::warning::"
[[ "$(state_of '[.chats[].status] | unique | join(",")')" == "waiting" ]] || fail_case "a chat is still active"
[[ "$(state_of '[.chats[].queued[]] | length')" == 0 ]] || fail_case "queued messages remain"

run_case requires-action "https://github.com/coder/coder/pull/12" '{
	list: ["r1"],
	chats: {
		r1: {status: "waiting", archived: false, workspace_id: null, children: ["c1"], queued: []},
		c1: {status: "requires_action", archived: false, workspace_id: null, children: [], queued: []}
	}
}'
expect_calls 2 "POST /api/v2/chats/c1/interrupt"
expect_output "Review chat r1 and its sub-agents are stopped."
expect_no_output "::warning::"

run_case stuck-chat "https://github.com/coder/coder/pull/12" '{
	list: ["r1"],
	stuck: ["r1"],
	chats: {r1: {status: "running", archived: false, workspace_id: "w1", children: [], queued: []}},
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["running"]}}
}'
expect_calls 3 "POST /api/v2/chats/r1/interrupt"
expect_output "::warning::Review chat r1 did not stop."
# The workspace still stops, which ends the stuck chat's tool calls.
expect_calls 1 "POST /api/v2/workspaces/w1/builds"

run_case build-in-progress "https://github.com/coder/coder/pull/12" '{
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["starting", "starting", "running"]}}
}'
expect_calls 1 "POST /api/v2/workspaces/w1/builds"
expect_before "GET /api/v2/workspaces/w1" "POST /api/v2/workspaces/w1/builds"
expect_output "Stopped workspace doc-check-pr-12."

run_case exact-workspace-name "https://github.com/coder/coder/pull/12" '{
	workspaces: {
		w1: {name: "doc-check-pr-12", transition: "start", statuses: ["running"]},
		w2: {name: "doc-check-pr-123", transition: "start", statuses: ["running"]}
	}
}'
expect_calls 1 "POST /api/v2/workspaces/w1/builds"
expect_calls 0 "POST /api/v2/workspaces/w2/builds"

run_case archived-chat "https://github.com/coder/coder/pull/12" '{
	list: ["r1"],
	chats: {r1: {status: "running", archived: true, workspace_id: null, children: [], queued: []}}
}'
expect_calls 0 "/interrupt"

run_case lookup-fails "https://github.com/coder/coder/pull/12" '{
	fail: ["/api/v2/chats"],
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["running"]}}
}'
expect_output "::warning::Could not look up the review chat for coder/coder#12."
expect_calls 1 "POST /api/v2/workspaces/w1/builds"

run_case stop-fails "https://github.com/coder/coder/pull/12" '{
	fail: ["/api/v2/workspaces/w1/builds"],
	workspaces: {w1: {name: "doc-check-pr-12", transition: "start", statuses: ["running"]}}
}'
expect_output "::warning::Could not stop workspace doc-check-pr-12."

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
