#!/usr/bin/env bash
#
# release_experimental.sh — Interactive release tagging script for coder/coder.
#
# Usage: ./scripts/release_experimental.sh
#
# Run this from a release branch (release/X.Y). The script will:
#   1. Detect the release branch and infer the next version from existing tags.
#   2. Let you override the version if the suggestion isn't what you want.
#   3. Warn (but not block) on semver violations.
#   4. Check for open PRs against the release branch.
#   5. Generate and preview release notes.
#   6. Tag, push, and trigger the release workflow — with explicit
#      confirmation before every mutating action.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

# --- Helpers ---------------------------------------------------------------

dependencies git gh jq

# Bold/color helpers (degrade gracefully if not a terminal).
if [[ -t 1 ]]; then
	BOLD='\033[1m'
	YELLOW='\033[1;33m'
	GREEN='\033[1;32m'
	CYAN='\033[0;36m'
	RESET='\033[0m'
else
	BOLD='' YELLOW='' GREEN='' CYAN='' RESET=''
fi

warn() {
	echo -e "${YELLOW}⚠️  WARNING: $*${RESET}" >&2
}

info() {
	echo -e "${CYAN}$*${RESET}" >&2
}

success() {
	echo -e "${GREEN}✓ $*${RESET}" >&2
}

confirm() {
	local prompt="$1"
	local reply
	while true; do
		read -r -p "$prompt (y/n) " reply
		case "$reply" in
		[Yy]) return 0 ;;
		[Nn]) return 1 ;;
		*) echo "Please answer y or n." ;;
		esac
	done
}

# Parse a version string into major, minor, patch. Sets globals.
parse_version() {
	local v="${1#v}"
	IFS='.' read -r VERSION_MAJOR VERSION_MINOR VERSION_PATCH <<<"$v"
}

# Compare two semver strings. Returns 0 if $1 > $2.
semver_gt() {
	local a_maj a_min a_pat b_maj b_min b_pat
	IFS='.' read -r a_maj a_min a_pat <<<"${1#v}"
	IFS='.' read -r b_maj b_min b_pat <<<"${2#v}"
	if ((a_maj > b_maj)); then return 0; fi
	if ((a_maj == b_maj && a_min > b_min)); then return 0; fi
	if ((a_maj == b_maj && a_min == b_min && a_pat > b_pat)); then return 0; fi
	return 1
}

semver_eq() {
	[[ "${1#v}" == "${2#v}" ]]
}

# --- Argument Parsing ------------------------------------------------------

while [[ $# -gt 0 ]]; do
	case "$1" in
	-h | --help)
		sed -n '2,/^$/s/^# \?//p' "${BASH_SOURCE[0]}"
		exit 0
		;;
	*)
		error "Unknown argument: $1. This script takes no arguments — just run it from a release branch."
		;;
	esac
done

# --- GitHub Auth -----------------------------------------------------------

if [[ -z ${GITHUB_TOKEN:-} ]]; then
	if [[ -n ${GH_TOKEN:-} ]]; then
		export GITHUB_TOKEN=${GH_TOKEN}
	elif token="$(gh auth token --hostname github.com 2>/dev/null)"; then
		export GITHUB_TOKEN=${token}
	else
		error "GitHub authentication required. Set GITHUB_TOKEN or run 'gh auth login'."
	fi
fi

# --- Current Release Landscape ---------------------------------------------

info "Checking current releases..."

# Latest mainline: highest semver tag across all tags.
latest_mainline="$(git tag --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -1 || true)"

# Latest stable: second-highest minor version series.
# e.g. if mainline is v2.21.x, stable is the highest v2.20.x tag.
latest_stable="(unknown)"
if [[ -n "$latest_mainline" ]]; then
	parse_version "$latest_mainline"
	mainline_major=$VERSION_MAJOR
	mainline_minor=$VERSION_MINOR
	stable_minor=$((mainline_minor - 1))
	latest_stable="$(git tag --sort=-v:refname | grep -E "^v${mainline_major}\.${stable_minor}\.[0-9]+$" | head -1 || true)"
	if [[ -z "$latest_stable" ]]; then
		latest_stable="(none found for v${mainline_major}.${stable_minor}.x)"
	fi
fi

log
echo -e "${BOLD}  Latest mainline release: ${latest_mainline:-"(none)"}${RESET}"
echo -e "${BOLD}  Latest stable release:   ${latest_stable}${RESET}"
log

# --- Branch Detection ------------------------------------------------------

current_branch="$(git branch --show-current)"

if ! [[ "$current_branch" =~ ^release/([0-9]+)\.([0-9]+)$ ]]; then
	error "You must be on a release branch (release/X.Y). Current branch: '${current_branch}'."
fi

branch_major="${BASH_REMATCH[1]}"
branch_minor="${BASH_REMATCH[2]}"
success "On release branch: ${current_branch}"

# Make sure branch is up to date.
info "Fetching latest from origin..."
git fetch --quiet --tags origin "$current_branch"

local_head="$(git rev-parse HEAD)"
remote_head="$(git rev-parse "origin/${current_branch}" 2>/dev/null || echo "")"

if [[ -n "$remote_head" ]] && [[ "$local_head" != "$remote_head" ]]; then
	warn "Your local branch is not up to date with origin/${current_branch}."
	log "  Local:  ${local_head:0:12}"
	log "  Remote: ${remote_head:0:12}"
	if ! confirm "Continue anyway?"; then
		exit 1
	fi
	log
fi

# --- Find Previous Version & Suggest Next ----------------------------------

# Find the most recent vX.Y.Z tag reachable from HEAD.
prev_version="$(git tag --merged HEAD --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -1 || true)"

if [[ -z "$prev_version" ]]; then
	info "No previous release tag found on this branch."
	suggested_version="v${branch_major}.${branch_minor}.0"
else
	info "Previous release tag: ${prev_version}"
	parse_version "$prev_version"
	suggested_version="v${VERSION_MAJOR}.${VERSION_MINOR}.$((VERSION_PATCH + 1))"
fi

log
echo -e "${BOLD}Suggested next version: ${suggested_version}${RESET}"
read -r -p "Version to release [${suggested_version}]: " version_input
new_version="${version_input:-$suggested_version}"

# Validate version format.
if ! [[ "$new_version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	error "Version must be in the format vMAJOR.MINOR.PATCH (e.g. v2.31.1). Got: $new_version"
fi

parse_version "$new_version"
new_major=$VERSION_MAJOR
new_minor=$VERSION_MINOR
new_patch=$VERSION_PATCH

# Warn if the version doesn't match the branch.
if ((new_major != branch_major || new_minor != branch_minor)); then
	warn "Version ${new_version} does not match branch ${current_branch} (expected v${branch_major}.${branch_minor}.X)."
	if ! confirm "Continue anyway?"; then
		exit 1
	fi
	log
fi

log
info "=== Coder Release: ${new_version} ==="
log

# --- Check If Tag Already Exists -------------------------------------------

if git tag -l "$new_version" | grep -q .; then
	warn "Tag '${new_version}' already exists!"
	if ! confirm "This will skip tagging. Continue?"; then
		exit 1
	fi
	tag_exists=1
	log
else
	tag_exists=0
fi

# --- Semver Sanity Checks --------------------------------------------------

if [[ -n "$prev_version" ]]; then
	parse_version "$prev_version"
	old_major=$VERSION_MAJOR
	old_minor=$VERSION_MINOR
	old_patch=$VERSION_PATCH

	# Check for downgrade.
	if semver_gt "$prev_version" "$new_version"; then
		warn "Version DOWNGRADE detected: ${prev_version} → ${new_version}."
		warn "The new version is lower than the previous tag."
		if ! confirm "Continue?"; then
			exit 1
		fi
		log
	fi

	# Check for duplicate.
	if semver_eq "$prev_version" "$new_version"; then
		warn "Version ${new_version} is the SAME as the previous tag ${prev_version}."
		if ! confirm "Continue?"; then
			exit 1
		fi
		log
	fi

	# Check for skipped patch versions.
	if ((new_major == old_major && new_minor == old_minor)); then
		expected_patch=$((old_patch + 1))
		if ((new_patch > expected_patch)); then
			warn "Skipping patch version(s): expected v${new_major}.${new_minor}.${expected_patch}, got ${new_version}."
			if ! confirm "Continue?"; then
				exit 1
			fi
			log
		fi
	fi

	# Check for breaking changes in a patch release.
	if ((new_major == old_major && new_minor == old_minor && new_patch > old_patch)); then
		info "Checking for breaking changes in patch release..."
		breaking_commits=()
		while IFS= read -r line; do
			[[ -z "$line" ]] && continue
			sha="${line%% *}"
			title="${line#* }"
			# Check conventional commit "!:" pattern.
			if [[ "$title" =~ ^[a-zA-Z]+(\(.+\))?!: ]]; then
				breaking_commits+=("$sha $title")
			fi
		done < <(git log --no-merges --pretty=format:'%h %s' "${prev_version}..HEAD" 2>/dev/null || true)

		# Also check PR labels for release/breaking.
		breaking_prs=""
		if [[ -n "$prev_version" ]]; then
			breaking_prs="$(gh pr list \
				--base "$current_branch" \
				--state merged \
				--label "release/breaking" \
				--json number,title \
				--jq '.[] | "#\(.number) \(.title)"' 2>/dev/null || true)"
		fi

		if ((${#breaking_commits[@]} > 0)) || [[ -n "$breaking_prs" ]]; then
			echo >&2
			warn "BREAKING CHANGES detected in a PATCH release — this violates semver!"
			echo >&2
			if ((${#breaking_commits[@]} > 0)); then
				log "  Breaking commits (by conventional commit prefix):"
				for c in "${breaking_commits[@]}"; do
					log "    - $c"
				done
			fi
			if [[ -n "$breaking_prs" ]]; then
				log "  PRs labeled release/breaking:"
				while IFS= read -r pr; do
					log "    - $pr"
				done <<<"$breaking_prs"
			fi
			echo >&2
			if ! confirm "Continue with patch release despite breaking changes?"; then
				exit 1
			fi
			log
		else
			success "No breaking changes detected."
		fi
	fi
fi

# --- Check Open PRs -------------------------------------------------------

info "Checking for open PRs against ${current_branch}..."
open_prs="$(gh pr list \
	--base "$current_branch" \
	--state open \
	--json number,title,author \
	--jq '.[] | "#\(.number) \(.title) (@\(.author.login))"' 2>/dev/null || true)"

if [[ -n "$open_prs" ]]; then
	echo >&2
	warn "There are open PRs targeting ${current_branch} that may need merging first:"
	echo >&2
	while IFS= read -r pr; do
		log "  ${pr}"
	done <<<"$open_prs"
	echo >&2
	if ! confirm "Continue without merging these?"; then
		exit 1
	fi
	log
else
	success "No open PRs against ${current_branch}."
fi
log

# --- Generate Release Notes ------------------------------------------------

info "Generating release notes..."

commit_range="${prev_version}..HEAD"
if [[ -z "$prev_version" ]]; then
	commit_range="HEAD"
fi

# Collect commits grouped by category.
declare -A section_commits
section_order=("breaking" "security" "feat" "fix" "docs" "refactor" "other")
declare -A section_titles=(
	["breaking"]="⚠️ BREAKING CHANGES"
	["security"]="🔒 Security"
	["feat"]="✨ Features"
	["fix"]="🐛 Bug Fixes"
	["docs"]="📖 Documentation"
	["refactor"]="♻️ Refactor"
	["other"]="📦 Other Changes"
)

# Initialize.
for s in "${section_order[@]}"; do
	section_commits[$s]=""
done

# Build a lookup of PR numbers to labels (batch query).
declare -A pr_labels_map
if [[ -n "$prev_version" ]]; then
	while IFS=$'\t' read -r pr_num pr_labels; do
		pr_labels_map[$pr_num]="$pr_labels"
	done < <(gh pr list \
		--base "$current_branch" \
		--state merged \
		--limit 500 \
		--json number,labels \
		--jq '.[] | "\(.number)\t\([.labels[].name] | join(","))"' 2>/dev/null || true)
fi

while IFS= read -r line; do
	[[ -z "$line" ]] && continue
	sha="${line%% *}"
	rest="${line#* }"
	title="$rest"

	# Extract PR number from title or commit body.
	pr_num=""
	if [[ "$title" =~ \(#([0-9]+)\) ]]; then
		pr_num="${BASH_REMATCH[1]}"
	fi

	# Determine category.
	category="other"
	labels="${pr_labels_map[$pr_num]:-}"

	# Label-based categorization takes priority.
	if [[ "$labels" == *"release/breaking"* ]] || [[ "$title" =~ ^[a-zA-Z]+(\(.+\))?!: ]]; then
		category="breaking"
	elif [[ "$labels" == *"security"* ]]; then
		category="security"
	elif [[ "$title" =~ ^feat ]]; then
		category="feat"
	elif [[ "$title" =~ ^fix ]]; then
		category="fix"
	elif [[ "$title" =~ ^docs ]]; then
		category="docs"
	elif [[ "$title" =~ ^refactor ]]; then
		category="refactor"
	fi

	# Format the line.
	entry="- ${title} (${sha})"
	section_commits[$category]+="${entry}"$'\n'
done < <(git log --no-merges --pretty=format:'%h %s' "$commit_range" 2>/dev/null || true)

# Build the release notes markdown.
release_notes="## ${new_version}"$'\n\n'

if [[ -n "$prev_version" ]]; then
	release_notes+="Compare: https://github.com/coder/coder/compare/${prev_version}...${new_version}"$'\n\n'
fi

has_content=0
for s in "${section_order[@]}"; do
	if [[ -n "${section_commits[$s]:-}" ]]; then
		release_notes+="### ${section_titles[$s]}"$'\n\n'
		release_notes+="${section_commits[$s]}"$'\n'
		has_content=1
	fi
done

if ((has_content == 0)); then
	release_notes+="_No changes since ${prev_version:-the beginning of time}._"$'\n'
fi

# Write to file.
mkdir -p build
release_notes_file="build/RELEASE-${new_version}.md"
echo -e "$release_notes" >"$release_notes_file"

# --- Preview Release Notes -------------------------------------------------

log
echo -e "${BOLD}--- Release Notes Preview ---${RESET}"
log
echo -e "$release_notes"
echo -e "${BOLD}--- End Preview ---${RESET}"
log
info "Release notes written to ${release_notes_file}"
log

# Offer to edit.
editor="${EDITOR:-${GIT_EDITOR:-}}"
if [[ -n "$editor" ]]; then
	if confirm "Edit release notes in ${editor}?"; then
		"$editor" "$release_notes_file"
		release_notes="$(<"$release_notes_file")"
		log "Release notes updated."
	fi
	log
fi

# --- Release Channel -------------------------------------------------------

# Suggest stable if this version's minor is the stable minor.
if ((new_minor == stable_minor)); then
	stable_default="Y/n"
	stable_hint=" (this looks like a stable release)"
else
	stable_default="y/N"
	stable_hint=""
fi

reply=""
while true; do
	read -r -p "Mark this as the latest stable release on GitHub?${stable_hint} (${stable_default}) " reply
	case "$reply" in
	[Yy]) channel="stable" && break ;;
	[Nn]) channel="mainline" && break ;;
	"")
		# Enter = use the suggested default.
		if ((new_minor == stable_minor)); then
			channel="stable"
		else
			channel="mainline"
		fi
		break
		;;
	*) echo "Please answer y or n." ;;
	esac
done

if [[ "$channel" == "stable" ]]; then
	info "Channel: stable (will be marked as GitHub Latest)."
else
	info "Channel: mainline (will be marked as prerelease)."
fi
log

# --- Tag -------------------------------------------------------------------

ref="$(git rev-parse HEAD)"
short_ref="${ref:0:12}"

if ((tag_exists == 0)); then
	echo -e "${BOLD}Next step: create an annotated tag.${RESET}"
	log "  Tag:    ${new_version}"
	log "  Commit: ${short_ref}"
	log "  Branch: ${current_branch}"
	log
	if confirm "Create tag?"; then
		git tag -a "$new_version" -m "Release $new_version" "$ref"
		success "Tag ${new_version} created."
	else
		error "Cannot proceed without a tag. Aborting."
	fi
	log
else
	info "Tag ${new_version} already exists, skipping creation."
	log
fi

# --- Push Tag --------------------------------------------------------------

echo -e "${BOLD}Next step: push tag '${new_version}' to origin.${RESET}"
log "  This will run: git push origin ${new_version}"
log
if confirm "Push tag?"; then
	git push origin "$new_version"
	success "Tag pushed."
else
	error "Cannot trigger release without pushing the tag. Aborting."
fi
log

# --- Trigger Release Workflow ----------------------------------------------

release_json=$(jq -n \
	--arg release_channel "$channel" \
	--arg release_notes "$release_notes" \
	'{dry_run: "false", release_channel: $release_channel, release_notes: $release_notes}')

echo -e "${BOLD}Next step: trigger the 'release.yaml' GitHub Actions workflow.${RESET}"
log "  Ref:     ${new_version}"
log "  Channel: ${channel}"
log "  Payload:"
log "    release_channel: ${channel}"
log "    release_notes:   (${#release_notes} chars, written to ${release_notes_file})"
log
if confirm "Trigger release workflow?"; then
	echo "$release_json" | gh workflow run release.yaml --json --ref "${new_version}"
	success "Release workflow triggered!"
else
	log "Skipped workflow trigger. You can trigger it manually:"
	log "  echo '...' | gh workflow run release.yaml --json --ref ${new_version}"
fi
log

log
success "Done! 🎉"
log
info "Follow the release workflow: https://github.com/coder/coder/actions/workflows/release.yaml"
