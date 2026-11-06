#!/usr/bin/env bash
# Checks harness behavior that needs no running instance: the launch.sh
# argument and password refusals, the random password for a new database, the
# loopback bind of the setup run, the launch lock, how vc_start_develop passes
# the environment, that a Docker error fails the leftover inventory, that log
# tails drop the password, that the member's default password is caught, and
# that vc_browser_login keeps the password out of the process arguments.
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
# environment develop.sh would get and exits.
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
STUB
# bin-built goes first on PATH for checks that need launch.sh past the build.
mkdir -p "$tmp/bin-built"
printf '#!/usr/bin/env bash\nexit 0\n' >"$tmp/bin-built/make"
chmod +x "$tmp/bin/make" "$tmp/bin/agent-browser" "$tmp/bin/env" "$tmp/bin/setsid" "$tmp/bin-built/make"
export VERIFY_CODER_STATE="$tmp/state" VC_SELFTEST_DIR="$tmp"
# shellcheck source=.agents/skills/verify-coder/scripts/lib.sh
source "$here/lib.sh"

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

# develop.sh creates the owner through an unauthenticated route, so the first
# develop start, the only one before setup is done, must bind 127.0.0.1 for
# every kind of data. launch.sh runs from a copy in a scratch repo, so its
# .coderv2 is a scratch folder. The stub develop exits at once, so launch.sh
# stops after that start.
setup_binds_loopback() {
	local repo="$tmp/repo" kind out rc
	git init -q "$repo"
	mkdir -p "$repo/.agents/skills/verify-coder"
	cp -R "$here" "$repo/.agents/skills/verify-coder/scripts"
	for kind in fresh new-data reused-data; do
		rm -rf "$repo/.coderv2" "$repo/build" "$tmp/develop-argv"
		local launch_flags=()
		case "$kind" in
		fresh) launch_flags=(--fresh) ;;
		reused-data)
			mkdir -p "$repo/.coderv2/postgres/data"
			echo selftest-saved-password >"$repo/.coderv2/.verify-coder-admin-password"
			;;
		esac
		rc=0
		out="$(env -u CODER_DEV_ADMIN_PASSWORD -u VERIFY_CODER_STATE CODER_BUILD_AGPL= PATH="$tmp/bin-built:$tmp/bin:$PATH" \
			"$repo/.agents/skills/verify-coder/scripts/launch.sh" "${launch_flags[@]}" 2>&1)" || rc=$?
		[[ "$rc" == 1 && "$out" == *"develop.sh reported no ports"* ]] || {
			echo "$kind: launch.sh exited $rc: ${out##*$'\n'}"
			return 1
		}
		[[ "$(wc -l <"$tmp/develop-argv")" == 1 && " $(cat "$tmp/develop-argv") " == *" --bind-address 127.0.0.1 "* ]] || {
			echo "$kind: the first develop start was not bound to 127.0.0.1: $(head -c 300 "$tmp/develop-argv")"
			return 1
		}
	done
	echo "the setup run binds 127.0.0.1 for --fresh, new data and reused data"
}
vc_check setup-loopback setup_binds_loopback

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

# develop logs the password when it creates the first user, so the log tails
# that launch.sh prints on a failure must drop that line and keep the rest.
log_tail_drops_password() {
	local marker="selftest-$RANDOM-$RANDOM" out
	printf 'before\ncreating first user  email=admin@coder.com  password=%s\nafter\n' "$marker" >"$tmp/tail.log"
	out="$(vc_log_tail 20 "$tmp/tail.log" "$marker")"
	if [[ "$out" == *"$marker"* ]]; then
		echo "the password appears in the log tail"
		return 1
	fi
	[[ "$out" == $'before\nafter' ]] || {
		echo "the log tail lost other lines: $out"
		return 1
	}
	echo "the log tail drops the password line only"
}
vc_check log-tail log_tail_drops_password

# Reused data can keep the published default on the member account alone.
# The stub curl answers 201 only for member@coder.com.
default_password_checks_member() {
	local user
	mkdir -p "$tmp/bin-curl"
	cat >"$tmp/bin-curl/curl" <<'STUB'
#!/bin/bash
if [[ "$*" == *'"member@coder.com"'* ]]; then echo 201; else echo 401; fi
STUB
	chmod +x "$tmp/bin-curl/curl"
	user="$(VC_API_URL=http://127.0.0.1:1 PATH="$tmp/bin-curl:$PATH" vc_default_password_accepted)" || {
		echo "a member that signs in with the default password went unnoticed"
		return 1
	}
	[[ "$user" == member@coder.com ]] || {
		echo "vc_default_password_accepted named $user, not member@coder.com"
		return 1
	}
	echo "the member's default password is caught"
}
vc_check default-member default_password_checks_member

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
