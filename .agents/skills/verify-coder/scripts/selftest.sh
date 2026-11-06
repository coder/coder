#!/usr/bin/env bash
# Checks harness behavior that needs no running instance: the launch.sh
# argument and password refusals, the random password for a new database, the
# loopback bind of develop, that launch.sh log tails show no password, the
# password gates after develop is ready, that cleanup keeps a .coderv2 that
# Postgres still runs from, the launch lock, how
# vc_start_develop passes the environment, that a Docker error or a missing
# docker CLI fails the leftover inventory, the default password check for
# admin and member, that docker-forward.py carries its port, and that
# vc_browser_login keeps the password out of the process arguments.
# Usage: selftest.sh   (prints one line per check and a count line)
set -euo pipefail
here="$(dirname "${BASH_SOURCE[0]}")"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# Stubs: make stops a launch that got past its refusals before it builds,
# and agent-browser records its arguments and stdin.
mkdir -p "$tmp/bin"
printf '#!/usr/bin/env bash\necho "selftest: launch reached the build" >&2\nexit 99\n' >"$tmp/bin/make"
cat >"$tmp/bin/agent-browser" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$VC_SELFTEST_DIR/argv"
if [[ " $* " == *" batch "* ]]; then
	cat >>"$VC_SELFTEST_DIR/stdin"
fi
STUB
# env records its arguments and runs the real env. setsid records the
# environment and arguments develop.sh would get. VC_SELFTEST_DEVELOP makes it
# print develop log lines first and, for *-wait, keep running.
cat >"$tmp/bin/env" <<'STUB'
#!/bin/bash
printf '%s\n' "$*" >>"$VC_SELFTEST_DIR/env-argv"
exec /usr/bin/env "$@"
STUB
cat >"$tmp/bin/setsid" <<'STUB'
#!/bin/bash
/usr/bin/env >"$VC_SELFTEST_DIR/develop-env"
shift
printf '%s\n' "$*" >>"$VC_SELFTEST_DIR/develop-argv"
out='selftest develop output'
ports='resolved listen ports  api=1 web=2 proxy=0 metrics=0'
first="creating first user  email=admin@coder.com  password=${CODER_DEV_ADMIN_PASSWORD:-}"
ready='Coder is now running in development mode.'
case "${VC_SELFTEST_DEVELOP:-}" in
password) printf '%s\n' "$out" "$first" ;;
ports) printf '%s\n' "$out" "$ports" "$first" ;;
ports-wait) printf '%s\n' "$out" "$ports" "$first" && exec sleep 30 ;;
ready) printf '%s\n' "$out" "$ports" "$first" "$ready" ;;
ready-wait) printf '%s\n' "$out" "$ports" "$first" "$ready" && exec sleep 30 ;;
esac
STUB
# curl answers VC_SELFTEST_CURL_CODE (default 201) to a sign-in as
# VC_SELFTEST_CURL_OK and 401 otherwise.
cat >"$tmp/bin/curl" <<'STUB'
#!/bin/bash
if [[ -n "${VC_SELFTEST_CURL_OK:-}" && "$*" == *"\"$VC_SELFTEST_CURL_OK\""* ]]; then echo "${VC_SELFTEST_CURL_CODE:-201}"; else echo 401; fi
STUB
# bin-built goes first on PATH for checks that need launch.sh past the build
# or a docker that lists nothing.
mkdir -p "$tmp/bin-built"
printf '#!/usr/bin/env bash\nexit 0\n' >"$tmp/bin-built/make"
printf '#!/usr/bin/env bash\nexit 0\n' >"$tmp/bin-built/docker"
chmod +x "$tmp/bin/make" "$tmp/bin/agent-browser" "$tmp/bin/env" "$tmp/bin/setsid" "$tmp/bin/curl" "$tmp/bin-built/make" "$tmp/bin-built/docker"
export VERIFY_CODER_STATE="$tmp/state" VC_SELFTEST_DIR="$tmp"
# shellcheck source=.agents/skills/verify-coder/scripts/lib.sh
source "$here/lib.sh"

# Checks that take launch.sh or cleanup.sh past their refusals run a copy in a
# scratch repo, so its .coderv2 and run state are scratch folders.
repo="$tmp/repo"
git init -q "$repo"
mkdir -p "$repo/.agents/skills/verify-coder"
cp -R "$here" "$repo/.agents/skills/verify-coder/scripts"
scratch_scripts="$repo/.agents/skills/verify-coder/scripts"
scratch_config="$repo/.coderv2"
scratch_run="$repo/build/verify-coder/run"
scratch_reset() {
	rm -rf "$scratch_config" "$repo/build" "$tmp/develop-argv"
}
# scratch_run_script <launch.sh|cleanup.sh> <stub develop mode> <args...> sets
# out and rc.
scratch_run_script() {
	local script="$1" mode="$2"
	shift 2
	rc=0
	out="$(env -u VERIFY_CODER_STATE CODER_BUILD_AGPL= VC_SELFTEST_DEVELOP="$mode" PATH="$tmp/bin-built:$tmp/bin:$PATH" \
		"$scratch_scripts/$script" "$@" 2>&1)" || rc=$?
}
# stop_scratch_develop stops a stub develop that launch.sh left running.
stop_scratch_develop() {
	local pid
	pid="$(sed -n 's/^VC_PID=//p' "$scratch_run/env")"
	kill "$pid" 2>/dev/null || true
}

refuses() {
	local want="$1" out rc=0
	shift
	out="$(env "$@" PATH="$tmp/bin:$PATH" "$here/launch.sh" "${launch_args[@]}" 2>&1)" || rc=$?
	[[ "$rc" == 1 && "$out" == *"$want"* ]] || {
		echo "launch.sh exited $rc: ${out##*$'\n'}"
		return 1
	}
	echo "launch.sh refused: $want"
}

launch_args=()
vc_check agpl-env refuses "CODER_BUILD_AGPL is not supported" CODER_BUILD_AGPL=1
launch_args=(--agpl)
vc_check develop-flag refuses "--agpl is not supported" CODER_BUILD_AGPL=
launch_args=(--fresh --db-reset)
vc_check db-reset-flag refuses "--db-reset is not supported" CODER_BUILD_AGPL=
launch_args=(--port 1)
vc_check port-flag refuses "--port is not supported" CODER_BUILD_AGPL=
launch_args=(-- --verbose --postgres-url=postgres://elsewhere)
vc_check server-flag refuses "server flag --postgres-url=postgres://elsewhere is not supported" CODER_BUILD_AGPL=
launch_args=(--fresh)
vc_check default-password refuses "the published default password is not allowed" CODER_BUILD_AGPL= CODER_DEV_ADMIN_PASSWORD='SomeSecurePassword!'

# A launch that passes every launch.sh check reaches the stub make.
reaches_build() {
	local out rc=0
	out="$(env -u CODER_DEV_ADMIN_PASSWORD CODER_BUILD_AGPL= PATH="$tmp/bin:$PATH" "$here/launch.sh" "$@" 2>&1)" || rc=$?
	[[ "$out" == *"selftest: launch reached the build"* ]] || {
		echo "launch.sh $* exited $rc before the build: ${out##*$'\n'}"
		return 1
	}
	echo "launch.sh $* gets past its checks"
}
# Without CODER_DEV_ADMIN_PASSWORD, a new database gets a random password,
# not the refused default.
vc_check random-password reaches_build --fresh
vc_check allowed-server-flags reaches_build --fresh -- --verbose --experiments=example

start_develop_isolates() {
	local marker="selftest-$RANDOM-$RANDOM"
	# vc_check runs this in a subshell, so the exports stay here.
	# A caller's own CODER_DEV_ADMIN_PASSWORD must lose to the one passed in.
	export CODER_PG_CONNECTION_URL=postgres://selftest-elsewhere CODER_DEV_DB_RESET=1 \
		CODER_DEV_ADMIN_PASSWORD=selftest-caller-value BASH_ENV="$tmp/bash-env" PATH="$tmp/bin:$PATH"
	vc_start_develop "$tmp/develop.log" "$marker" 1 --port 1
	wait "$VC_DEVELOP_PID"
	if grep -qF "$marker" "$tmp/env-argv"; then
		echo "the password appears in the arguments of env"
		return 1
	fi
	grep -qxF "CODER_DEV_ADMIN_PASSWORD=$marker" "$tmp/develop-env" || {
		echo "develop.sh would not get the password"
		return 1
	}
	grep -qxF "DEVELOP_IN_CODER=1" "$tmp/develop-env" || {
		echo "develop.sh would not get DEVELOP_IN_CODER=1"
		return 1
	}
	if grep -qE '^(CODER_PG_CONNECTION_URL|CODER_DEV_DB_RESET|BASH_ENV)=' "$tmp/develop-env"; then
		echo "a caller CODER_* variable or BASH_ENV reaches develop.sh"
		return 1
	fi
	echo "develop.sh gets the password through its environment and no caller CODER_* variable"
}
vc_check start-develop start_develop_isolates

# develop.sh creates the owner through an unauthenticated route, and Vite
# serves checkout files, so develop must bind 127.0.0.1 for every kind of
# data. The stub develop exits at once, so launch.sh stops after that start.
setup_binds_loopback() {
	local kind
	unset CODER_DEV_ADMIN_PASSWORD
	for kind in fresh new-data reused-data; do
		scratch_reset
		local launch_flags=()
		case "$kind" in
		fresh) launch_flags=(--fresh) ;;
		reused-data)
			mkdir -p "$scratch_config/postgres/data"
			echo selftest-saved-password >"$scratch_config/.verify-coder-admin-password"
			echo selftest-other-user-token >"$scratch_config/session"
			;;
		esac
		scratch_run_script launch.sh "" "${launch_flags[@]}"
		[[ "$rc" == 1 && "$out" == *"develop.sh reported no ports"* ]] || {
			echo "$kind: launch.sh exited $rc: ${out##*$'\n'}"
			return 1
		}
		if [[ -e "$scratch_config/session" ]]; then
			echo "$kind: develop started with a saved CLI session, so it would not sign in as the admin"
			return 1
		fi
		[[ "$(wc -l <"$tmp/develop-argv")" == 1 && " $(cat "$tmp/develop-argv") " == *" --bind-address 127.0.0.1 "* ]] || {
			echo "$kind: the first develop start was not bound to 127.0.0.1: $(head -c 300 "$tmp/develop-argv")"
			return 1
		}
	done
	echo "develop binds 127.0.0.1 for --fresh, new data and reused data, without a saved session"
}
vc_check setup-loopback setup_binds_loopback

# develop logs the password when it creates the first user. Each develop log
# tail that launch.sh prints on a failure must drop that line and keep others.
launch_tails_drop_password() {
	local marker="selftest-$RANDOM-$RANDOM" mode want
	export CODER_DEV_ADMIN_PASSWORD="$marker" VERIFY_CODER_TIMEOUT=1
	for mode in password ports ports-wait; do
		case "$mode" in
		password) want="develop.sh reported no ports" ;;
		ports) want="develop.sh exited before it was ready" ;;
		ports-wait) want="not ready after 1s" ;;
		esac
		scratch_reset
		scratch_run_script launch.sh "$mode" --fresh
		if [[ "$mode" == *-wait ]]; then
			stop_scratch_develop
		fi
		[[ "$rc" == 1 && "$out" == *"$want"* ]] || {
			echo "$mode: launch.sh exited $rc: ${out##*$'\n'}"
			return 1
		}
		if [[ "$out" == *"$marker"* ]]; then
			echo "$mode: the password appears in the output of launch.sh"
			return 1
		fi
		[[ "$out" == *"selftest develop output"* ]] || {
			echo "$mode: launch.sh printed no develop log tail"
			return 1
		}
	done
	echo "the develop log tails of launch.sh show no password"
}
vc_check launch-tails launch_tails_drop_password

# develop logs a password with special characters quoted and escaped, so the
# tail drops any password field as well as any line with the password.
log_tail_drops_escaped_password() {
	printf 'before\ncreating first user  password="p\\"q"\nsecret p"q in another field\nafter\n' >"$tmp/tail.log"
	[[ "$(vc_log_tail 20 "$tmp/tail.log" 'p"q')" == $'before\nafter' ]] || {
		echo "the log tail kept a line with the password or lost other lines"
		return 1
	}
	echo "the log tail drops password lines and password fields only"
}
vc_check log-tail log_tail_drops_escaped_password

# launch.sh must fail and start develop only once when the instance accepts the
# published default password for the admin or the member, and must report a
# develop that died after its banner.
setup_gates() {
	local email
	export CODER_DEV_ADMIN_PASSWORD="selftest-$RANDOM"
	for email in admin@coder.com member@coder.com; do
		scratch_reset
		VC_SELFTEST_CURL_OK="$email" scratch_run_script launch.sh ready-wait --fresh
		stop_scratch_develop
		[[ "$rc" == 1 && "$out" == *"$email signs in with the published default password"* ]] || {
			echo "$email: launch.sh exited $rc: ${out##*$'\n'}"
			return 1
		}
		if [[ "$(wc -l <"$tmp/develop-argv")" != 1 ]]; then
			echo "$email: launch.sh started develop more than once"
			return 1
		fi
	done
	scratch_reset
	scratch_run_script launch.sh ready --fresh
	[[ "$rc" == 1 && "$out" == *"develop.sh exited after it was ready"* ]] || {
		echo "dead develop: launch.sh exited $rc: ${out##*$'\n'}"
		return 1
	}
	echo "launch.sh stops on a default password for admin or member and on a dead develop"
}
vc_check setup-gates setup_gates

# cleanup.sh must keep .coderv2 and fail while a Postgres runs from it, also
# when its binary was deleted or its pid cannot be identified (a zombie), and
# remove .coderv2 otherwise. A copy of bash named postgres stands in for
# Postgres, which runs from its data folder.
cleanup_keeps_running_postgres() {
	local case pid dead keep data="$scratch_config/postgres/data" loop='while :; do sleep 1; done'
	local others=()
	mkdir -p "$tmp/fake"
	for case in running deleted-binary zombie dead other-folder other-program no-pid-file; do
		scratch_reset
		mkdir -p "$scratch_run" "$repo/build/verify-coder/evidence" "$data"
		touch "$scratch_config/.verify-coder-created"
		true &
		dead=$!
		wait "$dead"
		printf 'VC_PID=%q\nVC_LOG=%q\nVC_STARTED_AT=%q\nVC_ADMIN_PASSWORD=x\n' "$dead" "$scratch_run/develop.log" "$(date -u +%FT%TZ)" >"$scratch_run/env"
		rm -rf "$tmp/fake/$case" "$tmp/zombie.pid"
		mkdir -p "$tmp/fake/$case"
		cp "$(command -v bash)" "$tmp/fake/$case/postgres"
		pid="$dead"
		keep=0
		case "$case" in
		running | deleted-binary)
			(cd "$data" && exec "$tmp/fake/$case/postgres" -c "$loop") &
			pid=$!
			keep=1
			;;
		zombie)
			# The child exits after its parent has become sleep, which never
			# reaps it.
			(cd "$data" && exec bash -c 'sleep 1 & echo $! >"$1"; exec sleep 20' _ "$tmp/zombie.pid") &
			others+=("$!")
			for _ in $(seq 100); do
				pid="$(cat "$tmp/zombie.pid" 2>/dev/null)" || true
				if [[ -n "$pid" ]] && grep -q '^State:.Z' "/proc/$pid/status" 2>/dev/null; then
					break
				fi
				sleep 0.1
			done
			grep -q '^State:.Z' "/proc/$pid/status" 2>/dev/null || {
				echo "zombie: the stand-in never became a zombie"
				return 1
			}
			keep=1
			;;
		other-folder)
			(cd "$tmp" && exec "$tmp/fake/$case/postgres" -c "$loop") &
			pid=$!
			;;
		other-program)
			(cd "$data" && exec sleep 60) &
			pid=$!
			;;
		esac
		if [[ "$case" == deleted-binary ]]; then
			# Delete the binary only once the stand-in runs it.
			for _ in $(seq 100); do
				[[ "$(readlink "/proc/$pid/exe" 2>/dev/null)" != "$tmp/fake/$case/postgres" ]] || break
				sleep 0.1
			done
			[[ "$(readlink "/proc/$pid/exe" 2>/dev/null)" == "$tmp/fake/$case/postgres" ]] || {
				echo "deleted-binary: the stand-in never ran its binary"
				return 1
			}
			rm "$tmp/fake/$case/postgres"
		fi
		if [[ "$case" != no-pid-file ]]; then
			echo "$pid" >"$data/postmaster.pid"
		fi
		scratch_run_script cleanup.sh ""
		kill "$pid" "${others[@]}" 2>/dev/null || true
		if ((keep)); then
			[[ "$rc" == 1 && -d "$scratch_config" && "$out" == *"Postgres pid $pid still runs from .coderv2"* ]] || {
				echo "$case: cleanup.sh exited $rc and did not keep .coderv2 for pid $pid: ${out##*$'\n'}"
				return 1
			}
		else
			[[ "$rc" == 0 && ! -d "$scratch_config" ]] || {
				echo "$case: cleanup.sh exited $rc or kept .coderv2: ${out##*$'\n'}"
				return 1
			}
		fi
	done
	echo "cleanup.sh keeps .coderv2 only while a Postgres runs from it"
}
vc_check cleanup-postgres cleanup_keeps_running_postgres

# A second launch in the same checkout must stop while the first holds the lock.
launch_lock_held() {
	local out rc=0
	mkdir -p "$VERIFY_CODER_STATE"
	exec 8>"$VERIFY_CODER_STATE/launch.lock"
	flock 8
	out="$(env CODER_BUILD_AGPL= PATH="$tmp/bin:$PATH" "$here/launch.sh" --fresh 2>&1)" || rc=$?
	[[ "$rc" == 1 && "$out" == *"another launch.sh runs in this checkout"* ]] || {
		echo "launch.sh exited $rc with the lock held: ${out##*$'\n'}"
		return 1
	}
	echo "launch.sh refused while another launch held the lock"
}
vc_check launch-lock launch_lock_held

# With the server down, cleanup relies on this inventory, so a Docker error
# must fail it instead of reading as no leftovers.
docker_error_fails() {
	printf '#!/bin/bash\nexit 1\n' >"$tmp/bin/docker"
	chmod +x "$tmp/bin/docker"
	if PATH="$tmp/bin:$PATH" vc_docker_since 2026-01-01T00:00:00Z >/dev/null; then
		echo "vc_docker_since succeeded although docker failed"
		return 1
	fi
	echo "vc_docker_since fails when docker cannot list"
}
vc_check docker-error docker_error_fails

# Without a docker CLI the inventory counts as empty only when no Docker
# daemon is reachable either.
docker_missing() {
	mkdir -p "$tmp/nodocker"
	ln -sf "$(command -v date)" "$tmp/nodocker/date"
	if DOCKER_HOST=unix:///selftest-docker.sock PATH="$tmp/nodocker" vc_docker_since 2026-01-01T00:00:00Z >/dev/null; then
		echo "vc_docker_since succeeded without a docker CLI although DOCKER_HOST is set"
		return 1
	fi
	echo "vc_docker_since fails without a docker CLI while a daemon is configured"
}
vc_check docker-missing docker_missing

# Reused data can keep the published default on either account.
default_password_checks_users() {
	local email user
	for email in admin@coder.com member@coder.com; do
		user="$(VC_API_URL=http://127.0.0.1:1 VC_SELFTEST_CURL_OK="$email" PATH="$tmp/bin:$PATH" vc_default_password_accepted)" || {
			echo "$email signs in with the default password unnoticed"
			return 1
		}
		[[ "$user" == "$email" ]] || {
			echo "vc_default_password_accepted named $user, not $email"
			return 1
		}
	done
	if VC_API_URL=http://127.0.0.1:1 PATH="$tmp/bin:$PATH" vc_default_password_accepted >/dev/null; then
		echo "every account refuses the default password, but the check failed"
		return 1
	fi
	VC_API_URL=http://127.0.0.1:1 VC_SELFTEST_CURL_OK=admin@coder.com VC_SELFTEST_CURL_CODE=500 PATH="$tmp/bin:$PATH" \
		vc_default_password_accepted >/dev/null || {
		echo "an answer other than 401 counted as refused"
		return 1
	}
	echo "the default password is caught for admin and member, and only 401 counts as refused"
}
vc_check default-users default_password_checks_users

# docker-forward.py carries the one port it was given from its listen address
# to 127.0.0.1, and cleanup recognizes it as a helper to stop. 127.0.0.2
# stands in for the Docker bridge gateway.
docker_forward_carries_port() {
	local port="" out="" www="$tmp/www" fw rc=0
	mkdir -p "$www"
	echo selftest-forward >"$www/x"
	# Port 0 lets the kernel pick a free port, which the server then prints.
	(cd "$www" && exec python3 -u -m http.server 0 --bind 127.0.0.1) >"$tmp/www.log" 2>&1 &
	local server=$!
	for _ in $(seq 50); do
		port="$(sed -n 's/.* port \([0-9][0-9]*\).*/\1/p' "$tmp/www.log" | head -n 1)"
		[[ -z "$port" ]] || break
		sleep 0.2
	done
	[[ -n "$port" ]] || {
		kill "$server" 2>/dev/null || true
		echo "the stand-in API server did not start"
		return 1
	}
	python3 "$VC_SKILL_SCRIPTS/docker-forward.py" 127.0.0.2 "$port" >"$tmp/forward.log" 2>&1 &
	fw=$!
	for _ in $(seq 50); do
		out="$(curl -sS --max-time 2 "http://127.0.0.2:$port/x" 2>/dev/null)" && break
		sleep 0.2
	done
	vc_is_docker_forward "$fw" || rc=1
	kill "$server" "$fw" 2>/dev/null || true
	[[ "$out" == selftest-forward ]] || {
		echo "docker-forward.py did not carry port $port: ${out:-no answer}"
		return 1
	}
	((rc == 0)) || {
		echo "cleanup would not recognize docker-forward.py pid $fw"
		return 1
	}
	echo "docker-forward.py carries its port to 127.0.0.1, and cleanup recognizes it"
}
vc_check docker-forward docker_forward_carries_port

login_keeps_password_off_argv() {
	local marker="selftest-$RANDOM-$RANDOM"
	VC_WEB_URL=http://127.0.0.1:1 VC_BROWSER_SESSION=verify-coder-selftest PATH="$tmp/bin:$PATH" \
		vc_browser_login "$VC_ADMIN_EMAIL" "$marker"
	if grep -qF "$marker" "$tmp/argv"; then
		echo "the password appears in agent-browser arguments"
		return 1
	fi
	grep -qF "$marker" "$tmp/stdin" || {
		echo "the password never reached agent-browser stdin"
		return 1
	}
	echo "the password reached agent-browser on stdin only"
}
vc_check login-argv login_keeps_password_off_argv

echo "selftest: $VC_PASS/$VC_TOTAL checks passed"
((VC_PASS == VC_TOTAL))
