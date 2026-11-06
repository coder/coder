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
case "$*" in
"") ;;
--keep-data) keep_data=1 ;;
*) vc_fail "usage: cleanup.sh [--keep-data]" ;;
esac
if [[ ! -f "$VC_RUN/env" ]]; then
	echo "cleanup: nothing recorded in $VC_RUN"
	exit 0
fi
vc_load_run
VC_CHECK_LOG="$VC_EVIDENCE/$(date -u +%Y%m%dT%H%M%SZ)-cleanup.log"

# Only workspaces that a journey or vc_record_workspace recorded belong to the
# harness. Any other workspace can be a developer's own work in the same
# .coderv2 or come from another client, so it is listed and kept. When this
# cleanup removes a .coderv2 that launch.sh created, its database goes too,
# so every workspace in it is deleted first and leaves no container behind.
delete_workspaces() {
	if ! vc_owns_pid "$VC_PID" || ! curl -fsS --max-time 5 "${VC_API_URL:-http://127.0.0.1:0}/healthz" >/dev/null 2>&1; then
		local left
		left="$(vc_docker_since "$VC_STARTED_AT")" || {
			echo "server is down and docker cannot list containers or volumes, run cleanup.sh again when it can"
			return 1
		}
		if [[ -n "$left" ]]; then
			echo "server is down, coder-* items created since the launch remain: $left(check they are yours, then docker rm -f the containers and docker volume rm the volumes)"
			return 1
		fi
		echo "server is down, and no coder-* container or volume was created since the launch"
		return 0
	fi
	local ws status kept deleted=()
	if [[ -f "$VC_CONFIG_DIR/.verify-coder-created" ]] && ((!keep_data)); then
		vc_coder list --all --output json 2>/dev/null |
			jq -r '.[] | "\(.owner_name)/\(.name)"' >>"$VC_RUN/workspaces"
	fi
	if [[ -f "$VC_RUN/workspaces" ]]; then
		while read -r ws; do
			[[ -n "$ws" ]] || continue
			status="$(vc_api_status "/api/v2/users/${ws%%/*}/workspace/${ws#*/}")"
			if [[ "$status" == 404 ]]; then
				continue
			fi
			[[ "$status" == 200 ]] || {
				echo "GET workspace $ws answered $status, run cleanup.sh again"
				return 1
			}
			vc_coder delete "$ws" --yes >>"$VC_LOG" 2>&1 || {
				echo "coder delete $ws failed, see $VC_LOG"
				return 1
			}
			deleted+=("$ws")
		done < <(sort -u "$VC_RUN/workspaces")
	fi
	kept="$(vc_coder list --all --output json 2>/dev/null | jq -r '[.[] | "\(.owner_name)/\(.name)"] | join(" ")')"
	echo "deleted: ${deleted[*]:-none}"
	echo "kept, not recorded by the harness: ${kept:-none}"
	echo "deleted ${#deleted[@]} workspaces, kept $(wc -w <<<"$kept") not recorded by the harness, names in $(basename "$VC_CHECK_LOG")"
}

close_browser() {
	if [[ -z "${VC_BROWSER_SESSION:-}" ]]; then
		echo "no browser session, develop reported no ports"
		return 0
	fi
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

ports_free() {
	local port busy=()
	if [[ -z "${VC_API_PORT:-}" ]]; then
		echo "develop reported no ports, so no port of this run was bound"
		return 0
	fi
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
	for f in develop-setup develop build; do
		if [[ -f "$VC_RUN/$f.log" ]]; then
			mv "$VC_RUN/$f.log" "$VC_EVIDENCE/$stamp-$f.log"
			kept+=("$stamp-$f.log")
		fi
	done
	rm -rf "$VC_RUN"
	echo "moved ${kept[*]:-no logs} to the evidence folder, which keeps every journey and hand-driven folder"
}

# postgres_running succeeds while Postgres runs from this checkout's .coderv2.
# Postgres keeps postmaster.pid in its data folder while it runs. It can
# outlive a crashed coder server, or belong to a plain develop.sh run on the
# same folder. A pid that cleanup cannot identify counts as running.
postgres_running() {
	local pid exe
	pid="$(head -n 1 "$VC_CONFIG_DIR/postgres/data/postmaster.pid" 2>/dev/null)" || return 1
	[[ "$pid" =~ ^[0-9]+$ && -d "/proc/$pid" ]] || return 1
	exe="$(readlink "/proc/$pid/exe")" || return 0
	[[ "${exe##*/}" == postgres ]]
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
	# The harness's server is stopped by now, so a Postgres that still runs
	# was left by a crashed server or belongs to a plain develop.sh run.
	if postgres_running; then
		echo "Postgres pid $(head -n 1 "$VC_CONFIG_DIR/postgres/data/postmaster.pid") still runs from $VC_CONFIG_DIR, so it stays: if no develop.sh uses that folder, stop it with kill -INT <pid> and run cleanup.sh again, or run cleanup.sh --keep-data"
		return 1
	fi
	# The marker goes last, so a removal that fails part way still counts as
	# the harness's data on the next try.
	find "$VC_CONFIG_DIR" -mindepth 1 -maxdepth 1 ! -name .verify-coder-created -exec rm -rf {} +
	rm -f "$VC_CONFIG_DIR/.verify-coder-created"
	rmdir "$VC_CONFIG_DIR" || {
		touch "$VC_CONFIG_DIR/.verify-coder-created"
		echo "could not remove $VC_CONFIG_DIR, its marker is back, run cleanup.sh again"
		return 1
	}
	echo "removed $VC_CONFIG_DIR (launch.sh created it)"
}

vc_check workspaces delete_workspaces
vc_check browser close_browser
vc_check helpers stop_helpers
vc_check develop vc_stop_develop "$VC_PID"
vc_check ports ports_free
# The data goes before the run state, so a failed removal keeps the state
# for a retry.
if ((VC_PASS == VC_TOTAL)); then
	vc_check data remove_data
fi
if ((VC_PASS == VC_TOTAL)); then
	vc_check logs archive_logs
fi
echo "cleanup: $VC_PASS/$VC_TOTAL steps passed, evidence in $VC_EVIDENCE"
((VC_PASS == VC_TOTAL))
