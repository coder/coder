#!/usr/bin/env bash
set -euo pipefail

# Smoke test for scripts/check_emdash.sh.
# Runs inside the coder repo using temporary branches/commits.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHECK_SCRIPT="${SCRIPT_DIR}/check_emdash.sh"

pass=0
fail=0

assert_exit() {
	local label="$1" expected="$2" actual="$3"
	if [[ "$actual" -eq "$expected" ]]; then
		echo "  PASS: ${label}"
		pass=$((pass + 1))
	else
		echo "  FAIL: ${label} (expected exit ${expected}, got ${actual})"
		fail=$((fail + 1))
	fi
}

assert_output_contains() {
	local label="$1" needle="$2" haystack="$3"
	if [[ "$haystack" == *"$needle"* ]]; then
		echo "  PASS: ${label}"
		pass=$((pass + 1))
	else
		echo "  FAIL: ${label} (expected output to contain '${needle}')"
		fail=$((fail + 1))
	fi
}

assert_output_not_contains() {
	local label="$1" needle="$2" haystack="$3"
	if [[ "$haystack" != *"$needle"* ]]; then
		echo "  PASS: ${label}"
		pass=$((pass + 1))
	else
		echo "  FAIL: ${label} (expected output NOT to contain '${needle}')"
		fail=$((fail + 1))
	fi
}

# Build emdash/endash from raw bytes (same approach as the script).
emdash=$'\xE2\x80\x94'
endash=$'\xE2\x80\x93'

# Save current state for cleanup.
original_branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "")
test_branch="test-check-emdash-$$"
test_file=".emdash-test-$$.txt"

cleanup() {
	rm -f "$test_file"
	if [[ -n "$original_branch" ]]; then
		git checkout -q "$original_branch" 2>/dev/null || true
	fi
	git branch -D "$test_branch" 2>/dev/null || true
}
trap cleanup EXIT

echo "--- check_emdash.sh smoke tests"

# Test 1: --all mode finds existing violations in the repo.
echo "Test 1: --all mode detects existing violations"
rc=0
bash "$CHECK_SCRIPT" --all >/dev/null 2>&1 || rc=$?
assert_exit "--all exits 1 (repo has existing violations)" 1 "$rc"

# Test 2: diff mode detects emdash in added lines.
echo "Test 2: diff mode detects emdash in added lines"
git checkout -q -b "$test_branch"
cat >"$test_file" <<EOF
clean first line
has emdash ${emdash} here
has endash ${endash} here
clean last line
EOF
git add "$test_file"
git -c core.hooksPath=/dev/null commit -q -m "test: add emdash for smoke test"

output=$(bash "$CHECK_SCRIPT" 2>&1 || true)
rc=0
bash "$CHECK_SCRIPT" >/dev/null 2>&1 || rc=$?
assert_exit "diff mode exits 1 on new violations" 1 "$rc"
assert_output_contains "finds emdash on line 2" "${test_file}:2:" "$output"
assert_output_contains "finds endash on line 3" "${test_file}:3:" "$output"
assert_output_not_contains "does not flag clean line 1" "${test_file}:1:" "$output"
assert_output_not_contains "does not flag clean line 4" "${test_file}:4:" "$output"

# Test 3: diff mode passes on clean changes.
echo "Test 3: diff mode passes on clean changes"
git checkout -q "$original_branch"
git branch -D "$test_branch" >/dev/null 2>&1
git checkout -q -b "$test_branch"
echo "just a clean addition" >"$test_file"
git add "$test_file"
git -c core.hooksPath=/dev/null commit -q -m "test: clean change"

rc=0
bash "$CHECK_SCRIPT" >/dev/null 2>&1 || rc=$?
assert_exit "diff mode exits 0 on clean diff" 0 "$rc"

echo ""
echo "Results: ${pass} passed, ${fail} failed"
if [[ "$fail" -gt 0 ]]; then
	exit 1
fi
