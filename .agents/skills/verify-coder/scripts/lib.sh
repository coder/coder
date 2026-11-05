#!/usr/bin/env bash
# Shared paths and ownership checks for the verify-coder skill scripts. It sets
# no shell options, so sourcing it in an interactive shell changes nothing.

VC_ROOT="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)"
VC_STATE="${VERIFY_CODER_STATE:-$VC_ROOT/build/verify-coder}"
VC_RUN="$VC_STATE/run"
VC_EVIDENCE="$VC_STATE/evidence"
VC_CONFIG_DIR="$VC_ROOT/.coderv2"
VC_ADMIN_EMAIL="admin@coder.com"
# vc_load_run replaces this with the password launch.sh gave the instance.
VC_ADMIN_PASSWORD="${CODER_DEV_ADMIN_PASSWORD:-SomeSecurePassword!}"
export VC_ROOT VC_STATE VC_RUN VC_EVIDENCE VC_CONFIG_DIR VC_ADMIN_EMAIL VC_ADMIN_PASSWORD

vc_fail() {
	echo "error: $*" >&2
	exit 1
}

VC_PASS=0
VC_TOTAL=0
# vc_check runs one step and prints "PASS|FAIL <name>: <last output line>".
# The step runs with errexit on, so any failing command fails it. It must not
# run inside an if test: bash ignores errexit there, and every step would pass.
# It works the same in a hand-driven shell and leaves its errexit state alone.
# VC_CHECK_LOG keeps full output, VC_RESULT_FILE the lines. A caller's
# vc_on_fail <name> function, when defined, runs after a failed step.
vc_check() {
	local _vc_name="$1" _vc_out _vc_rc _vc_status=FAIL _vc_errexit=0
	shift
	VC_TOTAL=$((VC_TOTAL + 1))
	if [[ $- == *e* ]]; then
		_vc_errexit=1
	fi
	set +e
	_vc_out="$(
		set -e
		"$@" 2>&1
	)"
	_vc_rc=$?
	if ((_vc_errexit)); then
		set -e
	fi
	if ((_vc_rc == 0)); then
		VC_PASS=$((VC_PASS + 1))
		_vc_status=PASS
	fi
	if [[ -n "${VC_CHECK_LOG:-}" ]]; then
		printf '== %s %s (exit %d)\n%s\n' "$_vc_status" "$_vc_name" "$_vc_rc" "$_vc_out" >>"$VC_CHECK_LOG"
	fi
	if ((_vc_rc != 0)) && declare -F vc_on_fail >/dev/null; then
		vc_on_fail "$_vc_name" >/dev/null 2>&1 || true
	fi
	_vc_out="${_vc_out##*$'\n'}"
	_vc_out="$_vc_status $_vc_name: ${_vc_out:0:160}"
	echo "$_vc_out"
	if [[ -n "${VC_RESULT_FILE:-}" ]]; then
		echo "$_vc_out" >>"$VC_RESULT_FILE"
	fi
}

# vc_start_develop starts scripts/develop.sh in its own process session and
# sets VC_DEVELOP_PID. Usage: vc_start_develop <log> <password> <0|1 in Coder>
# <develop args...>. develop.sh and coder server read many CODER_* variables
# (a database URL, a reset, ports, an env file), so none of the caller's reach
# them. Provider keys are dropped too, so the instance never spends real
# tokens by itself (task naming calls Anthropic when a key is present). The
# password goes through the environment, never an argument, so it stays out
# of the process list.
vc_start_develop() {
	local log="$1" password="$2" in_coder="$3" var
	shift 3
	local scrub=(-u ANTHROPIC_API_KEY -u ANTHROPIC_AUTH_TOKEN -u ANTHROPIC_BASE_URL -u OPENAI_API_KEY -u OPENAI_BASE_URL)
	while read -r var; do
		if [[ "$var" != CODER_DEV_ADMIN_PASSWORD ]]; then
			scrub+=(-u "$var")
		fi
	done < <(compgen -e | grep '^CODER_' || true)
	CODER_DEV_ADMIN_PASSWORD="$password" DEVELOP_IN_CODER="$in_coder" env "${scrub[@]}" \
		setsid "$VC_ROOT/scripts/develop.sh" "$@" >"$log" 2>&1 </dev/null &
	# shellcheck disable=SC2034 # launch.sh reads it.
	VC_DEVELOP_PID=$!
}

# vc_record_workspace marks <owner>/<name> as created by the harness, so
# cleanup.sh deletes it. Record a workspace before you create it.
vc_record_workspace() {
	echo "$1" >>"$VC_RUN/workspaces"
}

# vc_load_run returns instead of exiting, so a failure does not close an
# interactive shell. Scripts stop on it through their own errexit.
vc_load_run() {
	if [[ ! -f "$VC_RUN/env" ]]; then
		echo "error: no instance recorded in $VC_RUN/env, run launch.sh first" >&2
		return 1
	fi
	# shellcheck disable=SC1091
	source "$VC_RUN/env"
}

# vc_owns_pid succeeds when pid is the develop orchestrator that launch.sh
# started from this checkout as its own session leader.
vc_owns_pid() {
	local pid="$1" exe
	[[ -d "/proc/$pid" ]] || return 1
	exe="$(readlink "/proc/$pid/exe" 2>/dev/null)" || return 1
	[[ "$exe" == "$VC_ROOT/build/.bin/develop"* ]] || return 1
	[[ "$(ps -o sid= -p "$pid" | tr -d ' ')" == "$pid" ]]
}

vc_session_pids() {
	ps -eo pid=,sid= | awk -v s="$1" '$2 == s { print $1 }'
}

vc_port_listener_pid() {
	ss -Hltnp "sport = :$1" | { grep -o 'pid=[0-9]*' || true; } | head -n 1 | cut -d= -f2
}

vc_port_busy() {
	[[ -n "$(ss -Hltn "sport = :$1")" ]]
}

vc_coder_bin() {
	echo "$VC_ROOT/build/coder_$(go env GOOS)_$(go env GOARCH)"
}

vc_session_token() {
	cat "$VC_CONFIG_DIR/session"
}

# vc_curl runs curl as the admin user. The token header comes from a file
# descriptor, so it never shows in the process list.
vc_curl() {
	curl -H @<(printf 'Coder-Session-Token: %s\n' "$(vc_session_token)") "$@"
}

# vc_api GETs an API path as the admin user.
vc_api() {
	vc_curl -fsS --max-time 10 "$VC_API_URL$1"
}

# vc_api_status prints the HTTP status of a GET as the admin user, for checks
# such as "the workspace API answers 404".
vc_api_status() {
	vc_curl -sS -o /dev/null -w '%{http_code}' --max-time 10 "$VC_API_URL$1"
}

# vc_capture_failure saves the page, URL, page errors and console a failed
# browser step left.
vc_capture_failure() {
	local dir="$1" name="$2"
	vc_ab screenshot "$dir/fail-$name.png"
	echo "url at failure: $(vc_ab get url)" >>"$dir/steps.log"
	vc_ab errors >"$dir/fail-$name-errors.txt" 2>&1
	vc_ab console >"$dir/fail-$name-console.txt" 2>&1
	vc_ab network requests >"$dir/fail-$name-network.txt" 2>&1
	echo "document at failure: $(vc_ab eval 'document.readyState + ", root children " + (document.getElementById("root") || {}).childElementCount')" >>"$dir/steps.log"
}

# vc_ab runs agent-browser in this instance's own session. At load average
# near 30 on 8 cores, pages took longer than agent-browser's 25 s default to
# render, so waits and actions get 90 s unless overridden.
vc_ab() {
	AGENT_BROWSER_DEFAULT_TIMEOUT="${VERIFY_CODER_BROWSER_TIMEOUT_MS:-90000}" \
		agent-browser --session "$VC_BROWSER_SESSION" "$@"
}

# vc_browser_login signs in through the real login form and waits for the
# workspaces page, the same synchronization point as site/e2e/helpers.ts login.
# On the Vite dev server (VC_VITE_URL), a cached /login after a cookie clear
# sets no fixed csrf_token cookie (site/vite.config.mts), and every POST then
# fails with a CSRF 400. The unique query forces a fresh response there and
# costs nothing on the embedded frontend.
vc_browser_login() {
	local email="${1:-$VC_ADMIN_EMAIL}" password="${2:-$VC_ADMIN_PASSWORD}"
	vc_ab cookies clear >/dev/null
	vc_ab open "$VC_WEB_URL/login?verify=$(date +%s%N)" >/dev/null
	# The login page is lazy loaded and renders after the page load event.
	vc_ab wait 'input[name=password]' >/dev/null
	vc_ab find label "Email" fill "$email" >/dev/null
	# batch reads the command from stdin, so the password stays out of the
	# process list.
	VC_LOGIN_PASSWORD="$password" jq -cn '[["find", "label", "Password", "fill", env.VC_LOGIN_PASSWORD]]' |
		vc_ab batch --bail >/dev/null
	# Sign-in renders a client-side redirect and also sets location.href
	# (site/src/pages/LoginPage/LoginPage.tsx), so the workspaces page can
	# appear before the hard reload replaces it. The marker lives only on the
	# old window, so the wait passes only after the reload.
	vc_ab eval 'window.__verifyCoderBeforeSignIn = true' >/dev/null
	vc_ab find role button click --name "Sign In" >/dev/null
	vc_ab wait --url "**/workspaces" >/dev/null
	vc_ab wait --fn 'window.__verifyCoderBeforeSignIn !== true && document.title === "Workspaces - Coder"' >/dev/null
}
