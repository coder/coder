#!/usr/bin/env bash
# Journey for features/users.md: the admin creates a password user in the web
# UI, then the API, the CLI and a fresh sign-in prove the user exists and works.
# Usage: create-user.sh   (prints one line per check and a count line)
set -euo pipefail
scripts="$(dirname "${BASH_SOURCE[0]}")/../scripts"
# shellcheck source=.agents/skills/verify-coder/scripts/lib.sh
source "$scripts/lib.sh"
vc_load_run
"$scripts/doctor.sh" >/dev/null || vc_fail "doctor failed, run $scripts/doctor.sh"

out="$VC_EVIDENCE/$(date -u +%Y%m%dT%H%M%SZ)-create-user"
mkdir -p "$out"
username="vc-$(date +%s)"
email="$username@coder.com"
password="Verify-$(openssl rand -hex 8)"

VC_CHECK_LOG="$out/steps.log"
VC_RESULT_FILE="$out/result.txt"
vc_on_fail() {
	vc_capture_failure "$out" "$1"
}

sign_in() {
	vc_browser_login
	vc_ab screenshot "$out/01-workspaces.png" >/dev/null
	echo "signed in as $VC_ADMIN_EMAIL, title $(vc_ab get title)"
}

fill_form() {
	vc_ab open "$VC_WEB_URL/deployment/users" >/dev/null
	vc_ab wait --fn 'document.title === "Users - Coder"' >/dev/null
	vc_ab screenshot "$out/02-users-before.png" >/dev/null
	vc_ab find role link click --name "New user" >/dev/null
	vc_ab wait --fn 'document.title === "New user - Coder"' >/dev/null
	local login_type
	login_type="$(vc_ab get text '[data-testid=login-type-input]')"
	if [[ "$login_type" != *Password* ]]; then
		vc_ab click '[data-testid=login-type-input]' >/dev/null
		vc_ab find role option click --name "Password" >/dev/null
	fi
	vc_ab find label "Username" fill "$username" >/dev/null
	vc_ab fill 'input[name=name]' "Verify $username" >/dev/null
	vc_ab find label "Email" fill "$email" >/dev/null
	vc_ab screenshot "$out/03-new-user-form.png" >/dev/null
	vc_ab fill 'input[name=password]' "$password" >/dev/null
	echo "filled the form for $username (login type was '$login_type')"
}

submit_form() {
	vc_ab find role button click --name "Save" >/dev/null
	vc_ab wait --text "created successfully" >/dev/null
	vc_ab wait --fn 'document.title === "Users - Coder"' >/dev/null
	vc_ab wait --text "$email" >/dev/null
	vc_ab screenshot "$out/04-users-after-save.png" >/dev/null
	# The search box stores its query in ?filter=, so this is the list a user
	# sees after searching for the new user, with the row above the fold.
	vc_ab open "$VC_WEB_URL/deployment/users?filter=$username" >/dev/null
	vc_ab wait --text "$email" >/dev/null
	vc_ab screenshot "$out/05-users-search.png" >/dev/null
	vc_ab snapshot >"$out/05-users-search.snapshot.txt"
	echo "toast shown and $email listed on /deployment/users"
}

api_has_user() {
	vc_api "/api/v2/users?q=$username" |
		jq '[.users[] | {username, email, login_type, status, roles: [.roles[].name]}]' >"$out/api-user.json"
	jq -e --arg u "$username" --arg e "$email" \
		'length == 1 and .[0].username == $u and .[0].email == $e and .[0].login_type == "password"' \
		"$out/api-user.json" >/dev/null
	echo "GET /api/v2/users?q=$username returns 1 password user ($(jq -r '.[0].status' "$out/api-user.json"))"
}

cli_lists_user() {
	"$scripts/coder.sh" users list --output json | jq --arg u "$username" '[.[] | select(.username == $u) | {username, email}]' >"$out/cli-users.json"
	jq -e 'length == 1' "$out/cli-users.json" >/dev/null
	echo "coder users list includes $username"
}

new_user_can_sign_in() {
	local code
	code="$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
		-d "$(jq -n --arg e "$email" --arg p "$password" '{email: $e, password: $p}')" \
		"$VC_API_URL/api/v2/users/login")"
	[[ "$code" == 201 ]] || {
		echo "POST /api/v2/users/login answered $code"
		return 1
	}
	echo "POST /api/v2/users/login as $email answered 201"
}

vc_check sign-in sign_in
vc_check fill-form fill_form
vc_check submit submit_form
vc_check api api_has_user
vc_check cli cli_lists_user
vc_check new-user-login new_user_can_sign_in
echo "create-user: $VC_PASS/$VC_TOTAL checks passed, evidence in $out" | tee -a "$out/result.txt"
((VC_PASS == VC_TOTAL))
