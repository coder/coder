#!/usr/bin/env bash

set -euo pipefail

# Default timeout in seconds (10 minutes)
TIMEOUT=${1:-600}
MAX_ITERATIONS=10

# Log file for Claude output
LOG_FILE="aitest-$(date +%Y%m%d-%H%M%S).log"

# Coverage tracking - use temporary directory
COVERAGE_DIR=$(mktemp -d)
BASELINE_COVERAGE="$COVERAGE_DIR/baseline.out"
CURRENT_COVERAGE="$COVERAGE_DIR/current.out"

# Cleanup function
cleanup() {
	if [[ -d "$COVERAGE_DIR" ]]; then
		rm -rf "$COVERAGE_DIR"
	fi
}

# Set up cleanup on exit
trap cleanup EXIT

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

# Function to get coverage profile
get_coverage() {
	local output_file="$1"
	log "Getting test coverage profile..."
	if ! GOMAXPROCS=4 gotestsum --packages="./..." --rerun-fails=1 -- --coverprofile="$output_file" >/dev/null 2>&1; then
		return 1
	fi
	return 0
}

# Function to extract coverage percentage from profile
get_coverage_percentage() {
	local profile_file="$1"
	if [[ ! -f "$profile_file" ]]; then
		echo "0.0"
		return
	fi

	# Use go tool cover to get coverage percentage
	go tool cover -func="$profile_file" | tail -n 1 | awk '{print $3}' | sed 's/%//'
}

# Function to compare coverage and generate diff if decreased
check_coverage_change() {
	local baseline_file="$1"
	local current_file="$2"

	if [[ ! -f "$baseline_file" ]]; then
		log "No baseline coverage file found, skipping coverage comparison"
		return 0
	fi

	if [[ ! -f "$current_file" ]]; then
		warn "No current coverage file found, skipping coverage comparison"
		return 0
	fi

	local baseline_pct
	baseline_pct=$(get_coverage_percentage "$baseline_file")
	local current_pct
	current_pct=$(get_coverage_percentage "$current_file")

	log "Coverage: baseline ${baseline_pct}%, current ${current_pct}%"

	# Compare coverage (using bc for floating point comparison)
	if command -v bc >/dev/null 2>&1; then
		local decreased
		decreased=$(echo "$current_pct < $baseline_pct" | bc -l)
		if [[ "$decreased" == "1" ]]; then
			warn "Coverage decreased from ${baseline_pct}% to ${current_pct}%"

			# Generate coverage diff
			local diff_output=""
			if command -v go >/dev/null 2>&1; then
				diff_output="Coverage decreased from ${baseline_pct}% to ${current_pct}%

=== BASELINE COVERAGE ===
$(go tool cover -func="$baseline_file")

=== CURRENT COVERAGE ===
$(go tool cover -func="$current_file")

Please fix the code to maintain or improve test coverage."
			else
				diff_output="Coverage decreased from ${baseline_pct}% to ${current_pct}%. Please fix the code to maintain or improve test coverage."
			fi

			echo "$diff_output"
			return 1
		fi
	else
		warn "bc command not available, skipping precise coverage comparison"
	fi

	return 0
}

# Get baseline coverage before starting
log "Getting baseline coverage profile..."
if ! get_coverage "$BASELINE_COVERAGE"; then
	error "Failed to get baseline coverage profile"
	exit 1
fi

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

	log "Running tests..."

	local failed=false
	local output=""

	# Run Go tests if needed
	if [[ "$go_tests" == "true" ]]; then
		log "Running Go tests..."
		if ! go_output=$(make test 2>&1); then
			failed=true
			output+="=== GO TEST FAILURES ===\n$go_output\n\n"
		else
			# Get updated coverage profile after tests pass
			log "Getting updated coverage profile..."
			if get_coverage "$CURRENT_COVERAGE"; then
				# Check for coverage decrease
				if coverage_diff=$(check_coverage_change "$BASELINE_COVERAGE" "$CURRENT_COVERAGE"); then
					log "Coverage maintained or improved"
				else
					failed=true
					output+="=== COVERAGE DECREASED ===\n$coverage_diff\n\n"
				fi
			else
				warn "Failed to get updated coverage profile, but continuing..."
			fi
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
	if ! claude_output=$(echo "$prompt" | claude -p --dangerously-skip-permissions 2>&1); then
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
