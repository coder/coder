#!/usr/bin/env bash
# Tears down what launch.sh started: workspaces in the instance, the browser
# session, the develop process session, and the run state. Evidence and the
# develop log move to build/verify-coder/evidence and are never deleted.
# Usage: cleanup.sh [--keep-data]
#   --keep-data  keep .coderv2 even when launch.sh created it, and from then on
#                treat it as the developer's (later cleanups keep it too).
set -euo pipefail
here="$(dirname "${BASH_SOURCE[0]}")"
# shellcheck source=.agents/skills/verify-coder/scripts/lib.sh
source "$here/lib.sh"

keep_data=0
if [[ "${1:-}" == "--keep-data" ]]; then
	keep_data=1
fi
if [[ ! -f "$VC_RUN/env" ]]; then
	echo "cleanup: nothing recorded in $VC_RUN"
	exit 0
fi
vc_load_run

# Only workspaces created after this launch belong to the harness. Older ones
# can be a developer's own work in the same .coderv2, so they are listed and
# kept.
delete_workspaces() {
	if ! vc_owns_pid "$VC_PID" || ! curl -fsS --max-time 5 "$VC_API_URL/healthz" >/dev/null 2>&1; then
		echo "server is down, skipped (check docker ps for coder-* containers by hand)"
		return 0
	fi
	local list ws kept n=0
	list="$("$here/coder.sh" list --all --output json 2>/dev/null)"
	kept="$(jq -r --arg t "$VC_STARTED_AT" '[.[] | select(.created_at | sub("\\.[0-9]+"; "") | fromdateiso8601 < ($t | fromdateiso8601)) | "\(.owner_name)/\(.name)"] | join(" ")' <<<"$list")"
	while read -r ws; do
		[[ -n "$ws" ]] || continue
		"$here/coder.sh" delete "$ws" --yes >>"$VC_LOG" 2>&1 || {
			echo "coder delete $ws failed, see $VC_LOG"
			return 1
		}
		n=$((n + 1))
	done < <(jq -r --arg t "$VC_STARTED_AT" '.[] | select(.created_at | sub("\\.[0-9]+"; "") | fromdateiso8601 >= ($t | fromdateiso8601)) | "\(.owner_name)/\(.name)"' <<<"$list")
	echo "deleted $n workspaces created since $VC_STARTED_AT, kept older: ${kept:-none}"
}

close_browser() {
	if agent-browser session list --json | jq -e --arg s "$VC_BROWSER_SESSION" \
		'.data.sessions[] | select((if type == "string" then . else .name end) == $s)' >/dev/null; then
		agent-browser --session "$VC_BROWSER_SESSION" close >/dev/null
		echo "closed session $VC_BROWSER_SESSION"
	else
		echo "no session $VC_BROWSER_SESSION"
	fi
}

# Journeys record helpers they start (such as llm-mock) as $VC_RUN/*.pid.
# Stop only pids that still run this checkout's coder binary.
stop_helpers() {
	local f pid n=0 bin
	bin="$(vc_coder_bin)"
	for f in "$VC_RUN"/*.pid; do
		[[ -f "$f" ]] || continue
		pid="$(cat "$f")"
		if [[ "$(readlink "/proc/$pid/exe" 2>/dev/null)" == "$bin"* ]]; then
			kill "$pid" 2>/dev/null || true
			n=$((n + 1))
		fi
		rm -f "$f"
	done
	echo "stopped $n helpers"
}

stop_develop() {
	if [[ -d "/proc/$VC_PID" ]] && ! vc_owns_pid "$VC_PID"; then
		echo "pid $VC_PID now belongs to another program, left it alone"
		return 1
	fi
	if vc_owns_pid "$VC_PID"; then
		kill -INT "$VC_PID"
		for _ in $(seq 60); do
			[[ -d "/proc/$VC_PID" ]] || break
			sleep 1
		done
	fi
	# A live process whose session id is VC_PID can only be a leftover of our
	# session: Linux does not reuse a pid while a session still refers to it.
	local left
	mapfile -t left < <(vc_session_pids "$VC_PID")
	if ((${#left[@]})); then
		kill -TERM "${left[@]}" 2>/dev/null || true
		sleep 5
		mapfile -t left < <(vc_session_pids "$VC_PID")
		if ((${#left[@]})); then
			kill -KILL "${left[@]}" 2>/dev/null || true
		fi
	fi
	mapfile -t left < <(vc_session_pids "$VC_PID")
	((${#left[@]} == 0)) || {
		echo "processes still running: ${left[*]}"
		return 1
	}
	echo "develop session $VC_PID stopped"
}

ports_free() {
	local port busy=()
	for port in "$VC_API_PORT" "$VC_WEB_PORT"; do
		if vc_port_busy "$port"; then
			busy+=("$port")
		fi
	done
	((${#busy[@]} == 0)) || {
		echo "still listening: ${busy[*]}"
		return 1
	}
	echo "ports $VC_API_PORT and $VC_WEB_PORT are free"
}

archive_logs() {
	local stamp f kept=()
	stamp="$(date -u +%Y%m%dT%H%M%SZ)"
	mkdir -p "$VC_EVIDENCE"
	for f in develop build; do
		if [[ -f "$VC_RUN/$f.log" ]]; then
			mv "$VC_RUN/$f.log" "$VC_EVIDENCE/$stamp-$f.log"
			kept+=("$stamp-$f.log")
		fi
	done
	rm -rf "$VC_RUN"
	echo "moved ${kept[*]:-no logs} to $VC_EVIDENCE, journey and hand-driven folders kept"
}

# --keep-data hands the folder over: without the marker, a later cleanup keeps
# it, because the folder can then hold a developer's own work.
remove_data() {
	if ((keep_data)); then
		rm -f "$VC_CONFIG_DIR/.verify-coder-created"
		echo "kept $VC_CONFIG_DIR, a later cleanup keeps it too"
		return 0
	fi
	if [[ ! -f "$VC_CONFIG_DIR/.verify-coder-created" ]]; then
		echo "kept $VC_CONFIG_DIR (launch.sh did not create it)"
		return 0
	fi
	rm -rf "$VC_CONFIG_DIR"
	echo "removed $VC_CONFIG_DIR (launch.sh created it)"
}

vc_check workspaces delete_workspaces
vc_check browser close_browser
vc_check helpers stop_helpers
vc_check develop stop_develop
vc_check ports ports_free
if ((VC_PASS == VC_TOTAL)); then
	vc_check logs archive_logs
	vc_check data remove_data
fi
echo "cleanup: $VC_PASS/$VC_TOTAL steps passed, evidence in $VC_EVIDENCE"
((VC_PASS == VC_TOTAL))
