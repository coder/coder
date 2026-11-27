#!/usr/bin/env bash
# Tests for dependabot-alert-match.sh. No CI job runs this file.
#
# Run: bash .github/scripts/dependabot-alert-match.test.sh

set -euo pipefail

SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/dependabot-alert-match.sh"
WORK=$(mktemp -d)
trap 'rm -rf "${WORK}"' EXIT

# A gh stub that serves fixtures from ${WORK}. The script passes --jq, so the
# fixtures are the already-filtered output:
#   commit-message          the first commit's message
#   files                   the PR's changed files, one per line
#   alerts.<package>        number<TAB>manifest<TAB>first patched version
mkdir -p "${WORK}/bin"
cat >"${WORK}/bin/gh" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
args="$*"
case "${args}" in
*/commits*) cat "${FIXTURES}/commit-message" ;;
*/files*) cat "${FIXTURES}/files" ;;
*/dependabot/alerts*)
	pkg=""
	while (($#)); do
		if [[ "$1" == "-f" && "$2" == package=* ]]; then
			pkg="${2#package=}"
		fi
		shift
	done
	f="${FIXTURES}/alerts.${pkg//\//_}"
	[[ -f "${f}" ]] && cat "${f}"
	exit 0
	;;
*)
	echo "unexpected gh call: ${args}" >&2
	exit 1
	;;
esac
STUB
chmod +x "${WORK}/bin/gh"

failures=0
case_name=""
OUT=""

# begin <name>: starts a case with empty fixtures.
begin() {
	case_name="$1"
	rm -rf "${WORK}/fixtures"
	mkdir -p "${WORK}/fixtures"
	: >"${WORK}/fixtures/files"
}

# commit <name> <old> <new>...: sets a Dependabot commit message with one
# dependency per name/old/new triple.
commit() {
	local args=("$@")
	{
		echo "chore: bump things"
		echo
		if ((${#args[@]} == 3)); then
			echo "Bumps [${args[0]}](https://example.com) from ${args[1]} to ${args[2]}."
		else
			echo "Bumps the group with ${#args[@]} updates."
			for ((i = 0; i < ${#args[@]}; i += 3)); do
				echo "Updates \`${args[i]}\` from ${args[i + 1]} to ${args[i + 2]}"
			done
		fi
		echo
		echo "---"
		echo "updated-dependencies:"
		for ((i = 0; i < ${#args[@]}; i += 3)); do
			echo "- dependency-name: ${args[i]}"
			echo "  dependency-version: ${args[i + 2]}"
			echo "  dependency-type: direct:production"
		done
		echo "..."
	} >"${WORK}/fixtures/commit-message"
}

# run: runs the script and captures GITHUB_OUTPUT in OUT.
run() {
	: >"${WORK}/output"
	FIXTURES="${WORK}/fixtures" PATH="${WORK}/bin:${PATH}" \
		PR_NUMBER=1 REPO=coder/coder GITHUB_OUTPUT="${WORK}/output" \
		bash "${SCRIPT}" >"${WORK}/stdout" 2>"${WORK}/stderr" || {
		echo "FAIL: ${case_name}: script exited non-zero"
		sed 's/^/    stderr: /' "${WORK}/stderr"
		failures=$((failures + 1))
		return
	}
	OUT=$(<"${WORK}/output")
}

# expect <matched> <alerts>
expect() {
	local want
	want=$(printf 'matched=%s\nalerts=%s' "$1" "$2")
	if [[ "${OUT}" != "${want}" ]]; then
		failures=$((failures + 1))
		echo "FAIL: ${case_name}"
		echo "    want: ${want//$'\n'/ }"
		echo "    got:  ${OUT//$'\n'/ }"
	fi
}

begin "npm: alert patched in the new version"
commit dompurify 3.4.14 3.4.17
echo "site/package.json" >"${WORK}/fixtures/files"
printf '705\tsite/package.json\t3.4.17\n' >"${WORK}/fixtures/alerts.dompurify"
run
expect true 705

begin "go: v prefix on the patched version"
commit github.com/moby/sys/user 0.4.0 0.4.1
printf 'go.mod\ngo.sum\n' >"${WORK}/fixtures/files"
printf '683\tgo.mod\tv0.4.1\n' >"${WORK}/fixtures/alerts.github.com_moby_sys_user"
run
expect true 683

begin "new version is newer than the patched version"
commit next 15.0.0 15.10.0
echo "site/package.json" >"${WORK}/fixtures/files"
printf '9\tsite/package.json\t15.9.2\n' >"${WORK}/fixtures/alerts.next"
run
expect true 9

begin "new version is older than the patched version"
commit axios 1.7.0 1.8.0
echo "site/package.json" >"${WORK}/fixtures/files"
printf '10\tsite/package.json\t1.8.4\n' >"${WORK}/fixtures/alerts.axios"
run
expect false ""

begin "version compare is numeric, not lexical"
commit axios 1.8.0 1.10.0
echo "site/package.json" >"${WORK}/fixtures/files"
printf '10\tsite/package.json\t1.9.0\n' >"${WORK}/fixtures/alerts.axios"
run
expect true 10

begin "alert is for a manifest the PR did not change"
commit dompurify 3.4.14 3.4.17
echo "offlinedocs/package.json" >"${WORK}/fixtures/files"
printf '705\tsite/package.json\t3.4.17\n' >"${WORK}/fixtures/alerts.dompurify"
run
expect false ""

begin "alert was already patched before the PR"
commit google.golang.org/grpc 1.84.0 1.85.0
echo "go.mod" >"${WORK}/fixtures/files"
printf '554\tgo.mod\t1.82.1\n' >"${WORK}/fixtures/alerts.google.golang.org_grpc"
run
expect false ""

begin "old version equals the patched version"
commit dompurify 3.4.17 3.4.18
echo "site/package.json" >"${WORK}/fixtures/files"
printf '705\tsite/package.json\t3.4.17\n' >"${WORK}/fixtures/alerts.dompurify"
run
expect false ""

begin "old version unknown: match on the new version"
printf -- '---\nupdated-dependencies:\n- dependency-name: dompurify\n  dependency-version: 3.4.17\n...\n' >"${WORK}/fixtures/commit-message"
echo "site/package.json" >"${WORK}/fixtures/files"
printf '705\tsite/package.json\t3.4.17\n' >"${WORK}/fixtures/alerts.dompurify"
run
expect true 705

begin "no alert for the package"
commit left-pad 1.3.0 1.3.1
echo "site/package.json" >"${WORK}/fixtures/files"
run
expect false ""

begin "alert has no patched version"
commit foo 1.0.0 2.0.0
echo "site/package.json" >"${WORK}/fixtures/files"
printf '11\tsite/package.json\t\n' >"${WORK}/fixtures/alerts.foo"
run
expect false ""

begin "grouped PR: one of two dependencies fixes an alert"
commit left-pad 1.3.0 1.3.1 dompurify 3.4.14 3.4.17
echo "site/package.json" >"${WORK}/fixtures/files"
printf '705\tsite/package.json\t3.4.17\n' >"${WORK}/fixtures/alerts.dompurify"
run
expect true 705

begin "several alerts for one package"
commit dompurify 3.4.14 3.4.17
echo "site/package.json" >"${WORK}/fixtures/files"
printf '705\tsite/package.json\t3.4.17\n706\tsite/package.json\t3.4.16\n' >"${WORK}/fixtures/alerts.dompurify"
run
expect true 705,706

begin "commit without Dependabot metadata"
echo "chore: manual change" >"${WORK}/fixtures/commit-message"
echo "go.mod" >"${WORK}/fixtures/files"
run
expect false ""

if ((failures > 0)); then
	echo "${failures} failure(s)"
	exit 1
fi
echo "ok"
