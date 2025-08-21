#!/usr/bin/env bash

set -euo pipefail

# Default timeout in seconds (10 minutes)
TIMEOUT=${1:-600}
MAX_ITERATIONS=10

# Log file for Claude output
LOG_FILE="aitest-$(date +%Y%m%d-%H%M%S).log"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() {
	echo -e "${BLUE}[aitest]${NC} $1" >&2
}

error() {
	echo -e "${RED}[aitest ERROR]${NC} $1" >&2
}

success() {
	echo -e "${GREEN}[aitest SUCCESS]${NC} $1" >&2
}

warn() {
	echo -e "${YELLOW}[aitest WARN]${NC} $1" >&2
}

# Check if claude command is available
if ! command -v claude >/dev/null 2>&1; then
	error "claude command not found. Please install Claude CLI."
	exit 1
fi

# Read prompt from stdin
log "Reading prompt from stdin..."
prompt=$(cat)

if [[ -z "$prompt" ]]; then
	error "No prompt provided via stdin"
	exit 1
fi

log "Received prompt (${#prompt} characters)"
log "Logging Claude output to: $LOG_FILE"

# Initialize log file with header
{
	echo "=== AI Test Session Started at $(date) ==="
	echo "Initial prompt:"
	echo "$prompt"
	echo ""
} >"$LOG_FILE"

# Function to detect if changes affect Go or TypeScript
detect_test_type() {
	local changed_files
	# Get recently changed files (last 5 minutes or uncommitted changes)
	if git status --porcelain | grep -q .; then
		# Use uncommitted changes
		changed_files=$(git status --porcelain | awk '{print $2}')
	else
		# Use recently changed files
		changed_files=$(git diff --name-only HEAD~1 2>/dev/null || echo "")
	fi

	local needs_go=false
	local needs_ts=false

	while IFS= read -r file; do
		if [[ "$file" == *.go ]]; then
			needs_go=true
		elif [[ "$file" == site/* && ("$file" == *.ts || "$file" == *.tsx || "$file" == *.js || "$file" == *.jsx) ]]; then
			needs_ts=true
		fi
	done <<<"$changed_files"

	echo "go:$needs_go ts:$needs_ts"
}

# Function to run tests and lint
run_tests() {
	local test_type="$1"
	local go_tests="${test_type#*:}"
	go_tests="${go_tests%% *}"
	local ts_tests="${test_type#* }"
	ts_tests="${ts_tests#*:}"

	log "Running tests (Go: $go_tests, TS: $ts_tests)..."

	local failed=false
	local output=""

	# Run Go tests if needed
	if [[ "$go_tests" == "true" ]]; then
		log "Running Go tests..."
		if ! go_output=$(make test 2>&1); then
			failed=true
			output+="=== GO TEST FAILURES ===\n$go_output\n\n"
		fi
	fi

	# Run TypeScript tests if needed
	if [[ "$ts_tests" == "true" ]]; then
		log "Running TypeScript tests..."
		if ! ts_output=$(cd site && pnpm test 2>&1); then
			failed=true
			output+="=== TYPESCRIPT TEST FAILURES ===\n$ts_output\n\n"
		fi
	fi

	# Always run lint
	log "Running lint..."
	if ! lint_output=$(make lint 2>&1); then
		failed=true
		output+="=== LINT FAILURES ===\n$lint_output\n\n"
	fi

	if [[ "$failed" == "true" ]]; then
		echo "$output"
		return 1
	else
		return 0
	fi
}

# Main loop
log "Starting test-fix cycle (timeout: ${TIMEOUT}s, max iterations: $MAX_ITERATIONS)"
start_time=$(date +%s)
iteration=0

while ((iteration < MAX_ITERATIONS)); do
	iteration=$((iteration + 1))
	current_time=$(date +%s)
	elapsed=$((current_time - start_time))

	if ((elapsed >= TIMEOUT)); then
		error "Timeout reached after ${elapsed}s"
		echo "=== FAILURE: Timeout reached after ${elapsed}s at $(date) ===" >>"$LOG_FILE"
		exit 1
	fi

	log "Iteration $iteration (elapsed: ${elapsed}s)"

	# Send prompt to Claude
	log "Sending prompt to Claude..."

	# Log the prompt for this iteration
	{
		echo "=== Iteration $iteration at $(date) ==="
		echo "Prompt:"
		echo "$prompt"
		echo ""
		echo "Claude response:"
	} >>"$LOG_FILE"

	# Send to Claude and capture output
	if ! claude_output=$(echo "$prompt" | claude -p 2>&1); then
		error "Claude command failed"
		echo "Claude command failed: $claude_output" >>"$LOG_FILE"
		exit 1
	fi

	# Log Claude's response
	echo "$claude_output" >>"$LOG_FILE"
	echo "" >>"$LOG_FILE"

	# Check if Claude made any changes
	if ! git status --porcelain | grep -q .; then
		# No changes detected, check if this is the first iteration
		if [[ $iteration -gt 1 ]]; then
			error "No file changes detected after Claude's response. Bailing out."
			echo "=== FAILURE: No changes made by Claude in iteration $iteration at $(date) ===" >>"$LOG_FILE"
			exit 1
		fi
		warn "No changes detected, but this is the first iteration. Continuing..."
	fi

	# Detect what tests to run
	test_type=$(detect_test_type)
	log "Detected test requirements: $test_type"

	# Run tests
	if test_output=$(run_tests "$test_type"); then
		success "All tests passed! Completed in $iteration iteration(s)"
		echo "=== SUCCESS: All tests passed in $iteration iteration(s) at $(date) ===" >>"$LOG_FILE"
		exit 0
	else
		warn "Tests failed, providing feedback to Claude..."

		# Log the test failures
		{
			echo "Test failures:"
			echo "$test_output"
			echo ""
		} >>"$LOG_FILE"

		# Update prompt to include only the test failures
		prompt="Fix these test failures:

$test_output"

		log "Updated prompt with test failures for next iteration"
	fi
done

error "Maximum iterations ($MAX_ITERATIONS) reached without success"
echo "=== FAILURE: Maximum iterations reached without success at $(date) ===" >>"$LOG_FILE"
exit 1
