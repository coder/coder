#!/usr/bin/env bash

# Summarize failed Playwright tests from the JSON reporter output.

set -euo pipefail
# shellcheck source=scripts/lib.sh
# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

if [[ $# -ne 1 ]]; then
	error "Usage: playwright-failure-summary.sh <results.json>"
fi

results_file=$1
if [[ ! -f "$results_file" ]]; then
	exit 0
fi

if ! command -v jq >/dev/null; then
	error "jq is required to summarize Playwright failures."
fi

artifact="playwright-artifacts-${MATRIX_VARIANT:-unknown}-${GITHUB_SHA_SHORT:-unknown}"

jq -r --arg artifact "$artifact" --arg root "$PROJECT_ROOT" '
	def clean_block:
		tostring
		| gsub("\u001b\\[[0-9;]*[A-Za-z]"; "")
		| gsub("```"; "``");
	def clean_inline:
		tostring | gsub("`"; "");
	def truncate($max):
		if length > $max then .[0:$max] + "..." else . end;
	def failure_status:
		. == "failed" or . == "timedOut" or . == "interrupted";
	def relpath($root):
		if startswith($root + "/") then .[($root | length) + 1:]
		elif startswith("site/") then .
		elif startswith("e2e/") then "site/" + .
		else "site/e2e/" + .
		end;
	def all_specs($titles):
		([$titles[], (.title // empty)] | map(select(. != ""))) as $next_titles
		| (
			.specs[]?
			| . + {
				titlePath: ($next_titles + ([.title // ""] | map(select(. != ""))))
			}
		),
		(.suites[]? | all_specs($next_titles));
	def failure_entries:
		[
			.suites[]?
			| all_specs([]) as $spec
			| $spec.tests[]? as $test
			| select(($test.status // "") != "flaky")
			| select(
				(($test.status // "") == "unexpected")
				or any($test.results[]?; .status | failure_status)
			)
			| ([ $test.results[]? | select(.status | failure_status) ][0]
				// ($test.results[0] // {})) as $result
			| ((($result.error.message // "") | clean_block) as $message
				| (($result.error.stack // "") | clean_block) as $stack
				| {
					file: (($spec.file // "") | relpath($root)),
					line: ($spec.line // 0),
					title: (($spec.titlePath // [$spec.title // ""]) | join(" > ") | clean_inline),
					project: (($test.projectName // "unknown") | clean_inline),
					message: (if $message != "" then $message else $stack end | if . != "" then . else "No error message recorded." end | truncate(600)),
					attachments: ([ $result.attachments[]? | .name // empty | clean_inline ] | unique)
				})
		];
	failure_entries as $entries
	| if ($entries | length) == 0 then
		empty
	else
		(.stats // {}) as $stats
		| ($stats.unexpected // 0) as $stats_failed
		| ([($stats_failed | tonumber), ($entries | length)] | max) as $failed
		| (($stats.expected // 0) + ($stats.unexpected // 0) + ($stats.flaky // 0) + ($stats.skipped // 0)) as $computed_total
		| ($stats.total // $computed_total) as $total
		| [
			"## Playwright failures (\($failed) of \($total))",
			"- Duration: \($stats.duration // 0)ms",
			"- Skipped: \($stats.skipped // 0), Flaky: \($stats.flaky // 0)",
			"- Artifact: `\($artifact)` (download from the run summary)",
			"",
			($entries[]
				| "### \(.file):\(.line)\n"
				+ "- Test: `\(.title)`\n"
				+ "- Project: `\(.project)`\n"
				+ "- Attachments:\n"
				+ (if (.attachments | length) == 0 then
					"  - None recorded in artifact `\($artifact)`"
				else
					(.attachments | map("  - `\(.)` in artifact `\($artifact)`") | join("\n"))
				end)
				+ "\n\n```\n\(.message)\n```\n")
		]
		| join("\n")
	end
' "$results_file" | sed -E $'s/\x1b\[[0-9;]*m//g'
