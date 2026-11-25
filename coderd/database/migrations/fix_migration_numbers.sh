#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")

list_migrations() {
	branch=$1
	git ls-tree -r --name-only "${branch}" -- . | grep -E '[0-9]{6}.*' | sort -n
}

# Lists migrations in the index rather than HEAD, so the script also works in
# the middle of a merge or rebase. Mid-rebase, HEAD does not yet contain the
# branch's migrations, but the index does. uniq drops the extra entries that
# conflicted paths have, one per merge stage.
#
# Mid-rebase, the result is only correct when a single commit adds
# migrations. Later stops cannot tell migrations that earlier stops already
# renumbered from incoming ones, so they can end up sharing a number or
# swapping order. The documented flow runs this script after the rebase.
list_index_migrations() {
	git ls-files -- . | grep -E '[0-9]{6}.*' | sort -n | uniq
}

main() {
	cd "${SCRIPT_DIR}"

	origin=$(git remote -v | grep "github.com[:/]*coder/coder.*(fetch)" | cut -f1)

	echo "Fetching ${origin}/main..."
	git fetch -u "${origin}" main

	curr_num=$(
		set -e
		list_migrations "${origin}"/main | grep '^[0-9]' | tail -n1
	)
	echo "Last migration (main): ${curr_num}"
	next_num=$(("1${curr_num:0:6}" - 1000000 + 1))
	curr_num=$(printf "%06d" "${next_num}")
	echo "Next migration number: ${curr_num}"

	main_files="$(
		set -e
		list_migrations "${origin}"/main
	)"
	head_files="$(
		set -e
		list_index_migrations
	)"

	declare -A prefix_map=()
	declare -a git_add_files=()

	# Renumber migrations part of this branch (as compared to main)
	diff_files="$(diff -u <(echo "${main_files}") <(echo "${head_files}") | sed -E -ne 's;^\+(0.*);\1;p' | sort -n || true)"
	if [[ -z "${diff_files}" ]]; then
		echo "No migrations to rename, exiting."
		return
	fi
	while read -r file; do
		old_file="${file}"
		dir=$(dirname "${file}")
		file=$(basename "${file}")
		num="${file:0:6}"
		set +u
		new_num="${prefix_map["${num}"]}"
		set -u
		if [[ -z "${new_num}" ]]; then
			new_num="${curr_num}"
			prefix_map["${num}"]="${new_num}"
			next_num=$((next_num + 1))
			curr_num=$(printf "%06d" "${next_num}")
		fi
		name="${file:7:-4}"
		new_file="${new_num}_${name}.sql"
		# A stacked PR, or a branch that was already renumbered, can keep its
		# number. It still needs LATEST rewritten below.
		if [[ "${new_file}" == "${old_file}" ]]; then
			echo "Keeping ${old_file}, its number is already free"
			continue
		fi
		echo "Renaming ${old_file} to ${new_file}"
		mv "${old_file}" "${new_file}"
		git_add_files+=("${new_file}" "${old_file}")
	done <<<"${diff_files}"

	# Point LATEST at the newest migration. If merging or rebasing on main left
	# LATEST conflicted, this also resolves it.
	../../../scripts/check_migrations_latest.sh --write
	git_add_files+=("LATEST")

	# Renumber fixtures if there's a matching migration in this branch (as compared to main).
	diff_files="$(diff -u <(echo "${main_files}") <(echo "${head_files}") | sed -E -ne 's;^\+(testdata/[^/]*/0.*);\1;p' | sort -n || true)"
	if [[ -z "${diff_files}" ]]; then
		echo "No testdata fixtures to rename, skipping."
	else
		while read -r file; do
			old_file="${file}"
			dir=$(dirname "${file}")
			file=$(basename "${file}")
			num="${file:0:6}"
			set +u
			new_num="${prefix_map["${num}"]}"
			set -u
			if [[ -z "${new_num}" ]]; then
				echo "Skipping ${old_file}, no matching migration in ${SCRIPT_DIR}"
				continue
			fi
			name="${file:7:-4}"
			new_file="${dir}/${new_num}_${name}.sql"
			if [[ "${new_file}" == "${old_file}" ]]; then
				echo "Keeping ${old_file}, its number is already free"
				continue
			fi
			echo "Renaming ${old_file} to ${new_file}"
			mv "${old_file}" "${new_file}"
			git_add_files+=("${new_file}" "${old_file}")
		done <<<"${diff_files}"
	fi

	# Renumbering maps each old number to one new number, so it cannot split
	# two migrations that already share a number. That happens when the upper
	# PR of a stack merges a lower PR that was renumbered onto its number. Fail
	# before staging rather than report success.
	if ! ../../../scripts/check_migrations_latest.sh; then
		exit 1
	fi

	git add "${git_add_files[@]}"
	git status
	echo "Run 'git commit' to commit the changes."
}

(main "$@")
