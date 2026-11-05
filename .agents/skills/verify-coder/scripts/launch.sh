#!/usr/bin/env bash
# Starts a Coder dev instance owned by this checkout and waits until it is
# usable. Usage: launch.sh [--fresh] [develop.sh flags] [-- coder server flags]
#   --fresh  passes --db-reset to develop.sh (destroys this checkout's .coderv2 database).
# Every other argument reaches develop.sh unchanged, so "launch.sh -- --trace"
# passes --trace to coder server.
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
umask 022

# Build first so the ports are picked seconds before develop binds them, not
# after a build that can take half an hour. DEVELOP_IN_CODER must match what
# develop passes to make, or the prebuilt binary gets the wrong ldflags.
develop_in_coder=0
if [[ "${DEVELOP_IN_CODER:-}" == 1 || -n "${CODER_AGENT_URL:-}" ]]; then
	develop_in_coder=1
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

# The marker survives cleanup.sh --keep-data, so a later cleanup still knows
# this harness created the folder.
if [[ ! -d "$VC_CONFIG_DIR" ]]; then
	mkdir -p "$VC_CONFIG_DIR"
	touch "$VC_CONFIG_DIR/.verify-coder-created"
fi
args=(--port "$api_port" --web-port "$web_port" --prometheus-port 0)
if ((fresh)); then
	args+=(--db-reset)
fi

# Provider keys are dropped so the instance never spends real tokens by
# itself (task naming calls Anthropic when a key is present).
log="$VC_RUN/develop.log"
env -u ANTHROPIC_API_KEY -u ANTHROPIC_AUTH_TOKEN -u ANTHROPIC_BASE_URL -u OPENAI_API_KEY -u OPENAI_BASE_URL \
	setsid "$VC_ROOT/scripts/develop.sh" "${args[@]}" "$@" >"$log" 2>&1 </dev/null &
pid=$!

# Journeys drive the production frontend that coder server serves from the
# embedded build (VC_WEB_URL, the API port). The Vite dev server
# (VC_VITE_URL) serves thousands of unbundled modules per page, and under host
# load one failed fetch broke a lazy route import with the app's error page.
cat >"$VC_RUN/env" <<EOF
VC_PID=$pid
VC_API_PORT=$api_port
VC_WEB_PORT=$web_port
VC_API_URL=http://127.0.0.1:$api_port
VC_WEB_URL=http://127.0.0.1:$api_port
VC_VITE_URL=http://127.0.0.1:$web_port
VC_LOG=$log
VC_BROWSER_SESSION=verify-coder-$api_port
VC_STARTED_AT=$(date -u +%FT%TZ)
EOF
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
