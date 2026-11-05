#!/usr/bin/env bash
# Shared paths and ownership checks for the verify-coder skill scripts.
set -euo pipefail

VC_ROOT="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)"
VC_STATE="${VERIFY_CODER_STATE:-$VC_ROOT/build/verify-coder}"
VC_RUN="$VC_STATE/run"
VC_EVIDENCE="$VC_STATE/evidence"
VC_CONFIG_DIR="$VC_ROOT/.coderv2"
VC_ADMIN_EMAIL="admin@coder.com"
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
# VC_CHECK_LOG keeps full output, VC_RESULT_FILE the lines. A caller's
# vc_on_fail <name> function, when defined, runs after a failed step.
vc_check() {
	local _vc_name="$1" _vc_out _vc_rc _vc_status=FAIL
	shift
	VC_TOTAL=$((VC_TOTAL + 1))
	set +e
	_vc_out="$(
		set -e
		"$@" 2>&1
	)"
	_vc_rc=$?
	set -e
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

vc_load_run() {
	[[ -f "$VC_RUN/env" ]] || vc_fail "no instance recorded in $VC_RUN/env, run launch.sh first"
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

# vc_api GETs an API path as the admin user without printing the token.
vc_api() {
	curl -fsS --max-time 10 -H "Coder-Session-Token: $(vc_session_token)" "$VC_API_URL$1"
}

# vc_capture_failure saves the page and URL a failed browser step left.
vc_capture_failure() {
	local dir="$1" name="$2"
	vc_ab screenshot "$dir/fail-$name.png"
	echo "url at failure: $(vc_ab get url)" >>"$dir/steps.log"
}

# vc_ab runs agent-browser in this instance's own session.
vc_ab() {
	agent-browser --session "$VC_BROWSER_SESSION" "$@"
}

# vc_browser_login signs in through the real login form and waits for the
# workspaces page, the same synchronization point as site/e2e/helpers.ts login.
# The Vite dev server sets a fixed csrf_token cookie that matches the site's
# fixed CSRF header (site/vite.config.mts). After a cookie clear, a cached
# /login sets no cookie, coderd issues a different one, and every POST then
# fails with a CSRF 400. The unique query forces a fresh response.
vc_browser_login() {
	local email="${1:-$VC_ADMIN_EMAIL}" password="${2:-$VC_ADMIN_PASSWORD}"
	vc_ab cookies clear >/dev/null
	vc_ab open "$VC_WEB_URL/login?verify=$(date +%s%N)" >/dev/null
	vc_ab find label "Email" fill "$email" >/dev/null
	vc_ab find label "Password" fill "$password" >/dev/null
	# Sign-in renders a client-side redirect and also sets location.href
	# (site/src/pages/LoginPage/LoginPage.tsx), so the workspaces page can
	# appear before the hard reload replaces it. The marker lives only on the
	# old window, so the wait passes only after the reload.
	vc_ab eval 'window.__verifyCoderBeforeSignIn = true' >/dev/null
	vc_ab find role button click --name "Sign In" >/dev/null
	vc_ab wait --url "**/workspaces" >/dev/null
	vc_ab wait --fn 'window.__verifyCoderBeforeSignIn !== true && document.title === "Workspaces - Coder"' >/dev/null
}
