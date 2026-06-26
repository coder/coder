#!/usr/bin/env bash
# Regression tests for the release.published branch in the "Compute
# action and ref" step of deploy-docs.yaml. The workflow translates a
# stable vX.Y.Z release tag into its release/X.Y branch and skips
# prereleases or non-semver tags. This script exercises that bash
# block against the documented event sources (push, workflow_dispatch,
# release.published) plus regex boundary cases so we can catch
# regressions in the regex, the prerelease gate, or either early-exit
# path without spinning up the full workflow.
#
# Keep compute_action_ref below in sync with deploy-docs.yaml. The
# workflow comment "Tested in test-deploy-docs-release.sh" is the
# contract.

set -euo pipefail

# compute_action_ref runs the workflow's release-event logic in a
# subshell so its `exit 0` only ends one invocation. Reads EVENT_NAME,
# RELEASE_TAG, RELEASE_PRERELEASE, INPUT_ACTION, INPUT_REF, and
# GITHUB_REF_NAME from the environment and prints lines compatible
# with the tests below:
#   * release skip: stdout has the `::notice::` line, no ACTION/REF.
#   * release accept: stdout has ACTION=, REF=, and the `::notice::`
#     line, in the same order as the workflow.
#   * push/workflow_dispatch: stdout has ACTION= and REF= only.
#
# This duplicates the workflow block byte-for-byte. Update both
# together; the assertions below describe the contract.
compute_action_ref() {
	(
		set -u
		ACTION=""
		REF=""
		if [ "${EVENT_NAME:-}" = "release" ]; then
			if [ "${RELEASE_PRERELEASE:-false}" = "true" ]; then
				echo "::notice::Skipping prerelease ${RELEASE_TAG:-<unknown>}; no docs reindex."
				exit 0
			fi
			if [[ "${RELEASE_TAG:-}" =~ ^v([0-9]+)\.([0-9]+)\.[0-9]+$ ]]; then
				ACTION="index"
				REF="release/${BASH_REMATCH[1]}.${BASH_REMATCH[2]}"
				echo "::notice::Release ${RELEASE_TAG} resolved to ref ${REF}."
			else
				echo "::notice::Skipping ${RELEASE_TAG:-<unknown>}: not a plain vX.Y.Z release tag."
				exit 0
			fi
		fi
		ACTION="${ACTION:-${INPUT_ACTION:-index}}"
		REF="${REF:-${INPUT_REF:-$GITHUB_REF_NAME}}"
		echo "ACTION=$ACTION"
		echo "REF=$REF"
	)
}

failures=0
section=""

start_section() {
	section="$1"
	echo
	echo "--- $section ---"
}

# run_case clears the relevant env vars and runs the function with the
# values from the scenario. Captures stdout into a string the test can
# assert against. Unset vars use the function's :- defaults so the
# tests exercise the same fallbacks the workflow does.
run_case() {
	local event_name="$1"
	local release_tag="$2"
	local release_prerelease="$3"
	local input_action="$4"
	local input_ref="$5"
	local github_ref_name="$6"
	EVENT_NAME="$event_name" \
		RELEASE_TAG="$release_tag" \
		RELEASE_PRERELEASE="$release_prerelease" \
		INPUT_ACTION="$input_action" \
		INPUT_REF="$input_ref" \
		GITHUB_REF_NAME="$github_ref_name" \
		compute_action_ref
}

# assert_equals checks the captured output against the expected lines
# joined by literal newlines. Quoting prevents shell expansion of `*`
# or `$` inside the expected payload.
assert_equals() {
	local description="$1"
	local actual="$2"
	local expected="$3"
	if [ "$actual" = "$expected" ]; then
		printf 'ok  %s\n' "$description"
	else
		printf 'FAIL %s\n' "$description"
		printf '  expected:\n'
		printf '%s\n' "$expected" | sed 's/^/    /'
		printf '  actual:\n'
		printf '%s\n' "$actual" | sed 's/^/    /'
		failures=$((failures + 1))
	fi
}

# Each scenario names its event source so a future reader can match a
# test to the workflow path it exercises without reading the bash.

# ---------------------------------------------------------------
start_section "push event (existing behavior)"
# ---------------------------------------------------------------

actual=$(run_case "push" "" "" "" "" "main")
assert_equals "push to main keeps ACTION=index, REF=main" \
	"$actual" \
	$'ACTION=index\nREF=main'

actual=$(run_case "push" "" "" "" "" "release/2.34")
assert_equals "push to release/2.34 keeps ACTION=index, REF=release/2.34" \
	"$actual" \
	$'ACTION=index\nREF=release/2.34'

# ---------------------------------------------------------------
start_section "workflow_dispatch event (existing behavior)"
# ---------------------------------------------------------------

actual=$(run_case "workflow_dispatch" "" "" "index" "release/2.34" "main")
assert_equals "workflow_dispatch index release/2.34 honors inputs" \
	"$actual" \
	$'ACTION=index\nREF=release/2.34'

actual=$(run_case "workflow_dispatch" "" "" "delete" "release/2.31" "main")
assert_equals "workflow_dispatch delete release/2.31 honors inputs" \
	"$actual" \
	$'ACTION=delete\nREF=release/2.31'

# ---------------------------------------------------------------
start_section "release.published event (new in DOCS-327)"
# ---------------------------------------------------------------

actual=$(run_case "release" "v2.35.0" "false" "" "" "")
assert_equals "stable v2.35.0 resolves to release/2.35" \
	"$actual" \
	$'::notice::Release v2.35.0 resolved to ref release/2.35.\nACTION=index\nREF=release/2.35'

actual=$(run_case "release" "v2.35.0-rc.1" "true" "" "" "")
assert_equals "marked prerelease v2.35.0-rc.1 is skipped, no ACTION/REF" \
	"$actual" \
	'::notice::Skipping prerelease v2.35.0-rc.1; no docs reindex.'

actual=$(run_case "release" "v2.35.0-rc.1" "false" "" "" "")
assert_equals "rc tag without prerelease flag fails regex and is skipped" \
	"$actual" \
	'::notice::Skipping v2.35.0-rc.1: not a plain vX.Y.Z release tag.'

actual=$(run_case "release" "v2.35" "false" "" "" "")
assert_equals "two-segment v2.35 fails regex and is skipped" \
	"$actual" \
	'::notice::Skipping v2.35: not a plain vX.Y.Z release tag.'

actual=$(run_case "release" "release-2.35" "false" "" "" "")
assert_equals "release-2.35 fails regex and is skipped" \
	"$actual" \
	'::notice::Skipping release-2.35: not a plain vX.Y.Z release tag.'

# v0.0.0 satisfies the regex by design. Defense in depth lives in the
# downstream allowlist gate and the workflow's main|release/* case
# validator; this test pins the regex behavior so a future tightening
# is intentional.
actual=$(run_case "release" "v0.0.0" "false" "" "" "")
assert_equals "v0.0.0 satisfies the regex; allowlist is the gate" \
	"$actual" \
	$'::notice::Release v0.0.0 resolved to ref release/0.0.\nACTION=index\nREF=release/0.0'

# Empty tag with prerelease unset reaches the non-semver skip and
# prints <unknown> for the tag. The :- defaults in the workflow
# determine the substitution; this test pins both.
actual=$(EVENT_NAME=release \
	GITHUB_REF_NAME='' \
	INPUT_ACTION='' \
	INPUT_REF='' \
	RELEASE_TAG='' \
	RELEASE_PRERELEASE='' \
	compute_action_ref)
assert_equals "empty tag with prerelease unset prints <unknown> and skips" \
	"$actual" \
	'::notice::Skipping <unknown>: not a plain vX.Y.Z release tag.'

# ---------------------------------------------------------------
start_section "regex boundary cases"
# ---------------------------------------------------------------

# Multi-digit minor and patch components should resolve, since
# backports may carry doc updates worth reindexing.
actual=$(run_case "release" "v2.100.42" "false" "" "" "")
assert_equals "multi-digit minor and patch resolve correctly" \
	"$actual" \
	$'::notice::Release v2.100.42 resolved to ref release/2.100.\nACTION=index\nREF=release/2.100'

# Trailing build metadata is not a plain vX.Y.Z, so it is skipped.
actual=$(run_case "release" "v2.35.0+build.1" "false" "" "" "")
assert_equals "semver build metadata is skipped" \
	"$actual" \
	'::notice::Skipping v2.35.0+build.1: not a plain vX.Y.Z release tag.'

# Leading whitespace is not a plain vX.Y.Z; the workflow rejects
# malformed tags instead of trimming them.
actual=$(run_case "release" " v2.35.0" "false" "" "" "")
assert_equals "leading whitespace fails the regex" \
	"$actual" \
	$'::notice::Skipping  v2.35.0: not a plain vX.Y.Z release tag.'

if [ "$failures" -gt 0 ]; then
	echo
	echo "$failures test(s) failed."
	exit 1
fi

echo
echo "All tests passed."
