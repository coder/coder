#!/usr/bin/env bash
# Starts the self-contained chat rendering benchmark stack:
#   fakellm      127.0.0.1:18081  OpenAI-compatible streaming stub
#   coder-embed  127.0.0.1:18080  coderd serving the embedded production site,
#                                 built-in postgres under $ROOT/config
#
# Usage: scripts/chatbench/up.sh [start|stop|status|build]
#   build  rebuilds the site bundle, the embed binary and fakellm, then restarts both
#
# CHATBENCH_BINARY selects the embed binary to build and serve, so before,
# after and CODER_REACT_PROFILING=true builds can coexist:
#   CHATBENCH_BINARY=build/coder-embed-profiling CODER_REACT_PROFILING=true \
#     scripts/chatbench/up.sh build
set -euo pipefail

REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# State (postgres data, seeded chat ids, results) lives under the cache dir,
# not /tmp, so it survives a workspace stop.
ROOT=${CHATBENCH_ROOT:-${XDG_CACHE_HOME:-$HOME/.cache}/chatbench}
# Which embed binary to serve; lets before/after builds be swapped.
BINARY=${CHATBENCH_BINARY:-$REPO/build/coder-embed}
API_PORT=18080
LLM_PORT=18081
export CODER_URL=http://127.0.0.1:$API_PORT
mkdir -p "$ROOT/config" "$ROOT/logs"

pidfile() { echo "$ROOT/$1.pid"; }

start_one() {
	local name=$1
	shift
	if [ -f "$(pidfile "$name")" ] && kill -0 "$(cat "$(pidfile "$name")")" 2>/dev/null; then
		echo "$name already running (pid $(cat "$(pidfile "$name")"))"
		return
	fi
	nohup "$@" >"$ROOT/logs/$name.log" 2>&1 &
	echo $! >"$(pidfile "$name")"
	echo "$name started (pid $!)"
}

stop_one() {
	local name=$1
	if [ -f "$(pidfile "$name")" ]; then
		local pid
		pid=$(cat "$(pidfile "$name")")
		pkill -TERM -P "$pid" 2>/dev/null || true
		kill -TERM "$pid" 2>/dev/null || true
		for _ in $(seq 300); do
			kill -0 "$pid" 2>/dev/null || break
			sleep 0.2
		done
		if kill -0 "$pid" 2>/dev/null; then
			echo "$name (pid $pid) did not exit within 60s" >&2
			exit 1
		fi
		rm -f "$(pidfile "$name")"
		echo "$name stopped"
	fi
}

wait_http() {
	local url=$1 tries=${2:-120}
	for _ in $(seq "$tries"); do
		if curl -fsS -o /dev/null "$url" 2>/dev/null; then return 0; fi
		sleep 0.5
	done
	echo "timeout waiting for $url" >&2
	return 1
}

start() {
	if ss -ltn | grep -q ":$API_PORT "; then
		echo "port $API_PORT is already in use; run '$0 stop' first" >&2
		ss -ltnp | grep ":$API_PORT " >&2 || true
		exit 1
	fi
	start_one fakellm "$REPO/build/fakellm" --addr "127.0.0.1:$LLM_PORT" --rate "${CHATBENCH_RATE:-40}"
	# CODER_SESSION_TOKEN and CODER_URL from the outer workspace must not
	# leak into the benchmark server.
	start_one coderd env -u CODER_SESSION_TOKEN -u CODER_URL "$BINARY" \
		--global-config "$ROOT/config" server \
		--http-address "127.0.0.1:$API_PORT" \
		--access-url "$CODER_URL" \
		--telemetry=false \
		--experiments '*'
	wait_http "http://127.0.0.1:$LLM_PORT/healthz" 20
	wait_http "http://127.0.0.1:$API_PORT/healthz" 240
	echo "stack ready at $CODER_URL (serving $BINARY)"
}

case "${1:-start}" in
start) start ;;
stop)
	stop_one coderd
	stop_one fakellm
	;;
status)
	for n in fakellm coderd; do
		if [ -f "$(pidfile "$n")" ] && kill -0 "$(cat "$(pidfile "$n")")" 2>/dev/null; then
			echo "$n running (pid $(cat "$(pidfile "$n")"))"
		else
			echo "$n not running"
		fi
	done
	;;
build)
	(cd "$REPO/site" && NODE_ENV=production pnpm vite build >"$ROOT/logs/site-build.log" 2>&1) || {
		tail -20 "$ROOT/logs/site-build.log"
		exit 1
	}
	(cd "$REPO" && go build -tags embed -o "$BINARY" ./cmd/coder)
	(cd "$REPO" && go build -o build/fakellm ./scripts/chatbench/fakellm)
	stop_one coderd
	stop_one fakellm
	start
	;;
*)
	echo "usage: $0 [start|stop|status|build]" >&2
	exit 2
	;;
esac
