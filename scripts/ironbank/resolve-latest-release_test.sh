#!/usr/bin/env bash

# Tests for resolve-latest-release.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVE_SCRIPT="${SCRIPT_DIR}/resolve-latest-release.sh"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

pass=0
fail=0

assert_eq() {
	local test_name="$1"
	local expected="$2"
	local actual="$3"
	if [[ "$expected" == "$actual" ]]; then
		echo "PASS: $test_name"
		pass=$((pass + 1))
	else
		echo "FAIL: $test_name"
		echo "  expected: $expected"
		echo "  actual:   $actual"
		fail=$((fail + 1))
	fi
}

assert_fail() {
	local test_name="$1"
	local exit_code="$2"
	if [[ "$exit_code" -ne 0 ]]; then
		echo "PASS: $test_name (exited with $exit_code)"
		pass=$((pass + 1))
	else
		echo "FAIL: $test_name (expected non-zero exit, got 0)"
		fail=$((fail + 1))
	fi
}

# Test 1: Stable release is the largest version (no ESR).
cat >"${TMPDIR}/test1.md" <<'EOF'
# Releases
<!-- RELEASE_CALENDAR_START -->
| Release name                                   | Release Date      | Status                   | Latest Release                                                   |
|------------------------------------------------|-------------------|--------------------------|------------------------------------------------------------------|
| [2.30](https://coder.com/changelog/coder-2-30) | February 03, 2026 | Not Supported            | [v2.30.9](https://github.com/coder/coder/releases/tag/v2.30.9)   |
| [2.31](https://coder.com/changelog/coder-2-31) | February 23, 2026 | Not Supported            | [v2.31.14](https://github.com/coder/coder/releases/tag/v2.31.14) |
| [2.32](https://coder.com/changelog/coder-2-32) | April 14, 2026    | Security Support         | [v2.32.5](https://github.com/coder/coder/releases/tag/v2.32.5)   |
| [2.33](https://coder.com/changelog/coder-2-33) | May 05, 2026      | Stable                   | [v2.33.6](https://github.com/coder/coder/releases/tag/v2.33.6)   |
| [2.34](https://coder.com/changelog/coder-2-34) | June 02, 2026     | Mainline                 | [v2.34.0](https://github.com/coder/coder/releases/tag/v2.34.0)   |
| 2.35                                           |                   | Not Released             | N/A                                                              |
<!-- RELEASE_CALENDAR_END -->
EOF
result="$(bash "$RESOLVE_SCRIPT" --index-file "${TMPDIR}/test1.md")"
assert_eq "Stable only (no ESR)" "v2.33.6" "$result"

# Test 2: ESR release is larger than Stable.
cat >"${TMPDIR}/test2.md" <<'EOF'
<!-- RELEASE_CALENDAR_START -->
| Release name | Release Date | Status | Latest Release |
|--|--|--|--|
| [2.33](https://coder.com/changelog/coder-2-33) | May 05, 2026 | Stable | [v2.33.6](https://github.com/coder/coder/releases/tag/v2.33.6) |
| [2.34](https://coder.com/changelog/coder-2-34) | June 02, 2026 | Mainline (ESR) | [v2.34.0](https://github.com/coder/coder/releases/tag/v2.34.0) |
<!-- RELEASE_CALENDAR_END -->
EOF
result="$(bash "$RESOLVE_SCRIPT" --index-file "${TMPDIR}/test2.md")"
assert_eq "ESR larger than Stable" "v2.34.0" "$result"

# Test 3: Older ESR release exists alongside Stable; Stable wins as larger.
cat >"${TMPDIR}/test3.md" <<'EOF'
<!-- RELEASE_CALENDAR_START -->
| Release name | Release Date | Status | Latest Release |
|--|--|--|--|
| [2.29](https://coder.com/changelog/coder-2-29) | December 02, 2025 | Extended Support Release | [v2.29.16](https://github.com/coder/coder/releases/tag/v2.29.16) |
| [2.33](https://coder.com/changelog/coder-2-33) | May 05, 2026 | Stable | [v2.33.6](https://github.com/coder/coder/releases/tag/v2.33.6) |
| [2.34](https://coder.com/changelog/coder-2-34) | June 02, 2026 | Mainline | [v2.34.0](https://github.com/coder/coder/releases/tag/v2.34.0) |
<!-- RELEASE_CALENDAR_END -->
EOF
result="$(bash "$RESOLVE_SCRIPT" --index-file "${TMPDIR}/test3.md")"
assert_eq "Older ESR + Stable = Stable wins" "v2.33.6" "$result"

# Test 4: Real-world table from coder/coder main branch.
cat >"${TMPDIR}/test4.md" <<'EOF'
<!-- RELEASE_CALENDAR_START -->
| Release name                                   | Release Date      | Status                   | Latest Release                                                   |
|------------------------------------------------|-------------------|--------------------------|------------------------------------------------------------------|
| [2.29](https://coder.com/changelog/coder-2-29) | December 02, 2025 | Extended Support Release | [v2.29.16](https://github.com/coder/coder/releases/tag/v2.29.16) |
| [2.30](https://coder.com/changelog/coder-2-30) | February 03, 2026 | Not Supported            | [v2.30.9](https://github.com/coder/coder/releases/tag/v2.30.9)   |
| [2.31](https://coder.com/changelog/coder-2-31) | February 23, 2026 | Not Supported            | [v2.31.14](https://github.com/coder/coder/releases/tag/v2.31.14) |
| [2.32](https://coder.com/changelog/coder-2-32) | April 14, 2026    | Security Support         | [v2.32.5](https://github.com/coder/coder/releases/tag/v2.32.5)   |
| [2.33](https://coder.com/changelog/coder-2-33) | May 05, 2026      | Stable                   | [v2.33.6](https://github.com/coder/coder/releases/tag/v2.33.6)   |
| [2.34](https://coder.com/changelog/coder-2-34) | June 02, 2026     | Mainline (ESR)           | [v2.34.0](https://github.com/coder/coder/releases/tag/v2.34.0)   |
| 2.35                                           |                   | Not Released             | N/A                                                              |
<!-- RELEASE_CALENDAR_END -->
EOF
result="$(bash "$RESOLVE_SCRIPT" --index-file "${TMPDIR}/test4.md")"
assert_eq "Real-world table: ESR 2.34 > Stable 2.33" "v2.34.0" "$result"

# Test 5: No Stable or ESR releases at all.
cat >"${TMPDIR}/test5.md" <<'EOF'
<!-- RELEASE_CALENDAR_START -->
| Release name | Release Date | Status | Latest Release |
|--|--|--|--|
| [2.34](https://coder.com/changelog/coder-2-34) | June 02, 2026 | Mainline | [v2.34.0](https://github.com/coder/coder/releases/tag/v2.34.0) |
| 2.35 | | Not Released | N/A |
<!-- RELEASE_CALENDAR_END -->
EOF
set +e
result="$(bash "$RESOLVE_SCRIPT" --index-file "${TMPDIR}/test5.md" 2>/dev/null)"
exit_code=$?
set -e
assert_fail "No Stable/ESR exits non-zero" "$exit_code"

# Test 6: Multiple ESR releases; picks the largest.
cat >"${TMPDIR}/test6.md" <<'EOF'
<!-- RELEASE_CALENDAR_START -->
| Release name | Release Date | Status | Latest Release |
|--|--|--|--|
| [2.24](https://coder.com/changelog/coder-2-24) | July 01, 2025 | Extended Support Release | [v2.24.12](https://github.com/coder/coder/releases/tag/v2.24.12) |
| [2.29](https://coder.com/changelog/coder-2-29) | December 02, 2025 | Extended Support Release | [v2.29.16](https://github.com/coder/coder/releases/tag/v2.29.16) |
| [2.33](https://coder.com/changelog/coder-2-33) | May 05, 2026 | Stable | [v2.33.6](https://github.com/coder/coder/releases/tag/v2.33.6) |
<!-- RELEASE_CALENDAR_END -->
EOF
result="$(bash "$RESOLVE_SCRIPT" --index-file "${TMPDIR}/test6.md")"
assert_eq "Multiple ESR + Stable = Stable wins (largest)" "v2.33.6" "$result"

# Test 7: ESR-only (no Stable release present).
cat >"${TMPDIR}/test7.md" <<'EOF'
<!-- RELEASE_CALENDAR_START -->
| Release name | Release Date | Status | Latest Release |
|--|--|--|--|
| [2.29](https://coder.com/changelog/coder-2-29) | December 02, 2025 | Extended Support Release | [v2.29.16](https://github.com/coder/coder/releases/tag/v2.29.16) |
| [2.34](https://coder.com/changelog/coder-2-34) | June 02, 2026 | Mainline | [v2.34.0](https://github.com/coder/coder/releases/tag/v2.34.0) |
<!-- RELEASE_CALENDAR_END -->
EOF
result="$(bash "$RESOLVE_SCRIPT" --index-file "${TMPDIR}/test7.md")"
assert_eq "ESR-only (no Stable)" "v2.29.16" "$result"

# Test 8: Stable (ESR) combined status.
cat >"${TMPDIR}/test8.md" <<'EOF'
<!-- RELEASE_CALENDAR_START -->
| Release name | Release Date | Status | Latest Release |
|--|--|--|--|
| [2.29](https://coder.com/changelog/coder-2-29) | December 02, 2025 | Extended Support Release | [v2.29.16](https://github.com/coder/coder/releases/tag/v2.29.16) |
| [2.33](https://coder.com/changelog/coder-2-33) | May 05, 2026 | Stable (ESR) | [v2.33.6](https://github.com/coder/coder/releases/tag/v2.33.6) |
| [2.34](https://coder.com/changelog/coder-2-34) | June 02, 2026 | Mainline | [v2.34.0](https://github.com/coder/coder/releases/tag/v2.34.0) |
<!-- RELEASE_CALENDAR_END -->
EOF
result="$(bash "$RESOLVE_SCRIPT" --index-file "${TMPDIR}/test8.md")"
assert_eq "Stable (ESR) combined status" "v2.33.6" "$result"

echo ""
echo "Results: $pass passed, $fail failed"
if [[ "$fail" -gt 0 ]]; then
	exit 1
fi
