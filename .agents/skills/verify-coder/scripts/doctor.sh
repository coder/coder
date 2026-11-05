#!/usr/bin/env bash
# Read-only check that the recorded instance is ours, current and usable.
# Prints one line per check and a count line, exits 1 on any failure.
set -euo pipefail
# shellcheck source=.agents/skills/verify-coder/scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
vc_load_run

owner() {
	vc_owns_pid "$VC_PID" || {
		echo "pid $VC_PID is not this checkout's build/.bin/develop session leader"
		return 1
	}
	echo "pid $VC_PID runs build/.bin/develop"
}

port_owned() {
	local port="$1" listener sid
	listener="$(vc_port_listener_pid "$port")"
	[[ -n "$listener" ]] || {
		echo "nothing listens on $port"
		return 1
	}
	sid="$(ps -o sid= -p "$listener" | tr -d ' ')"
	[[ "$sid" == "$VC_PID" ]] || {
		echo "port $port belongs to pid $listener (session $sid), not to session $VC_PID"
		return 1
	}
	echo "port $port held by pid $listener in our session"
}

healthz() {
	local body
	body="$(curl -fsS --max-time 5 "$VC_API_URL/healthz")"
	[[ "$body" == "OK" ]] || {
		echo "healthz answered '$body'"
		return 1
	}
	echo "$VC_API_URL/healthz OK"
}

# Dev versions end in "-devel+<short sha>" (scripts/version.sh).
build_current() {
	local version sha head
	version="$(curl -fsS --max-time 5 "$VC_API_URL/api/v2/buildinfo" | jq -r '.version')"
	sha="${version##*+}"
	head="$(git -C "$VC_ROOT" rev-parse HEAD)"
	[[ "$version" == *-devel+* && -n "$sha" && "$head" == "$sha"* ]] || {
		echo "server reports $version, HEAD is ${head:0:12}: relaunch"
		return 1
	}
	echo "server $version matches HEAD"
}

admin_session() {
	local user
	user="$(vc_api /api/v2/users/me | jq -r '.username')"
	[[ "$user" == "admin" ]] || {
		echo "session in $VC_CONFIG_DIR/session resolves to '$user'"
		return 1
	}
	echo "CLI session is the admin user"
}

embedded_ui() {
	curl -fsS --max-time 10 "$VC_WEB_URL/login" | grep -q '/assets/index-' || {
		echo "$VC_WEB_URL/login does not serve the embedded production frontend"
		return 1
	}
	echo "$VC_WEB_URL serves the embedded frontend"
}

vite_proxies_api() {
	curl -fsS --max-time 10 "$VC_VITE_URL/api/v2/buildinfo" | jq -e '.version' >/dev/null || {
		echo "$VC_VITE_URL does not proxy /api to the API server"
		return 1
	}
	echo "$VC_VITE_URL proxies /api"
}

starter_template() {
	if ! docker info >/dev/null 2>&1; then
		echo "docker unavailable, so develop.sh creates no docker template"
		return 0
	fi
	vc_api /api/v2/templates | jq -e '[.[].name] | index("docker")' >/dev/null || {
		echo "docker works but the docker template is missing: grep 'starter template' \$VC_LOG, then relaunch"
		return 1
	}
	echo "docker template exists"
}

vc_check owner owner
vc_check api-port port_owned "$VC_API_PORT"
vc_check web-port port_owned "$VC_WEB_PORT"
vc_check healthz healthz
vc_check build build_current
vc_check admin-session admin_session
vc_check embedded-ui embedded_ui
vc_check vite vite_proxies_api
vc_check starter-template starter_template

echo "INFO web UI and API $VC_WEB_URL, Vite dev server $VC_VITE_URL, admin $VC_ADMIN_EMAIL"
echo "doctor: $VC_PASS/$VC_TOTAL checks passed"
((VC_PASS == VC_TOTAL))
