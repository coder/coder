#!/bin/bash
set -euo pipefail

[[ $VERBOSE == 1 ]] && set -x

status=$1
shift

case "${status}" in
started) ;;
completed) ;;
failed) ;;
*)
	echo "Unknown status: ${status}" >&2
	exit 1
	;;
esac

# shellcheck disable=SC2153 source=scaletest/templates/scaletest-runner/scripts/lib.sh
. "${SCRIPTS_DIR}/lib.sh"

# NOTE(mafredri): API returns HTML if we accidentally use `...//api` vs `.../api`.
# https://github.com/coder/coder/issues/9877
CODER_URL="${CODER_URL%/}"
buildinfo="$(curl -sSL "${CODER_URL}/api/v2/buildinfo")"
server_version="$(jq -r '.version' <<<"${buildinfo}")"
server_version_commit="$(jq -r '.external_url' <<<"${buildinfo}")"

# Since `coder show` doesn't support JSON output, we list the workspaces instead.
# Use `command` here to bypass dry run.
workspace_json="$(
	command coder list --all --output json |
		jq --arg workspace "${CODER_WORKSPACE}" --arg user "${CODER_USER}" 'map(select(.name == $workspace) | select(.owner_name == $user)) | .[0]'
)"
owner_name="$(jq -r '.latest_build.workspace_owner_name' <<<"${workspace_json}")"
workspace_name="$(jq -r '.latest_build.workspace_name' <<<"${workspace_json}")"
initiator_name="$(jq -r '.latest_build.initiator_name' <<<"${workspace_json}")"

bullet='•'
app_urls_raw="$(jq -r '.latest_build.resources[].agents[]?.apps | map(select(.external == true)) | .[] | .display_name, .url' <<<"${workspace_json}")"
app_urls=()
while read -r app_name; do
	read -r app_url
	bold=
	if [[ ${status} != started ]] && [[ ${app_url} = *to=now* ]]; then
		# Update Grafana URL with end stamp and make bold.
		app_url="${app_url//to=now/to=$(($(date +%s) * 1000))}"
		bold='*'
	fi
	app_urls+=("${bullet} ${bold}${app_name}${bold}: ${app_url}")
done <<<"${app_urls_raw}"

params=()
header=

case "${status}" in
started)
	created_at="$(jq -r '.latest_build.created_at' <<<"${workspace_json}")"
	params=("${bullet} Options:")
	while read -r param; do
		params+=("    ${bullet} ${param}")
	done <<<"$(jq -r '.latest_build.resources[].agents[]?.environment_variables | to_entries | map(select(.key | startswith("SCALETEST_PARAM_"))) | .[] | "`\(.key)`: `\(.value)`"' <<<"${workspace_json}")"

	header="New scaletest started at \`${created_at}\` by \`${initiator_name}\` on ${CODER_URL} (<${server_version_commit}|\`${server_version}\`>)."
	;;
completed)
	completed_at=$(date -Iseconds)
	header="Scaletest completed at \`${completed_at}\` (started by \`${initiator_name}\`) on ${CODER_URL} (<${server_version_commit}|\`${server_version}\`>)."
	;;
failed)
	failed_at=$(date -Iseconds)
	header="Scaletest failed at \`${failed_at}\` (started by \`${initiator_name}\`) on ${CODER_URL} (<${server_version_commit}|\`${server_version}\`>)."
	;;
*)
	echo "Unknown status: ${status}" >&2
	exit 1
	;;
esac

text_arr=(
	"${header}"
	""
	"${bullet} *Comment:* ${SCALETEST_COMMENT}"
	"${bullet} Workspace (runner): ${CODER_URL}/@${owner_name}/${workspace_name}"
	"${bullet} Run ID: ${SCALETEST_RUN_ID}"
	"${app_urls[@]}"
	"${params[@]}"
)

text=
for field in "${text_arr[@]}"; do
	text+="${field}"$'\n'
done

json=$(
	jq -n --arg text "${text}" '{
		blocks: [
			{
				"type": "section",
				"text": {
					"type": "mrkdwn",
					"text": $text
				}
			}
		]
	}'
)

maybedryrun "${DRY_RUN}" curl -X POST -H 'Content-type: application/json' --data "${json}" "${SLACK_WEBHOOK_URL}"
