#!/usr/bin/env bash
# Reports whether a merged Dependabot PR fixed a Dependabot alert, so
# backport.yaml can label it for backport.
#
# A PR fixes an alert when, for one of its updated dependencies:
#   - the alert is for the same package,
#   - the alert's manifest is one of the files the PR changed, and
#   - the update crossed the alert's first patched version: the old version is
#     below it and the new version is at or above it.
#
# Open alerts count too, because GitHub may not have closed the alert yet when
# the PR merges. Comparing against the first patched version avoids parsing
# vulnerable version ranges. Requiring the old version to be below it skips
# alerts that were already fixed before the PR.
#
# Inputs (environment):
#   PR_NUMBER  the merged pull request
#   REPO       owner/name, defaults to GITHUB_REPOSITORY
#   GH_TOKEN   token that can read pull requests and Dependabot alerts
#
# Writes matched=true|false and alerts=<comma-separated alert numbers> to
# GITHUB_OUTPUT when it is set. Tests: dependabot-alert-match.test.sh.

set -euo pipefail

: "${PR_NUMBER:?PR_NUMBER is required}"
REPO="${REPO:-${GITHUB_REPOSITORY:?REPO or GITHUB_REPOSITORY is required}}"

# version_le A B: succeeds when version A <= version B. A leading "v" is
# ignored, because Go alerts and Dependabot commit messages disagree on it.
version_le() {
	local a="${1#v}" b="${2#v}"
	printf '%s\n%s\n' "${a}" "${b}" | sort -V -C
}

# version_lt A B: succeeds when version A < B.
version_lt() {
	[[ "${1#v}" != "${2#v}" ]] && version_le "$1" "$2"
}

# Dependabot's commit lists every updated dependency in a YAML trailer with
# the new version:
#   - dependency-name: dompurify
#     dependency-version: 3.4.17
# The old version is only in the body, as "Bumps [name](url) from A to B." or,
# in grouped PRs, "Updates `name` from A to B".
message=$(gh api "repos/${REPO}/pulls/${PR_NUMBER}/commits" --jq '.[0].commit.message')
deps=$(awk '
	{ gsub(/["\047]/, "") }
	/^- dependency-name: / { name = $3 }
	/^  dependency-version: / && name != "" { print name "\t" $2; name = "" }
' <<<"${message}")

olds=$(awk '
	/^Bumps \[/ {
		name = $0; sub(/^Bumps \[/, "", name); sub(/\].*/, "", name)
		if (match($0, /\) from [^ ]+ to /)) { split(substr($0, RSTART + 7), v, " "); print name "\t" v[1] }
	}
	/^Updates `/ {
		name = $0; sub(/^Updates `/, "", name); sub(/`.*/, "", name)
		if (match($0, /` from [^ ]+ to /)) { split(substr($0, RSTART + 7), v, " "); print name "\t" v[1] }
	}
' <<<"${message}")

if [[ -z "${deps}" ]]; then
	echo "No updated-dependencies metadata in PR #${PR_NUMBER}."
fi

files=$(gh api --paginate "repos/${REPO}/pulls/${PR_NUMBER}/files" --jq '.[].filename')

matched=()
while IFS=$'\t' read -r name version; do
	[[ -z "${name}" ]] && continue
	old=$(awk -F'\t' -v n="${name}" '$1 == n { print $2; exit }' <<<"${olds}")
	if [[ -z "${old}" ]]; then
		echo "::warning::No old version for ${name} in PR #${PR_NUMBER}; matching on the new version only."
	fi
	while IFS=$'\t' read -r number manifest patched; do
		[[ -z "${number}" || -z "${patched}" ]] && continue
		grep -qxF -- "${manifest}" <<<"${files}" || continue
		version_le "${patched}" "${version}" || continue
		if [[ -n "${old}" ]] && ! version_lt "${old}" "${patched}"; then
			continue
		fi
		echo "Alert #${number}: ${name} ${old:-?} to ${version} fixes ${manifest} (patched in ${patched})."
		matched+=("${number}")
	done < <(
		gh api --paginate -X GET "repos/${REPO}/dependabot/alerts" \
			-f state=open,fixed -f package="${name}" -f per_page=100 \
			--jq '.[] | [.number, .dependency.manifest_path, (.security_vulnerability.first_patched_version.identifier // "")] | @tsv'
	)
done <<<"${deps}"

result=false
if ((${#matched[@]} > 0)); then
	result=true
fi
alerts=$(
	IFS=,
	echo "${matched[*]:-}"
)
echo "matched=${result} alerts=${alerts}"

if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
	{
		echo "matched=${result}"
		echo "alerts=${alerts}"
	} >>"${GITHUB_OUTPUT}"
fi
