#!/usr/bin/env bats

# Tests for compile-findings.sh

SCRIPT_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
COMPILE_FINDINGS="$SCRIPT_DIR/compile-findings.sh"

setup() {
	TEST_DIR="$(mktemp -d)"
}

teardown() {
	rm -rf "$TEST_DIR"
}

@test "reads all reviewer JSON files from directory" {
	cat >"$TEST_DIR/test-auditor.json" <<'EOF'
[
  {
    "severity": "P2",
    "file": "coderd/handler.go",
    "line": 42,
    "summary": "Test passes by coincidence",
    "evidence": "The assertion checks length > 0",
    "reviewer": "test-auditor"
  }
]
EOF
	cat >"$TEST_DIR/contract-auditor.json" <<'EOF'
[
  {
    "severity": "P3",
    "file": "coderd/routes.go",
    "line": 10,
    "summary": "Missing error check",
    "evidence": "The error is ignored",
    "reviewer": "contract-auditor"
  }
]
EOF

	run "$COMPILE_FINDINGS" --dir "$TEST_DIR"
	[ "$status" -eq 0 ]
	total="$(echo "$output" | jq '.stats.total_findings')"
	[ "$total" -eq 2 ]
	reviewers="$(echo "$output" | jq '.stats.reviewers_reporting | length')"
	[ "$reviewers" -eq 2 ]
}

@test "deduplicates findings at exact same file:line with same severity" {
	cat >"$TEST_DIR/auditor-a.json" <<'EOF'
[
  {
    "severity": "P2",
    "file": "coderd/handler.go",
    "line": 42,
    "summary": "Finding from A",
    "evidence": "Evidence A",
    "reviewer": "auditor-a"
  }
]
EOF
	cat >"$TEST_DIR/auditor-b.json" <<'EOF'
[
  {
    "severity": "P2",
    "file": "coderd/handler.go",
    "line": 42,
    "summary": "Finding from B",
    "evidence": "Evidence B",
    "reviewer": "auditor-b"
  }
]
EOF

	run "$COMPILE_FINDINGS" --dir "$TEST_DIR"
	[ "$status" -eq 0 ]
	# Same file:line should be grouped into one finding.
	total="$(echo "$output" | jq '.stats.total_findings')"
	[ "$total" -eq 1 ]
	# Both reviewers should be listed.
	reviewer_count="$(echo "$output" | jq '.findings[0].reviewers | length')"
	[ "$reviewer_count" -eq 2 ]
}

@test "convergent findings — same file:line, different reviewers — grouped" {
	cat >"$TEST_DIR/test-auditor.json" <<'EOF'
[
  {
    "severity": "P2",
    "file": "coderd/handler.go",
    "line": 42,
    "summary": "Test passes by coincidence",
    "evidence": "The assertion checks...",
    "reviewer": "test-auditor"
  }
]
EOF
	cat >"$TEST_DIR/contract-auditor.json" <<'EOF'
[
  {
    "severity": "P1",
    "file": "coderd/handler.go",
    "line": 42,
    "summary": "Contract violation",
    "evidence": "The contract requires...",
    "reviewer": "contract-auditor"
  }
]
EOF

	run "$COMPILE_FINDINGS" --dir "$TEST_DIR"
	[ "$status" -eq 0 ]
	total="$(echo "$output" | jq '.stats.total_findings')"
	[ "$total" -eq 1 ]
	convergent="$(echo "$output" | jq '.findings[0].convergent')"
	[ "$convergent" = "true" ]
	reviewer_count="$(echo "$output" | jq '.findings[0].reviewers | length')"
	[ "$reviewer_count" -eq 2 ]
}

@test "max severity computed across convergent findings" {
	cat >"$TEST_DIR/reviewer-a.json" <<'EOF'
[
  {
    "severity": "P2",
    "file": "coderd/handler.go",
    "line": 42,
    "summary": "Minor issue",
    "evidence": "Some evidence",
    "reviewer": "reviewer-a"
  }
]
EOF
	cat >"$TEST_DIR/reviewer-b.json" <<'EOF'
[
  {
    "severity": "P1",
    "file": "coderd/handler.go",
    "line": 42,
    "summary": "Serious issue",
    "evidence": "More evidence",
    "reviewer": "reviewer-b"
  }
]
EOF

	run "$COMPILE_FINDINGS" --dir "$TEST_DIR"
	[ "$status" -eq 0 ]
	max_sev="$(echo "$output" | jq -r '.findings[0].max_severity')"
	[ "$max_sev" = "P1" ]
	# The finding-level summary should come from the P1 reviewer.
	summary="$(echo "$output" | jq -r '.findings[0].summary')"
	[ "$summary" = "Serious issue" ]
}

@test "preserves individual reviewer severity in attribution" {
	cat >"$TEST_DIR/reviewer-a.json" <<'EOF'
[
  {
    "severity": "P2",
    "file": "coderd/handler.go",
    "line": 42,
    "summary": "Minor issue",
    "evidence": "Some evidence",
    "reviewer": "reviewer-a"
  }
]
EOF
	cat >"$TEST_DIR/reviewer-b.json" <<'EOF'
[
  {
    "severity": "P1",
    "file": "coderd/handler.go",
    "line": 42,
    "summary": "Serious issue",
    "evidence": "More evidence",
    "reviewer": "reviewer-b"
  }
]
EOF

	run "$COMPILE_FINDINGS" --dir "$TEST_DIR"
	[ "$status" -eq 0 ]
	# Check individual reviewer severities are preserved.
	sev_a="$(echo "$output" | jq -r '.findings[0].reviewers[] | select(.role == "reviewer-a") | .severity')"
	[ "$sev_a" = "P2" ]
	sev_b="$(echo "$output" | jq -r '.findings[0].reviewers[] | select(.role == "reviewer-b") | .severity')"
	[ "$sev_b" = "P1" ]
}

@test "handles mixed P-level, Obs, and Nit findings" {
	cat >"$TEST_DIR/reviewer.json" <<'EOF'
[
  {
    "severity": "P1",
    "file": "a.go",
    "line": 1,
    "summary": "P1 finding",
    "evidence": "ev",
    "reviewer": "r1"
  },
  {
    "severity": "P2",
    "file": "b.go",
    "line": 2,
    "summary": "P2 finding",
    "evidence": "ev",
    "reviewer": "r1"
  },
  {
    "severity": "P2",
    "file": "c.go",
    "line": 3,
    "summary": "Another P2",
    "evidence": "ev",
    "reviewer": "r1"
  },
  {
    "severity": "Obs",
    "file": "d.go",
    "line": 4,
    "summary": "Observation",
    "evidence": "ev",
    "reviewer": "r1"
  },
  {
    "severity": "Nit",
    "file": "e.go",
    "line": 5,
    "summary": "Nitpick",
    "evidence": "ev",
    "reviewer": "r1"
  },
  {
    "severity": "Nit",
    "file": "f.go",
    "line": 6,
    "summary": "Another nit",
    "evidence": "ev",
    "reviewer": "r1"
  }
]
EOF

	run "$COMPILE_FINDINGS" --dir "$TEST_DIR"
	[ "$status" -eq 0 ]
	total="$(echo "$output" | jq '.stats.total_findings')"
	[ "$total" -eq 6 ]
	p0="$(echo "$output" | jq '.stats.by_severity.P0')"
	[ "$p0" -eq 0 ]
	p1="$(echo "$output" | jq '.stats.by_severity.P1')"
	[ "$p1" -eq 1 ]
	p2="$(echo "$output" | jq '.stats.by_severity.P2')"
	[ "$p2" -eq 2 ]
	p3="$(echo "$output" | jq '.stats.by_severity.P3')"
	[ "$p3" -eq 0 ]
	p4="$(echo "$output" | jq '.stats.by_severity.P4')"
	[ "$p4" -eq 0 ]
	obs="$(echo "$output" | jq '.stats.by_severity.Obs')"
	[ "$obs" -eq 1 ]
	nit="$(echo "$output" | jq '.stats.by_severity.Nit')"
	[ "$nit" -eq 2 ]
}

@test "empty reviewer files (no findings) are skipped" {
	cat >"$TEST_DIR/empty-reviewer.json" <<'EOF'
[]
EOF
	cat >"$TEST_DIR/real-reviewer.json" <<'EOF'
[
  {
    "severity": "P3",
    "file": "x.go",
    "line": 1,
    "summary": "Real finding",
    "evidence": "ev",
    "reviewer": "real"
  }
]
EOF

	run bash -c '"$1" --dir "$2" 2>/dev/null' _ "$COMPILE_FINDINGS" "$TEST_DIR"
	[ "$status" -eq 0 ]
	total="$(echo "$output" | jq '.stats.total_findings')"
	[ "$total" -eq 1 ]
	# Only the real reviewer should appear.
	reviewers="$(echo "$output" | jq -r '.stats.reviewers_reporting[]')"
	[ "$reviewers" = "real" ]
}

@test "skips non-array JSON files" {
	cat >"$TEST_DIR/pr-context.json" <<'EOF'
{"pr": {"title": "Test PR", "number": 123}}
EOF
	cat >"$TEST_DIR/real-reviewer.json" <<'EOF'
[
  {
    "severity": "P2",
    "file": "y.go",
    "line": 5,
    "summary": "A finding",
    "evidence": "ev",
    "reviewer": "auditor"
  }
]
EOF

	run bash -c '"$1" --dir "$2" 2>/dev/null' _ "$COMPILE_FINDINGS" "$TEST_DIR"
	[ "$status" -eq 0 ]
	total="$(echo "$output" | jq '.stats.total_findings')"
	[ "$total" -eq 1 ]
	file="$(echo "$output" | jq -r '.findings[0].file')"
	[ "$file" = "y.go" ]
}
