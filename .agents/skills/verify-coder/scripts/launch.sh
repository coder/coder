#!/usr/bin/env bash
# Starts a Coder dev instance owned by this checkout and waits until it is
# usable. Usage: launch.sh [--fresh] [-- coder server flags]
#   --fresh  passes --db-reset to develop.sh (destroys this checkout's .coderv2 database).
# After --, only the server flags in server_flag_allowed reach coder server.
set -euo pipefail
# shellcheck source=.agents/skills/verify-coder/scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# The harness owns the database, ports, listen and access addresses and the
# admin password, so it forwards no develop.sh flag and only server flags
# that change none of them.
server_flag_allowed() {
	case "$1" in
	--verbose | --experiments=*) return 0 ;;
	*) return 1 ;;
	esac
}
fresh=0
server_flags=()
if [[ "${1:-}" == "--fresh" ]]; then
	fresh=1
	shift
fi
if (($#)); then
	[[ "$1" == -- ]] || vc_fail "$1 is not supported: launch.sh takes only --fresh and, after --, the server flags in server_flag_allowed"
	shift
	for arg in "$@"; do
		server_flag_allowed "$arg" || vc_fail "server flag $arg is not supported: add it to server_flag_allowed in launch.sh if it changes no database, port, address or password"
		server_flags+=("$arg")
	done
fi

# One launch at a time per checkout: two could otherwise pass the check below
# together, pick the same ports and overwrite each other's run state.
mkdir -p "$VC_STATE"
exec 9>"$VC_STATE/launch.lock"
flock -n 9 || vc_fail "another launch.sh runs in this checkout"

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
# would be reused for an AGPL build.
case "${CODER_BUILD_AGPL:-}" in
"" | 0 | false) ;;
*) vc_fail "CODER_BUILD_AGPL is not supported: make would reuse the existing enterprise binary" ;;
esac

# After setup the instance listens on every interface, so it never runs with
# the published default password. A database that this launch creates (--fresh,
# or no built-in Postgres data yet) gets a random one. Reused data needs the
# password it already has, from CODER_DEV_ADMIN_PASSWORD or the file that an
# earlier launch saved next to the data.
password_file="$VC_CONFIG_DIR/.verify-coder-admin-password"
new_database=0
if ((fresh)) || [[ ! -d "$VC_CONFIG_DIR/postgres/data" ]]; then
	new_database=1
	password="${CODER_DEV_ADMIN_PASSWORD:-Vc-$(openssl rand -hex 16)}"
elif [[ -n "${CODER_DEV_ADMIN_PASSWORD:-}" ]]; then
	password="$CODER_DEV_ADMIN_PASSWORD"
elif [[ -f "$password_file" ]]; then
	password="$(<"$password_file")"
else
	vc_fail "$VC_CONFIG_DIR has no saved admin password, so its admin likely has the published default, which develop.sh would expose on every interface: run launch.sh --fresh, or set CODER_DEV_ADMIN_PASSWORD to its real password"
fi
if [[ "$password" == "SomeSecurePassword!" ]]; then
	vc_fail "the published default password is not allowed: develop.sh listens on every interface"
fi

bin="$(vc_coder_bin)"
echo "building build/.bin/develop and ${bin#"$VC_ROOT"/} (log: $VC_RUN/build.log)"
if ! (cd "$VC_ROOT" && DEVELOP_IN_CODER="$develop_in_coder" MAKE_TIMED=1 make -j build/.bin/develop "${bin#"$VC_ROOT"/}") >"$VC_RUN/build.log" 2>&1; then
	tail -n 30 "$VC_RUN/build.log" | cut -c1-300 >&2
	vc_fail "build failed, see $VC_RUN/build.log"
fi

# The marker tells cleanup.sh that this harness created the folder.
# cleanup.sh --keep-data deletes it, so the folder then counts as the
# developer's.
if [[ ! -d "$VC_CONFIG_DIR" ]]; then
	mkdir -p "$VC_CONFIG_DIR"
	touch "$VC_CONFIG_DIR/.verify-coder-created"
fi

# develop picks the ports: --port-offset is its per-checkout offset
# (.claude/docs/DEV_ISOLATION.md), and explicit ports skip it.
args=(--port-offset --prometheus-port 0)
if [[ -n "${CODER_DEV_PORT:-}" ]]; then
	args+=(--port "$CODER_DEV_PORT")
fi
if [[ -n "${CODER_DEV_WEB_PORT:-}" ]]; then
	args+=(--web-port "$CODER_DEV_WEB_PORT")
fi
if ((fresh)); then
	args+=(--db-reset)
fi

started_at="$(date -u +%FT%TZ)"
pid=""
api_port=""
web_port=""
# write_env records the instance. printf %q keeps paths with spaces or shell
# characters intact when lib.sh sources the file. Journeys drive the
# production frontend that coder server serves from the embedded build
# (VC_WEB_URL, the API port). The Vite dev server (VC_VITE_URL) serves
# thousands of unbundled modules per page, and under host load one failed
# fetch broke a lazy route import with the app's error page.
write_env() {
	{
		printf 'VC_PID=%q\n' "$pid"
		printf 'VC_LOG=%q\n' "$log"
		printf 'VC_STARTED_AT=%q\n' "$started_at"
		printf 'VC_ADMIN_PASSWORD=%q\n' "$password"
		if [[ -n "$api_port" ]]; then
			printf 'VC_API_PORT=%q\n' "$api_port"
			printf 'VC_WEB_PORT=%q\n' "$web_port"
			printf 'VC_API_URL=%q\n' "http://127.0.0.1:$api_port"
			printf 'VC_WEB_URL=%q\n' "http://127.0.0.1:$api_port"
			printf 'VC_VITE_URL=%q\n' "http://127.0.0.1:$web_port"
			printf 'VC_BROWSER_SESSION=%q\n' "verify-coder-$api_port"
		fi
	} >"$VC_RUN/env"
}

# The password file changes only once develop has reset the database, so a
# launch that fails before that keeps the old file for the old data.
password_saved=0
save_password() {
	if ((new_database && !password_saved)) && grep -q 'creating first user' "$log"; then
		(
			umask 077
			printf '%s\n' "$password" >"$password_file"
		)
		password_saved=1
	fi
}

# run_develop <develop args...> starts develop with $log as its log and waits
# until it is ready. The pid goes on record at once, so cleanup.sh can stop a
# launch that fails before develop reports its ports.
run_develop() {
	local ports alive
	vc_start_develop "$log" "$password" "$develop_in_coder" "$@" -- "${server_flags[@]}"
	pid="$VC_DEVELOP_PID"
	api_port=""
	web_port=""
	write_env
	# Both waits check that develop runs before they read its log, so a
	# develop that writes a line and exits between the two still counts as
	# having written it.
	deadline=$((SECONDS + 120))
	while :; do
		alive=0
		if kill -0 "$pid" 2>/dev/null; then
			alive=1
		fi
		ports="$(sed -n 's/.*resolved listen ports .*api=\([0-9][0-9]*\) .*web=\([0-9][0-9]*\) .*/\1 \2/p' "$log" | head -n 1)"
		[[ -z "$ports" ]] || break
		if ((!alive || SECONDS >= deadline)); then
			vc_log_tail 20 "$log" "$password" >&2
			vc_fail "develop.sh reported no ports, see $log, then run cleanup.sh"
		fi
		sleep 1
	done
	read -r api_port web_port <<<"$ports"
	write_env
	echo "started develop.sh as pid $pid (api $api_port, web $web_port, log: $log)"
	deadline=$((SECONDS + ${VERIFY_CODER_TIMEOUT:-900}))
	while :; do
		alive=0
		if kill -0 "$pid" 2>/dev/null; then
			alive=1
		fi
		save_password
		if grep -q 'Coder is now running in development mode' "$log"; then
			break
		fi
		if ((!alive)); then
			vc_log_tail 40 "$log" "$password" >&2
			vc_fail "develop.sh exited before it was ready, see $log, then run cleanup.sh"
		fi
		if ((SECONDS >= deadline)); then
			vc_log_tail 20 "$log" "$password" >&2
			vc_fail "not ready after ${VERIFY_CODER_TIMEOUT:-900}s, instance left running for diagnosis, run cleanup.sh when done"
		fi
		sleep 2
	done
}

# develop.sh creates the first user, the owner, through the unauthenticated
# /api/v2/users/first route once the server is healthy. On every interface, a
# host on the network could claim the owner first. So develop runs its setup
# on 127.0.0.1, also for reused data, which can lack a user when an earlier
# launch failed. The docker template's workspace agents reach the server
# through host.docker.internal, so develop then restarts on every interface
# with the same ports and the database it set up.
log="$VC_RUN/develop-setup.log"
echo "setting up on 127.0.0.1"
run_develop --bind-address 127.0.0.1 "${args[@]}"
if ((new_database && !password_saved)); then
	vc_fail "launch.sh created a new database but develop.sh logged no first user, see $log, then run cleanup.sh"
fi
# Reused data can still have the default password when the one launch.sh was
# given is wrong, and develop.sh reuses a saved session without signing in.
# Both show here, before the server listens on other interfaces.
VC_API_URL="http://127.0.0.1:$api_port"
kill -0 "$pid" 2>/dev/null || vc_fail "develop.sh exited after setup, see $log, then run cleanup.sh"
if default_user="$(vc_default_password_accepted)"; then
	vc_fail "$default_user signs in with the published default password, so launch.sh keeps the instance on 127.0.0.1: run cleanup.sh, then launch.sh --fresh"
fi
vc_password_accepted "$password" || vc_fail "the admin password launch.sh used does not sign in, so the journeys would fail: run cleanup.sh, then fix CODER_DEV_ADMIN_PASSWORD or launch.sh --fresh"
grep -E 'template created in org|starter template setup failed|created member user' "$log" | cut -c1-200 || true

vc_stop_develop "$pid" >/dev/null || vc_fail "could not stop the setup run (pid $pid), run cleanup.sh"
for port in "$api_port" "$web_port"; do
	if vc_port_busy "$port"; then
		vc_fail "port $port is still in use after the setup run stopped, run cleanup.sh"
	fi
done
log="$VC_RUN/develop.log"
echo "restarting on every interface"
run_develop --prometheus-port 0 --port "$api_port" --web-port "$web_port"
exec "$(dirname "${BASH_SOURCE[0]}")/doctor.sh"
