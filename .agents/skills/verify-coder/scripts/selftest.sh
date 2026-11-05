#!/usr/bin/env bash
# Checks harness behavior that needs no running instance: the launch.sh
# argument and password refusals, the random password for a new database, how
# vc_start_develop passes the environment, and that vc_browser_login keeps the
# password out of the process arguments.
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
STUB
chmod +x "$tmp/bin/make" "$tmp/bin/agent-browser" "$tmp/bin/env" "$tmp/bin/setsid"
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
vc_check default-password refuses "must not use the published default password" CODER_BUILD_AGPL= CODER_DEV_ADMIN_PASSWORD='SomeSecurePassword!'

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
	export CODER_PG_CONNECTION_URL=postgres://selftest-elsewhere CODER_DEV_DB_RESET=1 PATH="$tmp/bin:$PATH"
	vc_start_develop "$tmp/develop.log" "$marker" 0 --port 1
	wait "$VC_DEVELOP_PID"
	if grep -qF "$marker" "$tmp/env-argv"; then
		echo "the password appears in the arguments of env"
		return 1
	fi
	grep -qxF "CODER_DEV_ADMIN_PASSWORD=$marker" "$tmp/develop-env" || {
		echo "develop.sh would not get the password"
		return 1
	}
	if grep -qE '^(CODER_PG_CONNECTION_URL|CODER_DEV_DB_RESET)=' "$tmp/develop-env"; then
		echo "a caller CODER_* variable reaches develop.sh"
		return 1
	fi
	echo "develop.sh gets the password through its environment and no caller CODER_* variable"
}
vc_check start-develop start_develop_isolates

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
