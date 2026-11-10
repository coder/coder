#!/usr/bin/env bash
# Journey for features/workspaces.md: the admin creates one Docker workspace
# with the CLI and one in the web UI, proves each agent connected, runs a
# command over SSH and in the web terminal, stops and starts each workspace
# (CLI and UI), and deletes each one (UI dialog and CLI).
# Usage: workspace.sh   (prints one line per check and a count line)
set -euo pipefail
scripts="$(dirname "${BASH_SOURCE[0]}")/../scripts"
# shellcheck source=.agents/skills/verify-coder/scripts/lib.sh
source "$scripts/lib.sh"
vc_load_run
"$scripts/doctor.sh" >/dev/null || vc_fail "doctor failed, run $scripts/doctor.sh"
docker info >/dev/null 2>&1 || vc_fail "docker info failed, this journey needs a local Docker daemon"

out="$(vc_new_evidence workspace)"
VC_CHECK_LOG="$out/steps.log"
VC_RESULT_FILE="$out/result.txt"
vc_on_fail() {
	vc_capture_failure "$out" "$1"
}
# The random suffix keeps two runs that start in the same second apart.
stamp="$(date +%s)-$RANDOM"
cli_ws="vcc-$stamp"
ui_ws="vcu-$stamp"
# cleanup.sh deletes only recorded workspaces, so a failed run leaves none.
vc_record_workspace "admin/$cli_ws"
vc_record_workspace "admin/$ui_ws"

ws_json() {
	vc_api "/api/v2/users/admin/workspace/$1"
}

# wait_state polls the API until "<transition>/<status>" matches, for example
# start/running or stop/stopped.
wait_state() {
	local ws="$1" want="$2" got="" i
	for i in $(seq 150); do
		got="$(ws_json "$ws" | jq -r '"\(.latest_build.transition)/\(.latest_build.status)"')"
		if [[ "$got" == "$want" ]]; then
			return 0
		fi
		if [[ "$got" == */failed ]]; then
			break
		fi
		sleep 2
	done
	echo "$ws is $got, wanted $want after ${i}x2s, see grep level=ERROR \$VC_LOG"
	return 1
}

wait_gone() {
	local ws="$1" code i
	for i in $(seq 150); do
		code="$(vc_api_status "/api/v2/users/admin/workspace/$ws")"
		if [[ "$code" == 404 ]]; then
			return 0
		fi
		sleep 2
	done
	echo "GET workspace $ws still answers $code"
	return 1
}

container_count() {
	docker ps --filter "name=^coder-admin-$1\$" --format '{{.Names}}' | wc -l
}

# During a build the workspace page scrolls to follow the build log, which
# moves the header buttons under the sticky navbar. Scroll back up first, as a
# user would.
click_header() {
	vc_ab wait "$1" >/dev/null
	vc_ab eval 'window.scrollTo(0, 0)' >/dev/null
	vc_ab click "$1" >/dev/null
}

ui_status() {
	vc_ab wait --fn "[...document.querySelectorAll('*')].some(e => e.textContent.trim() === 'Workspace status: $1')" >/dev/null
}

cli_create() {
	vc_coder create "$cli_ws" --template docker --parameter 'jetbrains_ides=[]' --yes </dev/null >"$out/01-cli-create.txt" 2>&1
	wait_state "$cli_ws" start/running
	echo "coder create $cli_ws exited 0 and the build is start/running"
}

cli_agent() {
	vc_wait_agent "admin/$cli_ws"
	ws_json "$cli_ws" | jq '{name, status: .latest_build.status, agents: [.latest_build.resources[].agents[]? | {name, status, lifecycle_state}]}' >"$out/02-cli-api.json"
	[[ "$(container_count "$cli_ws")" == 1 ]] || {
		echo "docker ps has no running coder-admin-$cli_ws"
		return 1
	}
	echo "agent main is connected and ready, container coder-admin-$cli_ws runs"
}

cli_ssh() {
	local got
	got="$(vc_coder ssh "$cli_ws" -- echo verify-ssh-ok 2>/dev/null)"
	echo "$got" >"$out/03-cli-ssh.txt"
	[[ "$got" == verify-ssh-ok ]] || {
		echo "coder ssh printed '$got'"
		return 1
	}
	echo "coder ssh $cli_ws -- echo printed verify-ssh-ok"
}

cli_stop_start() {
	vc_coder stop "$cli_ws" --yes </dev/null >"$out/04-cli-stop.txt" 2>&1
	wait_state "$cli_ws" stop/stopped
	[[ "$(container_count "$cli_ws")" == 0 ]] || {
		echo "the container still runs after coder stop"
		return 1
	}
	vc_coder start "$cli_ws" --yes </dev/null >"$out/05-cli-start.txt" 2>&1
	wait_state "$cli_ws" start/running
	[[ "$(container_count "$cli_ws")" == 1 ]] || {
		echo "no container runs after coder start"
		return 1
	}
	echo "coder stop and coder start moved $cli_ws to stop/stopped and back to start/running"
}

ui_create() {
	vc_browser_login "$VC_ADMIN_EMAIL" "$VC_ADMIN_PASSWORD"
	vc_ab open "$VC_WEB_URL/templates/docker/workspace" >/dev/null
	vc_ab wait --fn 'document.title === "Create workspace - Coder"' >/dev/null
	vc_ab wait 'form[aria-label="Create workspace form"]' >/dev/null
	vc_ab wait --load networkidle >/dev/null
	vc_ab find label "Workspace name" fill "$ui_ws" >/dev/null
	vc_ab screenshot "$out/06-ui-create-form.png" >/dev/null
	vc_ab find role button click --name "Create workspace" >/dev/null
	vc_ab wait --url "**/@admin/$ui_ws*" >/dev/null
	wait_state "$ui_ws" start/running
	echo "the form opened /@admin/$ui_ws and the build is start/running"
}

ui_agent() {
	vc_wait_agent "admin/$ui_ws"
	vc_ab wait '[data-testid=agent-status-ready]' >/dev/null
	ui_status Running
	vc_ab screenshot "$out/07-ui-running.png" >/dev/null
	echo "the page shows agent-status-ready and Workspace status: Running"
}

ui_terminal() {
	local got
	vc_ab find role link click --name "Terminal" >/dev/null
	vc_ab wait --url "**/@admin/$ui_ws.main/terminal*" >/dev/null
	vc_ab wait '.xterm' >/dev/null
	sleep 2
	vc_ab click '.xterm' >/dev/null
	vc_ab keyboard type "echo verify-web-terminal-ok > /tmp/verify-web-terminal" >/dev/null
	vc_ab press Enter >/dev/null
	# Under load the terminal can take a while to run the command.
	for _ in $(seq 30); do
		got="$(vc_coder ssh "$ui_ws" -- cat /tmp/verify-web-terminal 2>/dev/null)" || true
		[[ "$got" != verify-web-terminal-ok ]] || break
		sleep 2
	done
	vc_ab screenshot "$out/08-ui-terminal.png" >/dev/null
	[[ "$got" == verify-web-terminal-ok ]] || {
		echo "the file the terminal wrote holds '$got'"
		return 1
	}
	echo "the terminal tab ran a command, coder ssh read back verify-web-terminal-ok"
}

ui_stop_start() {
	local tab
	tab="$(vc_ab tab list | grep -o '\[t[0-9]*\] admin/'"$ui_ws"' - Coder' | head -n 1 | grep -o 't[0-9]*')"
	vc_ab tab "$tab" >/dev/null
	click_header '[data-testid=workspace-stop-button]'
	wait_state "$ui_ws" stop/stopped
	ui_status Stopped
	vc_ab screenshot "$out/09-ui-stopped.png" >/dev/null
	[[ "$(container_count "$ui_ws")" == 0 ]] || {
		echo "the container still runs after stop"
		return 1
	}
	click_header '[data-testid=workspace-start]'
	wait_state "$ui_ws" start/running
	ui_status Running
	[[ "$(container_count "$ui_ws")" == 1 ]] || {
		echo "no container runs after start"
		return 1
	}
	echo "Stop and Start buttons moved the build to stop/stopped and back to start/running"
}

ui_delete() {
	local id
	id="$(ws_json "$ui_ws" | jq -r '.id')"
	click_header '[data-testid=workspace-options-button]'
	vc_ab wait '[role=menu]' >/dev/null
	vc_ab find role menuitem click --name "Delete…" >/dev/null
	vc_ab wait '[role=dialog] input' >/dev/null
	vc_ab fill '[role=dialog] input' "$ui_ws" >/dev/null
	vc_ab screenshot "$out/10-ui-delete-dialog.png" >/dev/null
	vc_ab find role button click --name "Delete" --exact >/dev/null
	wait_gone "$ui_ws"
	[[ "$(docker ps -a --filter "name=^coder-admin-$ui_ws\$" --format '{{.Names}}' | wc -l)" == 0 ]] || {
		echo "a container for $ui_ws is left"
		return 1
	}
	if docker volume inspect "coder-$id-home" >/dev/null 2>&1; then
		echo "volume coder-$id-home is left"
		return 1
	fi
	echo "the dialog deleted $ui_ws: API 404, no container, no home volume"
}

cli_delete() {
	local id
	id="$(ws_json "$cli_ws" | jq -r '.id')"
	vc_coder delete "$cli_ws" --yes >"$out/11-cli-delete.txt" 2>&1
	wait_gone "$cli_ws"
	[[ "$(docker ps -a --filter "name=^coder-admin-$cli_ws\$" --format '{{.Names}}' | wc -l)" == 0 ]] || {
		echo "a container for $cli_ws is left"
		return 1
	}
	if docker volume inspect "coder-$id-home" >/dev/null 2>&1; then
		echo "volume coder-$id-home is left"
		return 1
	fi
	echo "coder delete $cli_ws: API 404, no container, no home volume"
}

vc_check cli-create cli_create
vc_check cli-agent cli_agent
vc_check cli-ssh cli_ssh
vc_check cli-stop-start cli_stop_start
vc_check ui-create ui_create
vc_check ui-agent ui_agent
vc_check ui-terminal ui_terminal
vc_check ui-stop-start ui_stop_start
vc_check ui-delete ui_delete
vc_check cli-delete cli_delete
echo "workspace: $VC_PASS/$VC_TOTAL checks passed, evidence in $out" | tee -a "$out/result.txt"
((VC_PASS == VC_TOTAL))
