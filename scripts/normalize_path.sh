#!/bin/bash

# Call: normalize_path_with_symlinks [target_dir] [dir_prefix]
#
# Normalizes the PATH environment variable by replacing each directory that
# begins with dir_prefix with a symbolic link in target_dir. For example, if
# PATH is "/usr/bin:/bin", target_dir is /tmp, and dir_prefix is /usr, then
# PATH will become "/tmp/0:/bin", where /tmp/0 links to /usr/bin.
#
# This is useful for ensuring that PATH is consistent across CI runs and helps
# with reusing the same cache across them. Many of our go tests read the PATH
# variable, and if it changes between runs, the cache gets invalidated.
normalize_path_with_symlinks() {
	local target_dir="${1:-}"
	local dir_prefix="${2:-}"

	if [[ -z "$target_dir" || -z "$dir_prefix" ]]; then
		echo "Usage: normalize_path_with_symlinks <target_dir> <dir_prefix>"
		return 1
	fi

	local old_path="$PATH"
	local -a new_parts=()
	local i=0

	IFS=':' read -ra _parts <<<"$old_path"
	for dir in "${_parts[@]}"; do
		# Skip empty components that can arise from "::"
		[[ -z $dir ]] && continue

		# Skip directories that don't start with $dir_prefix
		if [[ "$dir" != "$dir_prefix"* ]]; then
			new_parts+=("$dir")
			continue
		fi

		local link="$target_dir/$i"

		# Replace any pre-existing file or link at $target_dir/$i
		if [[ -e $link || -L $link ]]; then
			rm -rf -- "$link"
		fi

		# without MSYS ln will deepcopy the directory on Windows
		MSYS=winsymlinks:nativestrict ln -s -- "$dir" "$link"
		new_parts+=("$link")
		i=$((i + 1))
	done

	export PATH
	PATH="$(
		IFS=':'
		echo "${new_parts[*]}"
	)"
}
