#!/usr/bin/env bats

# Tests for add-finding.sh

SCRIPT_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
ADD_FINDING="$SCRIPT_DIR/add-finding.sh"

setup() {
	TEST_TMPDIR="$(mktemp -d)"
	OUTPUT="$TEST_TMPDIR/findings.json"
}

teardown() {
	rm -rf "$TEST_TMPDIR"
}

@test "first finding creates valid JSON array" {
	run "$ADD_FINDING" \
		--output "$OUTPUT" \
		--severity P2 \
		--file "coderd/handler.go" \
		--line 42 \
		--summary "Test passes by coincidence" \
		--evidence "The assertion checks length > 0 but the slice is always non-empty" \
		--reviewer "test-auditor"
	[ "$status" -eq 0 ]
	# Output file must be valid JSON.
	jq . "$OUTPUT" >/dev/null
	# Must be an array with one element.
	count="$(jq 'length' "$OUTPUT")"
	[ "$count" -eq 1 ]
	# Verify fields.
	[ "$(jq -r '.[0].severity' "$OUTPUT")" = "P2" ]
	[ "$(jq -r '.[0].file' "$OUTPUT")" = "coderd/handler.go" ]
	[ "$(jq '.[0].line' "$OUTPUT")" = "42" ]
	[ "$(jq -r '.[0].reviewer' "$OUTPUT")" = "test-auditor" ]
}

@test "second finding appends to existing array" {
	"$ADD_FINDING" \
		--output "$OUTPUT" \
		--severity P1 \
		--file "coderd/a.go" \
		--line 10 \
		--summary "First finding" \
		--evidence "Evidence one" \
		--reviewer "security-reviewer"

	"$ADD_FINDING" \
		--output "$OUTPUT" \
		--severity P3 \
		--file "coderd/b.go" \
		--line 20 \
		--summary "Second finding" \
		--evidence "Evidence two" \
		--reviewer "structural-reviewer"

	count="$(jq 'length' "$OUTPUT")"
	[ "$count" -eq 2 ]
	[ "$(jq -r '.[0].summary' "$OUTPUT")" = "First finding" ]
	[ "$(jq -r '.[1].summary' "$OUTPUT")" = "Second finding" ]
}

@test "validates severity — rejects invalid values" {
	run "$ADD_FINDING" \
		--output "$OUTPUT" \
		--severity INVALID \
		--file "coderd/handler.go" \
		--line 42 \
		--summary "Bad severity" \
		--evidence "Evidence" \
		--reviewer "test-auditor"
	[ "$status" -ne 0 ]
}

@test "handles multi-line evidence with special characters" {
	evidence=$'Line one with "quotes"\nLine two with \\backslashes\\\nLine three with $dollars and {braces}'
	run "$ADD_FINDING" \
		--output "$OUTPUT" \
		--severity P2 \
		--file "coderd/handler.go" \
		--line 42 \
		--summary "Special chars test" \
		--evidence "$evidence" \
		--reviewer "test-auditor"
	[ "$status" -eq 0 ]
	# Output must be valid JSON.
	jq . "$OUTPUT" >/dev/null
	# Evidence must be preserved (jq -r renders it back).
	got="$(jq -r '.[0].evidence' "$OUTPUT")"
	[ "$got" = "$evidence" ]
}

@test "observation — severity is Obs, no line required" {
	run "$ADD_FINDING" \
		--output "$OUTPUT" \
		--severity Obs \
		--summary "General observation about architecture" \
		--reviewer "structural-reviewer"
	[ "$status" -eq 0 ]
	jq . "$OUTPUT" >/dev/null
	count="$(jq 'length' "$OUTPUT")"
	[ "$count" -eq 1 ]
	[ "$(jq '.[0].file' "$OUTPUT")" = "null" ]
	[ "$(jq '.[0].line' "$OUTPUT")" = "null" ]
	[ "$(jq '.[0].evidence' "$OUTPUT")" = "null" ]
	[ "$(jq -r '.[0].severity' "$OUTPUT")" = "Obs" ]
}

@test "nit — severity is Nit" {
	run "$ADD_FINDING" \
		--output "$OUTPUT" \
		--severity Nit \
		--file "coderd/handler.go" \
		--summary "Nit about naming" \
		--reviewer "structural-reviewer"
	[ "$status" -eq 0 ]
	jq . "$OUTPUT" >/dev/null
	count="$(jq 'length' "$OUTPUT")"
	[ "$count" -eq 1 ]
	[ "$(jq -r '.[0].file' "$OUTPUT")" = "coderd/handler.go" ]
	[ "$(jq '.[0].line' "$OUTPUT")" = "null" ]
	[ "$(jq '.[0].evidence' "$OUTPUT")" = "null" ]
	[ "$(jq -r '.[0].severity' "$OUTPUT")" = "Nit" ]
}

@test "missing required flags exits non-zero" {
	# Omit --summary, which is required for all severities.
	run "$ADD_FINDING" \
		--output "$OUTPUT" \
		--severity P2 \
		--file "coderd/handler.go" \
		--line 42 \
		--evidence "Some evidence" \
		--reviewer "test-auditor"
	[ "$status" -ne 0 ]
}

@test "output file is always valid JSON after each call" {
	for i in 1 2 3; do
		"$ADD_FINDING" \
			--output "$OUTPUT" \
			--severity P2 \
			--file "coderd/handler.go" \
			--line "$i" \
			--summary "Finding number $i" \
			--evidence "Evidence $i" \
			--reviewer "test-auditor"
		# Must be valid JSON after every call.
		jq . "$OUTPUT" >/dev/null
	done
	count="$(jq 'length' "$OUTPUT")"
	[ "$count" -eq 3 ]
}
