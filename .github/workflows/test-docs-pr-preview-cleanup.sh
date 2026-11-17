#!/usr/bin/env bash
# Tests for the script in the "Delete closed pull requests' previews" step of
# docs-pr-preview-cleanup.yaml. The script is read out of the workflow and run
# against stand-ins for curl and gh, so there is no copy to keep in sync.
# No CI job runs this file.
#
# Run: bash .github/workflows/test-docs-pr-preview-cleanup.sh

set -euo pipefail

WORKFLOW="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/docs-pr-preview-cleanup.yaml"
WORK=$(mktemp -d)
trap 'rm -rf "${WORK}"' EXIT
mkdir "${WORK}/bin"

# The step's run block: the lines under "run: |" without their indent, up to
# the first line indented less than the block.
awk '
	/^      - name: Delete closed pull requests/ { step = 1 }
	step && /^        run: \|$/ { body = 1; next }
	body {
		if ($0 ~ /[^ ]/) {
			match($0, /^ */)
			if (RLENGTH < 10) exit
		}
		print substr($0, 11)
	}
' "${WORKFLOW}" >"${WORK}/cleanup.sh"
if ! grep -q 'list_deployments' "${WORK}/cleanup.sh"; then
	echo "FAIL: could not read the cleanup script out of ${WORKFLOW}"
	exit 1
fi

readonly SHA_A="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
readonly SHA_B="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
readonly SHA_C="cccccccccccccccccccccccccccccccccccccccc"

# The preview project: two builds of pull request 101 on different pages, one
# of 102, and a deployment no pull request is recorded on.
cat >"${WORK}/page1.json" <<EOF
{"deployments": [
  {"uid": "dpl_101_a", "meta": {"pr": "101", "sha": "${SHA_A}"}},
  {"uid": "dpl_102_b", "meta": {"pr": "102", "sha": "${SHA_B}"}},
  {"uid": "dpl_untagged", "meta": {}}
], "pagination": {"next": 1700000000000}}
EOF
cat >"${WORK}/page2.json" <<EOF
{"deployments": [
  {"uid": "dpl_101_c", "meta": {"pr": "101", "sha": "${SHA_C}"}}
], "pagination": {"next": null}}
EOF

# Stand-in for curl: answers Secret Manager and the Vercel API from the pages
# above, and records the path of every DELETE. "dG9rZW4=" is "token".
cat >"${WORK}/bin/curl" <<'EOF'
#!/usr/bin/env bash
method=GET
url=""
while [ $# -gt 0 ]; do
	case "$1" in
	-X)
		method=$2
		shift 2
		;;
	-H | -o | -w | --retry) shift 2 ;;
	https://*)
		url=$1
		shift
		;;
	*) shift ;;
	esac
done
case "${method} ${url}" in
"GET https://secretmanager.googleapis.com/"*)
	printf '{"payload": {"data": "dG9rZW4="}}'
	;;
"GET https://api.vercel.com/v6/deployments?"*)
	if [ "${STUB_LIST}" = refused ]; then
		printf '{"error": {"code": "forbidden"}}'
		exit 22
	fi
	case "${url}" in
	*"&until="*) cat "${WORK}/page2.json" ;;
	*) cat "${WORK}/page1.json" ;;
	esac
	;;
"DELETE https://api.vercel.com/"*)
	path=${url#https://api.vercel.com/}
	echo "${path%%\?*}" >>"${WORK}/deleted"
	printf '200'
	;;
*)
	echo "unexpected request: ${method} ${url}" >&2
	exit 22
	;;
esac
EOF

# Stand-in for gh: pull request 101 is closed, every other one is open.
cat >"${WORK}/bin/gh" <<'EOF'
#!/usr/bin/env bash
case "$* " in
*"/pulls/101 "*) echo closed ;;
*) echo open ;;
esac
EOF
chmod +x "${WORK}/bin/curl" "${WORK}/bin/gh"

failures=0
STATUS=0
DELETED=""

# run <closed pull request, or "" for a groom> <ok|refused>: runs the script
# the way the runner does. DELETED is every deleted path, sorted.
run() {
	: >"${WORK}/deleted"
	STATUS=0
	env PATH="${WORK}/bin:${PATH}" WORK="${WORK}" STUB_LIST="$2" \
		REPO=coder/coder CLOSED_PR="$1" GH_TOKEN=x GCP_ACCESS_TOKEN=x \
		VERCEL_TOKEN_SECRET=projects/p/secrets/s/versions/1 \
		VERCEL_ORG_ID=team VERCEL_PROJECT_ID=prj \
		DOCS_PREVIEW_DOMAIN=docs.example.dev \
		bash --noprofile --norc -eo pipefail "${WORK}/cleanup.sh" \
		>"${WORK}/stdout" 2>"${WORK}/stderr" || STATUS=$?
	DELETED=$(LC_ALL=C sort "${WORK}/deleted" | tr '\n' ' ')
}

# expect <name> <want: zero|nonzero> <deleted paths>
expect() {
	local got=zero
	if [ "${STATUS}" -ne 0 ]; then
		got=nonzero
	fi
	if [ "${got}" = "$2" ] && [ "${DELETED}" = "$3" ]; then
		echo "PASS: $1"
		return
	fi
	failures=$((failures + 1))
	echo "FAIL: $1"
	echo "    exit status ${STATUS}, want $2"
	echo "    deleted: ${DELETED:-nothing}"
	echo "    want:    ${3:-nothing}"
	sed 's/^/    stderr: /' "${WORK}/stderr"
}

# A listing Vercel refuses must fail the groom. It once read as an empty
# project, and the groom reported success while every preview stayed up.
run "" refused
expect "a groom fails when the deployments cannot be listed" nonzero ""

# The groom removes a closed pull request's builds from every page, with
# their aliases, and leaves an open pull request's and an untagged one.
run "" ok
expect "a groom deletes only closed pull requests' previews" zero \
	"v13/deployments/dpl_101_a v13/deployments/dpl_101_c v2/aliases/pr-101-aaaaaaaaa.docs.example.dev v2/aliases/pr-101-ccccccccc.docs.example.dev v2/aliases/pr-101.docs.example.dev "

# A close removes that pull request's previews and nothing else.
run 102 ok
expect "a close deletes that pull request's previews" zero \
	"v13/deployments/dpl_102_b v2/aliases/pr-102-bbbbbbbbb.docs.example.dev v2/aliases/pr-102.docs.example.dev "

if [ "${failures}" -gt 0 ]; then
	echo ""
	echo "${failures} test(s) failed."
	exit 1
fi

echo ""
echo "All tests passed."
