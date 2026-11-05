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

web_proxies_api() {
	curl -fsS --max-time 10 "$VC_WEB_URL/api/v2/buildinfo" | jq -e '.version' >/dev/null || {
		echo "$VC_WEB_URL does not proxy /api to the API server"
		return 1
	}
	echo "$VC_WEB_URL proxies /api"
}

vc_check owner owner
vc_check api-port port_owned "$VC_API_PORT"
vc_check web-port port_owned "$VC_WEB_PORT"
vc_check healthz healthz
vc_check build build_current
vc_check admin-session admin_session
vc_check web web_proxies_api

templates="$(vc_api /api/v2/templates 2>/dev/null | jq -r '[.[].name] | join(",")' 2>/dev/null || true)"
echo "INFO templates: ${templates:-none}"
echo "INFO web $VC_WEB_URL, api $VC_API_URL, admin $VC_ADMIN_EMAIL"
echo "doctor: $VC_PASS/$VC_TOTAL checks passed"
((VC_PASS == VC_TOTAL))
