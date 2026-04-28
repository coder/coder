#!/usr/bin/env bash

# Summarize failed Go tests from go test JSON output.

set -euo pipefail
# shellcheck source=scripts/lib.sh
# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

if [[ $# -ne 1 ]]; then
	error "Usage: go-test-failure-summary.sh <go-test.json>"
fi

results_file=$1
if [[ ! -s "$results_file" ]]; then
	exit 0
fi

if ! command -v jq >/dev/null; then
	error "jq is required to summarize Go test failures."
fi

jq -sr '
	def clean_block:
		tostring
		| gsub("\u001b\\[[0-9;?]*[ -/]*[@-~]"; "")
		| gsub("```"; "``");
	def clean_inline:
		tostring | gsub("`"; "") | gsub("[\r\n]"; " ");
	def truncate($max):
		if length > $max then .[0:$max] + "..." else . end;
	def terminal_action:
		.Action == "pass" or .Action == "fail" or .Action == "skip";
	def test_key:
		(.Package // "") + "\u0000" + (.Test // "");
	def output_for($events; $package; $test):
		[
			$events[]
			| select(.Action == "output")
			| select((.Package // "") == $package)
			| select((.Test // "") == $test)
			| .Output // ""
		]
		| join("")
		| clean_block
		| if . == "" then "No output recorded." else . end
		| truncate(600);

	map(select(type == "object")) as $events
	| [
		$events
		| to_entries[]
		| .value + {idx: .key}
		| select((.Test // "") != "")
		| select(terminal_action)
	] as $terminal_tests
	| [
		$terminal_tests
		| group_by(test_key)
		| .[]
		| max_by(.idx)
		| select(.Action == "fail")
		| {
			package: ((.Package // "unknown") | clean_inline),
			test: ((.Test // "unknown") | clean_inline),
			elapsed: (.Elapsed // 0),
			output: output_for($events; (.Package // ""); (.Test // ""))
		}
	] as $failures
	| if ($failures | length) == 0 then
		empty
	else
		($failures | length) as $failed
		| ($failures | map(.package) | unique | length) as $packages
		| ([
			$events[]
			| select((.Test // "") == "")
			| select(.Action == "pass" or .Action == "fail")
			| .Elapsed // 0
		] | add // 0) as $duration
		| ([
			$events[]
			| select((.Test // "") == "")
			| select(.Action == "fail")
			| .Package // empty
		] | unique | length) as $package_failures
		| [
			"## Go test failures (\($failed) in \($packages))",
			"- Duration: \($duration)s",
			"- Package failures: \($package_failures)",
			"",
			($failures[]
				| "### \(.package) :: \(.test)\n"
				+ "- Elapsed: \(.elapsed)s\n\n"
				+ "```\n\(.output)\n```\n")
		]
		| join("\n")
	end
' "$results_file"
