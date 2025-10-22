#!/usr/bin/env bash

# Run rbac authz benchmark tests on the current Git branch or compare benchmark results
# between two branches using `benchstat`.
#
# The script supports:
# 1) Running benchmarks and saving output to a file.
# 2) Checking out two branches, running benchmarks on each, and saving the `benchstat`
# comparison results to a file.
# Benchmark results are saved with filenames based on the branch name.
#
# Usage:
#   benchmark_authz.sh --single                       # Run benchmarks on current branch
#   benchmark_authz.sh --compare <branchA> <branchB>  # Compare benchmarks between two branches

set -euo pipefail

# Go benchmark parameters
GOMAXPROCS=16
TIMEOUT=30m
BENCHTIME=5s
COUNT=5

# Script configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="${SCRIPT_DIR}/benchmark_outputs"

# List of benchmark tests
BENCHMARKS=(
	BenchmarkRBACAuthorize
	BenchmarkRBACAuthorizeGroups
	BenchmarkRBACFilter
)

# Create output directory
mkdir -p "$OUTPUT_DIR"

function run_benchmarks() {
	local branch=$1
	# Replace '/' with '-' for branch names with format user/branchName
	local filename_branch=${branch//\//-}
	local output_file_prefix="$OUTPUT_DIR/${filename_branch}"

	echo "Checking out $branch..."
	git checkout "$branch"

	# Move into the rbac directory to run the benchmark tests
	pushd ../../coderd/rbac/ >/dev/null

	for bench in "${BENCHMARKS[@]}"; do
		local output_file="${output_file_prefix}_${bench}.txt"
		echo "Running benchmark $bench on $branch..."
		GOMAXPROCS=$GOMAXPROCS go test -timeout $TIMEOUT -bench="^${bench}$" -run=^$ -benchtime=$BENCHTIME -count=$COUNT | tee "$output_file"
	done

	# Return to original directory
	popd >/dev/null
}

if [[ $# -eq 0 || "${1:-}" == "--single" ]]; then
	current_branch=$(git rev-parse --abbrev-ref HEAD)
	run_benchmarks "$current_branch"
elif [[ "${1:-}" == "--compare" ]]; then
	base_branch=$2
	test_branch=$3

	# Run all benchmarks on both branches
	run_benchmarks "$base_branch"
	run_benchmarks "$test_branch"

	# Compare results benchmark by benchmark
	for bench in "${BENCHMARKS[@]}"; do
		# Replace / with - for branch names with format user/branchName
		filename_base_branch=${base_branch//\//-}
		filename_test_branch=${test_branch//\//-}

		echo -e "\nGenerating benchmark diff for $bench using benchstat..."
		benchstat "$OUTPUT_DIR/${filename_base_branch}_${bench}.txt" "$OUTPUT_DIR/${filename_test_branch}_${bench}.txt" | tee "$OUTPUT_DIR/${bench}_diff.txt"
	done
else
	echo "Usage:"
	echo "  $0 --single                   # run benchmarks on current branch"
	echo "  $0 --compare branchA branchB  # compare benchmarks between two branches"
	exit 1
fi
