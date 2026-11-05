#!/usr/bin/env bash
# Starts a Coder dev instance owned by this checkout and waits until it is
# usable. Usage: launch.sh [--fresh] [develop.sh flags] [-- coder server flags]
#   --fresh  passes --db-reset to develop.sh (destroys this checkout's .coderv2 database).
# Every other argument reaches develop.sh unchanged, so "launch.sh -- --verbose"
# passes --verbose to coder server.
set -euo pipefail
# shellcheck source=.agents/skills/verify-coder/scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

fresh=0
if [[ "${1:-}" == "--fresh" ]]; then
	fresh=1
	shift
fi

if [[ -f "$VC_RUN/env" ]]; then
	vc_load_run
	if vc_owns_pid "$VC_PID"; then
		vc_fail "instance already running (pid $VC_PID, $VC_WEB_URL), run doctor.sh or cleanup.sh"
	fi
	vc_fail "stale run state in $VC_RUN (pid $VC_PID is not ours), run cleanup.sh first"
fi
mkdir -p "$VC_RUN" "$VC_EVIDENCE"
# develop.log records the first user's password, and cleanup keeps it as
# evidence, so only the owner may read the state folder.
chmod 700 "$VC_STATE"
umask 022

# Build first so the ports are picked seconds before develop binds them, not
# after a build that can take half an hour. DEVELOP_IN_CODER must match what
# develop passes to make, or the prebuilt binary gets the wrong ldflags.
develop_in_coder=0
if [[ "${DEVELOP_IN_CODER:-}" == 1 || -n "${CODER_AGENT_URL:-}" ]]; then
	develop_in_coder=1
fi
# make tracks files, not CODER_BUILD_AGPL, so an existing enterprise binary
# would be reused for --agpl.
case "${CODER_BUILD_AGPL:-}" in
"" | 0 | false) ;;
*) vc_fail "CODER_BUILD_AGPL is not supported: make would reuse the existing enterprise binary" ;;
esac
for arg in "$@"; do
	case "$arg" in
	--) break ;;
	--agpl | --agpl=*) vc_fail "--agpl is not supported: make would reuse the existing enterprise binary" ;;
	--password | --password=*) vc_fail "--password is not supported: set CODER_DEV_ADMIN_PASSWORD so the journeys sign in with it too" ;;
	esac
done

# develop.sh binds every interface, so a database that this launch creates
# (--fresh, or no built-in Postgres data yet) never gets the published
# default password. The password file sits next to the data it belongs to and
# lives as long as that data.
password_file="$VC_CONFIG_DIR/.verify-coder-admin-password"
new_database=0
if ((fresh)) || [[ ! -d "$VC_CONFIG_DIR/postgres/data" ]]; then
	new_database=1
	password="${CODER_DEV_ADMIN_PASSWORD:-Vc-$(openssl rand -hex 16)}"
	if [[ "$password" == "SomeSecurePassword!" ]]; then
		vc_fail "a new database must not use the published default password: develop.sh listens on every interface"
	fi
elif [[ -n "${CODER_DEV_ADMIN_PASSWORD:-}" ]]; then
	password="$CODER_DEV_ADMIN_PASSWORD"
elif [[ -f "$password_file" ]]; then
	password="$(<"$password_file")"
else
	password="SomeSecurePassword!"
fi

bin="$(vc_coder_bin)"
echo "building build/.bin/develop and ${bin#"$VC_ROOT"/} (log: $VC_RUN/build.log)"
if ! (cd "$VC_ROOT" && DEVELOP_IN_CODER="$develop_in_coder" MAKE_TIMED=1 make -j build/.bin/develop "${bin#"$VC_ROOT"/}") >"$VC_RUN/build.log" 2>&1; then
	tail -n 30 "$VC_RUN/build.log" | cut -c1-300 >&2
	vc_fail "build failed, see $VC_RUN/build.log"
fi

pick_port() {
	local port="$1"
	while vc_port_busy "$port"; do
		port=$((port + 1))
	done
	echo "$port"
}
offset=$(($(cksum <<<"$VC_ROOT" | cut -d' ' -f1) % 50 * 20))
if [[ -n "${CODER_DEV_PORT:-}" ]]; then
	vc_port_busy "$CODER_DEV_PORT" && vc_fail "CODER_DEV_PORT $CODER_DEV_PORT is busy"
	api_port="$CODER_DEV_PORT"
else
	api_port="$(pick_port $((13000 + offset)))"
fi
if [[ -n "${CODER_DEV_WEB_PORT:-}" ]]; then
	vc_port_busy "$CODER_DEV_WEB_PORT" && vc_fail "CODER_DEV_WEB_PORT $CODER_DEV_WEB_PORT is busy"
	web_port="$CODER_DEV_WEB_PORT"
else
	web_port="$(pick_port $((18000 + offset)))"
fi

# The marker tells cleanup.sh that this harness created the folder.
# cleanup.sh --keep-data deletes it, so the folder then counts as the
# developer's.
if [[ ! -d "$VC_CONFIG_DIR" ]]; then
	mkdir -p "$VC_CONFIG_DIR"
	touch "$VC_CONFIG_DIR/.verify-coder-created"
fi

if ((new_database)); then
	(
		umask 077
		printf '%s\n' "$password" >"$password_file"
	)
fi
args=(--port "$api_port" --web-port "$web_port" --prometheus-port 0)
if ((fresh)); then
	args+=(--db-reset)
fi

# Provider keys are dropped so the instance never spends real tokens by
# itself (task naming calls Anthropic when a key is present).
log="$VC_RUN/develop.log"
# The password goes through the environment, not an argument, so it stays out
# of the process list.
CODER_DEV_ADMIN_PASSWORD="$password" env -u ANTHROPIC_API_KEY -u ANTHROPIC_AUTH_TOKEN -u ANTHROPIC_BASE_URL -u OPENAI_API_KEY -u OPENAI_BASE_URL \
	setsid "$VC_ROOT/scripts/develop.sh" "${args[@]}" "$@" >"$log" 2>&1 </dev/null &
pid=$!

# Journeys drive the production frontend that coder server serves from the
# embedded build (VC_WEB_URL, the API port). The Vite dev server
# (VC_VITE_URL) serves thousands of unbundled modules per page, and under host
# load one failed fetch broke a lazy route import with the app's error page.
# printf %q keeps paths with spaces or shell characters intact when lib.sh
# sources the file.
{
	printf 'VC_PID=%q\n' "$pid"
	printf 'VC_API_PORT=%q\n' "$api_port"
	printf 'VC_WEB_PORT=%q\n' "$web_port"
	printf 'VC_API_URL=%q\n' "http://127.0.0.1:$api_port"
	printf 'VC_WEB_URL=%q\n' "http://127.0.0.1:$api_port"
	printf 'VC_VITE_URL=%q\n' "http://127.0.0.1:$web_port"
	printf 'VC_LOG=%q\n' "$log"
	printf 'VC_BROWSER_SESSION=%q\n' "verify-coder-$api_port"
	printf 'VC_STARTED_AT=%q\n' "$(date -u +%FT%TZ)"
	printf 'VC_ADMIN_PASSWORD=%q\n' "$password"
} >"$VC_RUN/env"
echo "started develop.sh as pid $pid (api $api_port, web $web_port, log: $log)"

deadline=$((SECONDS + ${VERIFY_CODER_TIMEOUT:-900}))
until grep -q 'Coder is now running in development mode' "$log"; do
	if ! kill -0 "$pid" 2>/dev/null; then
		tail -n 40 "$log" | cut -c1-300 >&2
		vc_fail "develop.sh exited before it was ready, see $log, then run cleanup.sh"
	fi
	if ((SECONDS >= deadline)); then
		tail -n 20 "$log" | cut -c1-300 >&2
		vc_fail "not ready after ${VERIFY_CODER_TIMEOUT:-900}s, instance left running for diagnosis, run cleanup.sh when done"
	fi
	sleep 2
done

grep -E 'template created in org|starter template setup failed|created member user' "$log" | cut -c1-200 || true
exec "$(dirname "${BASH_SOURCE[0]}")/doctor.sh"
