#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")

list_migrations() {
	branch=$1
	git ls-tree -r --name-only "${branch}" -- . | grep -E '[0-9]{6}.*' | sort -n
}

# list_migration_names lists migration file names without their directory.
# Migrations are compared by name because older ones live in archive
# directories, and a migration that only moved into one must not be mistaken
# for a migration added by this branch.
list_migration_names() {
	list_migrations "$1" | grep -v '^testdata/' | sed 's;.*/;;' | sort -n
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
		list_migrations HEAD
	)"

	declare -A prefix_map=()
	declare -a git_add_files=()

	# Renumber migrations part of this branch (as compared to main)
	declare -A main_names=()
	while read -r name; do
		[[ -n "${name}" ]] && main_names["${name}"]=1
	done <<<"$(list_migration_names "${origin}"/main)"

	diff_files=""
	while read -r file; do
		[[ -z "${file}" || "${file}" == testdata/* ]] && continue
		set +u
		known="${main_names["${file##*/}"]}"
		set -u
		[[ -n "${known}" ]] && continue
		diff_files+="${file}"$'\n'
	done <<<"${head_files}"
	diff_files="$(echo "${diff_files}" | sed -E '/^$/d' | sort -n || true)"
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
		echo "Renaming ${old_file} to ${new_file}"
		mv "${old_file}" "${new_file}"
		git_add_files+=("${new_file}" "${old_file}")
	done <<<"${diff_files}"

	# Renumber fixtures if there's a matching migration in this branch (as compared to main).
	diff_files="$(diff -u <(echo "${main_files}") <(echo "${head_files}") | sed -E -ne 's;^\+(testdata/[^/]*/0.*);\1;p' | sort -n || true)"
	if [[ -z "${diff_files}" ]]; then
		echo "No testdata fixtures to rename, skipping."
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
			echo "Skipping ${old_file}, no matching migration in ${SCRIPT_DIR}"
			continue
		fi
		name="${file:7:-4}"
		new_file="${dir}/${new_num}_${name}.sql"
		echo "Renaming ${old_file} to ${new_file}"
		mv "${old_file}" "${new_file}"
		git_add_files+=("${new_file}" "${old_file}")
	done <<<"${diff_files}"

	git add "${git_add_files[@]}"
	git status
	echo "Run 'git commit' to commit the changes."
}

(main "$@")
