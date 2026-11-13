#!/usr/bin/env bash
# Tests for doc-check-reviewed-head.sh. No CI job runs this file.
#
# Run: bash .github/scripts/doc-check-reviewed-head.test.sh

set -euo pipefail

SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/doc-check-reviewed-head.sh"
WORK=$(mktemp -d)
trap 'rm -rf "${WORK}"' EXIT

readonly SHA_A="3580c80cc5fe8f4a665e8d606eea353296fdaa05"
readonly SHA_B="9f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c"

failures=0
case_name=""
OUT=""
STATUS=0

# Starts a case.
begin() {
	case_name="$1"
}

# run <HEAD_SHA> <trace>: runs the script with the trace on stdin. The trace is
# written without a final newline, as an empty trace is.
run() {
	printf '%s' "$2" >"${WORK}/trace"
	STATUS=0
	OUT=$(HEAD_SHA="$1" bash "${SCRIPT}" <"${WORK}/trace" 2>"${WORK}/stderr") || STATUS=$?
}

fail_case() {
	failures=$((failures + 1))
	echo "FAIL: ${case_name}: $*"
	sed 's/^/    stderr: /' "${WORK}/stderr"
}

# expect_out <word>: stdout is the word, or nothing for "", and the exit is 0.
expect_out() {
	if [[ "${OUT}" != "$1" ]]; then
		fail_case "stdout was $(printf '%q' "${OUT}"), want $(printf '%q' "$1")"
	fi
	if [[ "${STATUS}" != "0" ]]; then
		fail_case "exit status ${STATUS}, want 0"
	fi
}

expect_stderr() {
	grep -qF -- "$1" "${WORK}/stderr" || fail_case "stderr lacks: $1"
}

# trace_of <head_sha> <outcome>: a minimal trace block.
trace_of() {
	printf 'doc-check-trace:\n  pr: 7\n  head_sha: %s\n  outcome: %s\n' "$1" "$2"
}

begin "a clean review of the head qualifies"
run "${SHA_A}" "$(trace_of "${SHA_A}" clean)"
expect_out "clean"
expect_stderr "covered ${SHA_A} and ended with clean."

begin "a finding review of the head qualifies"
run "${SHA_A}" "$(trace_of "${SHA_A}" finding)"
expect_out "finding"

begin "a review of another head does not qualify"
run "${SHA_B}" "$(trace_of "${SHA_A}" clean)"
expect_out ""
expect_stderr "The latest review covered ${SHA_A}, not ${SHA_B}. The review runs."

begin "the head comparison ignores case in both values"
run "${SHA_A^^}" "$(trace_of "${SHA_A}" clean)"
expect_out "clean"
run "${SHA_A}" "$(trace_of "${SHA_A^^}" clean)"
expect_out "clean"

begin "a CRLF HEAD_SHA is compared without its carriage return"
run "${SHA_A}"$'\r' "$(trace_of "${SHA_A}" clean)"
expect_out "clean"

begin "a CRLF trace is read as LF"
run "${SHA_A}" "$(trace_of "${SHA_A}" clean | sed 's/$/\r/')"
expect_out "clean"

begin "CRLF in both the trace and HEAD_SHA qualifies"
run "${SHA_A^^}"$'\r' "$(trace_of "${SHA_A}" finding | sed 's/$/\r/')"
expect_out "finding"

begin "quoted values are unquoted"
run "${SHA_A}" $'doc-check-trace:\n  head_sha: "'"${SHA_A}"$'"\n  outcome: \'clean\'\n'
expect_out "clean"

begin "a trailing comment on the outcome line is ignored"
run "${SHA_A}" "$(printf 'doc-check-trace:\n  head_sha: %s\n  outcome: finding          # finding | clean | error | skipped\n' "${SHA_A}")"
expect_out "finding"

begin "trailing spaces after a value are ignored"
run "${SHA_A}" "$(printf 'doc-check-trace:\n  head_sha: %s   \n  outcome: clean  \n' "${SHA_A}")"
expect_out "clean"

begin "a trailing comment on the head line is ignored"
run "${SHA_A}" "$(printf 'doc-check-trace:\n  head_sha: %s   # full commit\n  outcome: clean\n' "${SHA_A}")"
expect_out "clean"

begin "a short SHA in the trace does not match the full head"
run "${SHA_A}" "$(trace_of "${SHA_A:0:7}" clean)"
expect_out ""
expect_stderr "The latest review covered ${SHA_A:0:7}, not ${SHA_A}."

begin "a short HEAD_SHA is rejected even when the trace repeats it"
run "${SHA_A:0:7}" "$(trace_of "${SHA_A:0:7}" clean)"
expect_out ""
expect_stderr "HEAD_SHA is not a full commit SHA. The review runs."

begin "a HEAD_SHA with a non-hex character is rejected"
run "${SHA_A:0:39}g" "$(trace_of "${SHA_A:0:39}g" clean)"
expect_out ""
expect_stderr "HEAD_SHA is not a full commit SHA."

begin "a HEAD_SHA one character too long is rejected"
run "${SHA_A}0" "$(trace_of "${SHA_A}0" clean)"
expect_out ""

begin "an empty HEAD_SHA is rejected"
run "" "$(trace_of "${SHA_A}" clean)"
expect_out ""
expect_stderr "HEAD_SHA is not a full commit SHA."

begin "an error outcome does not qualify"
run "${SHA_A}" "$(trace_of "${SHA_A}" error)"
expect_out ""
expect_stderr "its outcome is 'error', not clean or finding."

begin "a skipped outcome does not qualify"
run "${SHA_A}" "$(trace_of "${SHA_A}" skipped)"
expect_out ""

begin "an unknown outcome does not qualify"
run "${SHA_A}" "$(trace_of "${SHA_A}" 'done')"
expect_out ""

begin "an empty outcome does not qualify"
run "${SHA_A}" "$(printf 'doc-check-trace:\n  head_sha: %s\n  outcome:\n' "${SHA_A}")"
expect_out ""

begin "an outcome in capitals does not qualify"
run "${SHA_A}" "$(trace_of "${SHA_A}" Clean)"
expect_out ""

begin "a missing outcome does not qualify"
run "${SHA_A}" "$(printf 'doc-check-trace:\n  head_sha: %s\n' "${SHA_A}")"
expect_out ""
expect_stderr "its outcome is '', not clean or finding."

begin "a missing head_sha does not qualify"
run "${SHA_A}" "$(printf 'doc-check-trace:\n  outcome: clean\n')"
expect_out ""
expect_stderr "The trace has no head_sha."

begin "a key that only starts with the same name is not a match"
run "${SHA_A}" "$(printf 'doc-check-trace:\n  head_sha_full: %s\n  outcome_note: clean\n  head_sha: %s\n  outcome: clean\n' "${SHA_B}" "${SHA_A}")"
expect_out "clean"

begin "an unclosed quote does not qualify"
run "${SHA_A}" "$(printf 'doc-check-trace:\n  head_sha: %s\n  outcome: "clean\n' "${SHA_A}")"
expect_out ""

begin "empty input does not qualify"
run "${SHA_A}" ""
expect_out ""
expect_stderr "The trace has no head_sha."

begin "the first head_sha and outcome lines win"
run "${SHA_A}" "$(printf 'doc-check-trace:\n  head_sha: %s\n  outcome: clean\n  head_sha: %s\n  outcome: error\n' "${SHA_A}" "${SHA_B}")"
expect_out "clean"
run "${SHA_B}" "$(printf 'doc-check-trace:\n  head_sha: %s\n  outcome: clean\n  head_sha: %s\n  outcome: error\n' "${SHA_A}" "${SHA_B}")"
expect_out ""

begin "a realistic trace block qualifies"
realistic=$(
	cat <<EOF
Review complete. Nothing needed a comment after verification.

\`\`\`yaml
doc-check-trace:
  pr: 31159
  head_sha: ${SHA_A}
  outcome: finding          # finding | clean | error | skipped
  blocking: 3
  suggestions: 4
  rejected_by_verifier: 2
  personas_run: [accuracy, coverage, learner, consistency, audience, structure, language, verifier]
  personas_skipped: []      # each with a reason
\`\`\`
EOF
)
run "${SHA_A}" "${realistic}"
expect_out "finding"

begin "a realistic trace block for an older head does not qualify"
older=$(
	cat <<EOF
\`\`\`yaml
doc-check-trace:
  pr: 31159
  head_sha: ${SHA_A}
  outcome: clean
\`\`\`
EOF
)
run "${SHA_B}" "${older}"
expect_out ""

if ((failures > 0)); then
	echo "${failures} doc-check-reviewed-head.sh check(s) failed."
	exit 1
fi
echo "All doc-check-reviewed-head.sh cases pass."
