#!/usr/bin/env bash

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

usage() {
	cat <<EOH
Usage: ./release.sh [--dry-run] [-h | --help] [--ref <ref>] [--major | --minor | --patch] [--force]

This script should be called to create a new release.

When run, this script will display the new version number and optionally a
preview of the release notes. The new version will be selected automatically
based on if the release contains breaking changes or not. If the release
contains breaking changes, a new minor version will be created. Otherwise, a
new patch version will be created.

To mark a release as containing breaking changes, the commit title should
either contain a known prefix with an exclamation mark ("feat!:",
"feat(api)!:") or the PR that was merged can be tagged with the
"release/breaking" label.

GitHub labels that affect release notes:

- release/breaking: Shown under BREAKING CHANGES, prevents patch release.
- release/experimental: Shown at the bottom under Experimental.
- security: Shown under SECURITY.

Flags:

Set --major or --minor to force a larger version bump, even when there are no
breaking changes. By default a patch version will be created, --patch is no-op.

Set --force force the provided increment to be used (e.g. --patch), even if
there are breaking changes, etc.

Set --ref if you need to specify a specific commit that the new version will
be tagged at, otherwise the latest commit will be used.

Set --dry-run to see what this script would do without making actual changes.
EOH
}

branch=main
remote=origin
dry_run=0
ref=
increment=
force=0
script_check=1
mainline=1
channel=mainline

# These values will be used for any PRs created.
pr_review_assignee=${CODER_RELEASE_PR_REVIEW_ASSIGNEE:-@me}
pr_review_reviewer=${CODER_RELEASE_PR_REVIEW_REVIEWER:-bpmct,stirby}

args="$(getopt -o h -l dry-run,help,ref:,mainline,stable,major,minor,patch,force,ignore-script-out-of-date -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--dry-run)
		dry_run=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	--mainline)
		mainline=1
		channel=mainline
		shift
		;;
	--stable)
		mainline=0
		channel=stable
		shift
		;;
	--ref)
		ref="$2"
		shift 2
		;;
	--major | --minor | --patch)
		if [[ -n $increment ]]; then
			error "Cannot specify multiple version increments."
		fi
		increment=${1#--}
		shift
		;;
	--force)
		force=1
		shift
		;;
	# Allow the script to be run with an out-of-date script for
	# development purposes.
	--ignore-script-out-of-date)
		script_check=0
		shift
		;;
	--)
		shift
		break
		;;
	*)
		error "Unrecognized option: $1"
		;;
	esac
done

# Check dependencies.
dependencies gh jq sort

# Authenticate gh CLI
gh_auth

if [[ -z $increment ]]; then
	# Default to patch versions.
	increment="patch"
fi

# Check if the working directory is clean.
if ! git diff --quiet --exit-code; then
	log "Working directory is not clean, it is highly recommended to stash changes."
	while [[ ! ${stash:-} =~ ^[YyNn]$ ]]; do
		read -p "Stash changes? (y/n) " -n 1 -r stash
		log
	done
	if [[ ${stash} =~ ^[Yy]$ ]]; then
		maybedryrun "${dry_run}" git stash push --message "scripts/release.sh: autostash"
	fi
	log
fi

# Check if the main is up-to-date with the remote.
log "Checking remote ${remote} for repo..."
remote_url=$(git remote get-url "${remote}")
# Allow either SSH or HTTPS URLs.
if ! [[ ${remote_url} =~ [@/]github.com ]] && ! [[ ${remote_url} =~ [:/]coder/coder(\.git)?$ ]]; then
	error "This script is only intended to be run with github.com/coder/coder repository set as ${remote}."
fi

# Make sure the repository is up-to-date before generating release notes.
log "Fetching ${branch} and tags from ${remote}..."
git fetch --quiet --tags "${remote}" "$branch"

# Resolve to the current commit unless otherwise specified.
ref_name=${ref:-HEAD}
ref=$(git rev-parse "${ref_name}")

# Make sure that we're running the latest release script.
script_diff=$(git diff --name-status "${remote}/${branch}" -- scripts/release.sh)
if [[ ${script_check} = 1 ]] && [[ -n ${script_diff} ]]; then
	error "Release script is out-of-date. Please check out the latest version and try again."
fi

# Make sure no other remote release contains this ref.
release_contains_ref="$(git branch --remotes --contains "${ref}" --list "${remote}/release/*" --format='%(refname)')"
if [[ -n ${release_contains_ref} ]]; then
	error "Ref ${ref_name} is already part of another release: $(git describe --always "${ref}") on ${release_contains_ref#"refs/remotes/${remote}/"}."
fi

log "Checking GitHub for latest release(s)..."

# Check the latest version tag from GitHub (by version) using the API.
versions_out="$(gh api -H "Accept: application/vnd.github+json" /repos/coder/coder/git/refs/tags -q '.[].ref | split("/") | .[2]' | grep '^v[0-9]' | sort -r -V)"
mapfile -t versions <<<"${versions_out}"
latest_mainline_version=${versions[0]}

latest_stable_version="$(curl -fsSLI -o /dev/null -w "%{url_effective}" https://github.com/coder/coder/releases/latest)"
latest_stable_version="${latest_stable_version#https://github.com/coder/coder/releases/tag/}"

log "Latest mainline release: ${latest_mainline_version}"
log "Latest stable release: ${latest_stable_version}"
log

old_version=${latest_mainline_version}
if ((!mainline)); then
	old_version=${latest_stable_version}
fi

trap 'log "Check commit metadata failed, you can try to set \"export CODER_IGNORE_MISSING_COMMIT_METADATA=1\" and try again, if you know what you are doing."' EXIT
# shellcheck source=scripts/release/check_commit_metadata.sh
source "$SCRIPT_DIR/release/check_commit_metadata.sh" "$old_version" "$ref"
trap - EXIT
log

tag_version_args=(--old-version "$old_version" --ref "$ref_name" --"$increment")
if ((force == 1)); then
	tag_version_args+=(--force)
fi
log "Executing DRYRUN of release tagging..."
tag_version_out="$(execrelative ./release/tag_version.sh "${tag_version_args[@]}" --dry-run)"
log
while [[ ! ${continue_release:-} =~ ^[YyNn]$ ]]; do
	read -p "Continue? (y/n) " -n 1 -r continue_release
	log
done
if ! [[ $continue_release =~ ^[Yy]$ ]]; then
	exit 0
fi
log

mapfile -d ' ' -t tag_version <<<"$tag_version_out"
release_branch=${tag_version[0]}
new_version=${tag_version[1]}
new_version="${new_version%$'\n'}" # Remove the trailing newline.

release_notes="$(execrelative ./release/generate_release_notes.sh --old-version "$old_version" --new-version "$new_version" --ref "$ref")"

mkdir -p build
release_notes_file="build/RELEASE-${new_version}.md"
if ((dry_run)); then
	release_notes_file="build/RELEASE-${new_version}-DRYRUN.md"
fi
get_editor() {
	if command -v editor >/dev/null; then
		readlink -f "$(command -v editor || true)"
	elif [[ -n ${GIT_EDITOR:-} ]]; then
		echo "${GIT_EDITOR}"
	elif [[ -n ${EDITOR:-} ]]; then
		echo "${EDITOR}"
	fi
}
editor="$(get_editor)"
write_release_notes() {
	if [[ -z ${editor} ]]; then
		log "Release notes written to $release_notes_file, you can now edit this file manually."
	else
		log "Release notes written to $release_notes_file, you can now edit this file manually or via your editor."
	fi
	echo -e "${release_notes}" >"${release_notes_file}"
}
log "Writing release notes to ${release_notes_file}"
if [[ -f ${release_notes_file} ]]; then
	log
	while [[ ! ${overwrite:-} =~ ^[YyNn]$ ]]; do
		read -p "Release notes already exists, overwrite? (y/n) " -n 1 -r overwrite
		log
	done
	log
	if [[ ${overwrite} =~ ^[Yy]$ ]]; then
		write_release_notes
	else
		log "Release notes not overwritten, using existing release notes."
		release_notes="$(<"$release_notes_file")"
	fi
else
	write_release_notes
fi
log

if [[ -z ${editor} ]]; then
	log "No editor found, please set the \$EDITOR environment variable for edit prompt."
else
	while [[ ! ${edit:-} =~ ^[YyNn]$ ]]; do
		read -p "Edit release notes in \"${editor}\"? (y/n) " -n 1 -r edit
		log
	done
	if [[ ${edit} =~ ^[Yy]$ ]]; then
		"${editor}" "${release_notes_file}"
		release_notes2="$(<"$release_notes_file")"
		if [[ "${release_notes}" != "${release_notes2}" ]]; then
			log "Release notes have been updated!"
			release_notes="${release_notes2}"
		else
			log "No changes detected..."
		fi
	fi
fi
log

while [[ ! ${preview:-} =~ ^[YyNn]$ ]]; do
	read -p "Preview release notes? (y/n) " -n 1 -r preview
	log
done
if [[ ${preview} =~ ^[Yy]$ ]]; then
	log
	echo -e "$release_notes\n"
fi
log

while [[ ! ${create:-} =~ ^[YyNn]$ ]]; do
	read -p "Create, build and publish release? (y/n) " -n 1 -r create
	log
done
if ! [[ ${create} =~ ^[Yy]$ ]]; then
	exit 0
fi
log

# Run without dry-run to actually create the tag, note we don't update the
# new_version variable here to ensure we're pushing what we showed before.
maybedryrun "$dry_run" execrelative ./release/tag_version.sh "${tag_version_args[@]}" >/dev/null
maybedryrun "$dry_run" git push -u origin "$release_branch"
maybedryrun "$dry_run" git push --tags -u origin "$new_version"

log
log "Release tags for ${new_version} created successfully and pushed to ${remote}!"

log
# Write to a tmp file for ease of debugging.
release_json_file=$(mktemp -t coder-release.json.XXXXXX)
log "Writing release JSON to ${release_json_file}"
jq -n \
	--argjson dry_run "${dry_run}" \
	--arg release_channel "${channel}" \
	--arg release_notes "${release_notes}" \
	'{dry_run: ($dry_run > 0) | tostring, release_channel: $release_channel, release_notes: $release_notes}' \
	>"${release_json_file}"

log "Running release workflow..."
maybedryrun "${dry_run}" cat "${release_json_file}" |
	maybedryrun "${dry_run}" gh workflow run release.yaml --json --ref "${new_version}"

log
log "Release workflow started successfully!"

log
log "Would you like for me to create a pull request for you to automatically bump the version numbers in the docs?"
while [[ ! ${create_pr:-} =~ ^[YyNn]$ ]]; do
	read -p "Create PR? (y/n) " -n 1 -r create_pr
	log
done
if [[ ${create_pr} =~ ^[Yy]$ ]]; then
	pr_branch=autoversion/${new_version}
	title="docs: bump ${channel} version to ${new_version}"
	body="This PR was automatically created by the [release script](https://github.com/coder/coder/blob/main/scripts/release.sh).

Please review the changes and merge if they look good and the release is complete.

You can follow the release progress [here](https://github.com/coder/coder/actions/workflows/release.yaml) and view the published release [here](https://github.com/coder/coder/releases/tag/${new_version}) (once complete)."

	log
	log "Creating branch \"${pr_branch}\" and updating versions..."

	create_pr_stash=0
	if ! git diff --quiet --exit-code -- docs; then
		maybedryrun "${dry_run}" git stash push --message "scripts/release.sh: autostash (autoversion)" -- docs
		create_pr_stash=1
	fi
	maybedryrun "${dry_run}" git checkout -b "${pr_branch}" "${remote}/${branch}"
	execrelative go run ./release autoversion --channel "${channel}" "${new_version}" --dry-run
	maybedryrun "${dry_run}" git add docs
	maybedryrun "${dry_run}" git commit -m "${title}"
	# Return to previous branch.
	maybedryrun "${dry_run}" git checkout -
	if ((create_pr_stash)); then
		maybedryrun "${dry_run}" git stash pop
	fi

	log "Creating pull request..."
	maybedryrun "${dry_run}" gh pr create \
		--assignee "${pr_review_assignee}" \
		--reviewer "${pr_review_reviewer}" \
		--base "${branch}" \
		--head "${pr_branch}" \
		--title "${title}" \
		--body "${body}"
fi

if ((dry_run)); then
	# We can't watch the release.yaml workflow if we're in dry-run mode.
	exit 0
fi

log
while [[ ! ${watch:-} =~ ^[YyNn]$ ]]; do
	read -p "Watch release? (y/n) " -n 1 -r watch
	log
done
if ! [[ ${watch} =~ ^[Yy]$ ]]; then
	exit 0
fi

log 'Waiting for job to become "in_progress"...'

# Wait at most 10 minutes (60*10/60) for the job to start.
for _ in $(seq 1 60); do
	output="$(
		# Output:
		# 3886828508
		# in_progress
		gh run list -w release.yaml \
			--limit 1 \
			--json status,databaseId \
			--jq '.[] | (.databaseId | tostring), .status'
	)"
	mapfile -t run <<<"$output"
	if [[ ${run[1]} != "in_progress" ]]; then
		sleep 10
		continue
	fi
	gh run watch --exit-status "${run[0]}"
	exit 0
done

error "Waiting for job to start timed out."
