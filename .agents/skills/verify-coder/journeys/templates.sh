#!/usr/bin/env bash
# Journey for features/templates.md: the admin sees the seeded docker template
# in the UI and the CLI, creates a template from the docker starter in the UI,
# pulls, edits and pushes a new version with the CLI, and deletes it.
# Usage: templates.sh   (prints one line per check and a count line)
set -euo pipefail
scripts="$(dirname "${BASH_SOURCE[0]}")/../scripts"
# shellcheck source=.agents/skills/verify-coder/scripts/lib.sh
source "$scripts/lib.sh"
vc_load_run
"$scripts/doctor.sh" >/dev/null || vc_fail "doctor failed, run $scripts/doctor.sh"
# develop.sh seeds the docker template only when Docker works, and the doctor
# passes without it.
docker info >/dev/null 2>&1 || vc_fail "docker info failed, this journey needs a local Docker daemon and the seeded docker template"

mkdir -p "$VC_EVIDENCE"
# mktemp keeps two runs that start in the same second apart.
out="$(mktemp -d "$VC_EVIDENCE/$(date -u +%Y%m%dT%H%M%SZ)-templates-XXXX")"
chmod 755 "$out"
VC_CHECK_LOG="$out/steps.log"
VC_RESULT_FILE="$out/result.txt"
vc_on_fail() {
	vc_capture_failure "$out" "$1"
}
# The random suffix keeps two runs that start in the same second apart.
name="vct-$(date +%s)-$RANDOM"
pulled="$out/pulled-template"

template_json() {
	vc_api /api/v2/templates | jq --arg n "$1" '.[] | select(.name == $n)'
}

list() {
	vc_browser_login "$VC_ADMIN_EMAIL" "$VC_ADMIN_PASSWORD"
	vc_ab open "$VC_WEB_URL/templates" >/dev/null
	vc_ab wait --fn 'document.title === "Templates - Coder"' >/dev/null
	vc_ab wait 'a[href="/templates/docker/workspace"]' >/dev/null
	vc_ab screenshot "$out/01-ui-templates.png" >/dev/null
	vc_coder templates list --output json 2>/dev/null | jq -r '.[].Template.name' >"$out/01-cli-templates.txt"
	grep -qx docker "$out/01-cli-templates.txt" || {
		echo "coder templates list has no docker template"
		return 1
	}
	echo "the UI links /templates/docker/workspace, coder templates list has docker"
}

starter() {
	vc_ab open "$VC_WEB_URL/starter-templates/docker" >/dev/null
	vc_ab wait --fn 'document.title === "Docker Containers - Coder"' >/dev/null
	vc_ab find role link click --name "Use template" >/dev/null
	vc_ab wait --url "**/templates/new?exampleId=docker" >/dev/null
	vc_ab wait 'input[name=name]' >/dev/null
	vc_ab fill 'input[name=name]' "$name" >/dev/null
	vc_ab screenshot "$out/02-ui-create-form.png" >/dev/null
	vc_ab find role button click --name "Save" >/dev/null
	vc_ab wait --url "**/templates/$name/files" >/dev/null
	vc_ab screenshot "$out/03-ui-created.png" >/dev/null
	template_json "$name" | jq '{name, active_version_id}' >"$out/03-api-template.json"
	jq -e --arg n "$name" '.name == $n' "$out/03-api-template.json" >/dev/null
	echo "Use template and Save created $name and opened /templates/$name/files"
}

push() {
	local before after message
	before="$(jq -r '.active_version_id' "$out/03-api-template.json")"
	vc_coder templates pull "$name" "$pulled" >"$out/04-cli-pull.txt" 2>&1
	echo "# verify-coder push $name" >>"$pulled/main.tf"
	vc_coder templates push "$name" --directory "$pulled" --message "verify-coder push" --yes </dev/null >"$out/05-cli-push.txt" 2>&1
	after="$(template_json "$name" | jq -r '.active_version_id')"
	[[ -n "$after" && "$after" != "$before" ]] || {
		echo "the active version did not change"
		return 1
	}
	message="$(vc_api "/api/v2/templateversions/$after" | jq -r '.message')"
	[[ "$message" == "verify-coder push" ]] || {
		echo "the active version message is '$message'"
		return 1
	}
	vc_coder templates versions list "$name" >"$out/06-cli-versions.txt" 2>/dev/null
	echo "pull, edit and push made a new active version with message 'verify-coder push'"
}

delete() {
	local listed
	vc_coder templates delete "$name" --yes >"$out/07-cli-delete.txt" 2>&1
	# A plain assignment keeps errexit, so a failed read fails the check.
	listed="$(vc_api /api/v2/templates | jq --arg n "$name" 'any(.[]; .name == $n)')"
	[[ "$listed" == false ]] || {
		echo "GET /api/v2/templates still lists $name"
		return 1
	}
	echo "coder templates delete removed $name"
}

# A failed or interrupted run still deletes the template it made, unless the
# delete check passed.
deleted=0
on_exit() {
	if ((!deleted)); then
		vc_coder templates delete "$name" --yes >>"$VC_CHECK_LOG" 2>&1 || true
	fi
}
trap on_exit EXIT
trap 'exit 130' INT TERM

vc_check list list
vc_check starter starter
vc_check push push
passed_before_delete=$VC_PASS
vc_check delete delete
if ((VC_PASS > passed_before_delete)); then
	deleted=1
fi
echo "templates: $VC_PASS/$VC_TOTAL checks passed, evidence in $out" | tee -a "$out/result.txt"
((VC_PASS == VC_TOTAL))
