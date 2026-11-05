#!/usr/bin/env bash
# Checks harness behavior that needs no running instance: the launch.sh
# refusals, the random password for a new database, and that vc_browser_login
# keeps the password out of the process arguments. Usage: selftest.sh   (prints one line per check and a count line)
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
chmod +x "$tmp/bin/make" "$tmp/bin/agent-browser"
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
vc_check agpl-flag refuses "--agpl is not supported" CODER_BUILD_AGPL=
launch_args=(--fresh --password secret)
vc_check password-flag refuses "--password is not supported" CODER_BUILD_AGPL=
launch_args=(--password=secret)
vc_check password-eq-flag refuses "--password is not supported" CODER_BUILD_AGPL=
launch_args=(--fresh)
vc_check default-password refuses "must not use the published default password" CODER_BUILD_AGPL= CODER_DEV_ADMIN_PASSWORD='SomeSecurePassword!'

# Without CODER_DEV_ADMIN_PASSWORD, a new database passes the password step
# and reaches the stub make.
random_password() {
	local out rc=0
	out="$(env -u CODER_DEV_ADMIN_PASSWORD CODER_BUILD_AGPL= PATH="$tmp/bin:$PATH" "$here/launch.sh" --fresh 2>&1)" || rc=$?
	[[ "$out" == *"selftest: launch reached the build"* ]] || {
		echo "launch.sh exited $rc before the build: ${out##*$'\n'}"
		return 1
	}
	echo "a new database without CODER_DEV_ADMIN_PASSWORD gets past the password step"
}
vc_check random-password random_password

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
