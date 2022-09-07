#!/usr/bin/env bash

# This script is meant to be sourced by other scripts. To source this script:
#     # shellcheck source=scripts/lib.sh
#     source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

set -euo pipefail

# realpath returns an absolute path to the given relative path. It will fail if
# the parent directory of the path does not exist. Make sure you are in the
# expected directory before running this to avoid errors.
#
# GNU realpath relies on coreutils, which are not installed or the default on
# Macs out of the box, so we have this mostly working bash alternative instead.
#
# Taken from https://stackoverflow.com/a/3915420 (CC-BY-SA 4.0)
realpath() {
	local dir
	local base
	dir="$(dirname "$1")"
	base="$(basename "$1")"

	if [[ ! -d "$dir" ]]; then
		error "Could not change directory to '$dir': directory does not exist"
	fi
	echo "$(
		cd "$dir" || error "Could not change directory to '$dir'"
		pwd -P
	)"/"$base"
}

# We have to define realpath before these otherwise it fails on Mac's bash.
SCRIPT_DIR="$(realpath "$(dirname "${BASH_SOURCE[0]}")")"
PROJECT_ROOT="$(cd "$SCRIPT_DIR" && realpath "$(git rev-parse --show-toplevel)")"

# pushd is a silent alternative to the real pushd shell command.
pushd() {
	command pushd "$@" >/dev/null || error "Could not pushd to '$*'"
}

# popd is a silent alternative to the real popd shell command.
# shellcheck disable=SC2120
popd() {
	command popd >/dev/null || error "Could not restore directory with popd"
}

# cdself changes directory to the directory of the current script. This should
# not be used in scripts that may be sourced by other scripts.
cdself() {
	cd "$SCRIPT_DIR" || error "Could not change directory to '$SCRIPT_DIR'"
}

# cdroot changes directory to the root of the repository.
cdroot() {
	cd "$PROJECT_ROOT" || error "Could not change directory to '$PROJECT_ROOT'"
}

# execrelative can be used to execute scripts as if you were in the parent
# directory of the current script. This should not be used in scripts that may
# be sourced by other scripts.
execrelative() {
	pushd "$SCRIPT_DIR" || error "Could not change directory to '$SCRIPT_DIR'"
	local rc=0
	"$@" || rc=$?
	popd
	return $rc
}

dependencies() {
	local fail=0
	for dep in "$@"; do
		if ! command -v "$dep" >/dev/null; then
			log "ERROR: The '$dep' dependency is required, but is not available."
			fail=1
		fi
	done

	if [[ "$fail" == 1 ]]; then
		log
		error "One or more dependencies are not available, check above log output for more details."
	fi
}

requiredenvs() {
	local fail=0
	for env in "$@"; do
		if [[ "${!env:-}" == "" ]]; then
			log "ERROR: The '$env' environment variable is required, but is not set."
			fail=1
		fi
	done

	if [[ "$fail" == 1 ]]; then
		log
		error "One or more required environment variables are not set, check above log output for more details."
	fi
}

# maybedryrun prints the given program and flags, and then, if the first
# argument is 0, executes it. The reason the first argument should be 0 is that
# it is expected that you have a dry_run variable in your script that is set to
# 0 by default (i.e. do not dry run) and set to 1 if the --dry-run flag is
# specified.
#
# Usage: maybedryrun 1 gh release create ...
# Usage: maybedryrun 0 docker push ghcr.io/coder/coder:latest
maybedryrun() {
	if [[ "$1" == 1 ]]; then
		shift
		log "DRYRUN: $*"
	else
		shift
		log $ "$@"
		"$@"
	fi
}

# log prints a message to stderr.
log() {
	echo "$*" 1>&2
}

# error prints an error message and returns an error exit code.
error() {
	log "ERROR: $*"
	exit 1
}

# isdarwin returns an error if the current platform is not darwin.
isdarwin() {
	[[ "${OSTYPE:-darwin}" == *darwin* ]]
}

# We don't need to check dependencies more than once per script, but some
# scripts call other scripts that also `source lib.sh`, so we set an environment
# variable after successfully checking dependencies once.
if [[ "${CODER_LIBSH_NO_CHECK_DEPENDENCIES:-}" != *t* ]]; then
	libsh_bad_dependencies=0

	if ((BASH_VERSINFO[0] < 4)); then
		libsh_bad_dependencies=1
		log "ERROR: You need at least bash 4.0 to run the scripts in the Coder repo."
		if isdarwin; then
			log "On darwin:"
			log "- brew install bash"
			log "- Restart your terminal"
		fi
		log
	fi

	# BSD getopt (which is installed by default on Macs) is not supported.
	if [[ "$(getopt --version)" == *--* ]]; then
		libsh_bad_dependencies=1
		log "ERROR: You need GNU getopt to run the scripts in the Coder repo."
		if isdarwin; then
			log "On darwin:"
			log "- brew install gnu-getopt"
			# shellcheck disable=SC2016
			log '- Add "$(brew --prefix)/opt/gnu-getopt/bin" to your PATH'
			log "- Restart your terminal"
		fi
		log
	fi

	# The bash scripts don't call Make directly, but we want to make (ha ha)
	# sure that make supports the features the repo uses. Notably, Macs have an
	# old version of Make installed out of the box that doesn't support new
	# features like ONESHELL.
	#
	# We have to disable pipefail temporarily to avoid ERRPIPE errors when
	# piping into `head -n1`.
	set +o pipefail
	make_version="$(make --version 2>/dev/null | head -n1 | grep -oE '([[:digit:]]+\.){1,2}[[:digit:]]+')"
	set -o pipefail
	if [[ ${make_version//.*/} -lt 4 ]]; then
		libsh_bad_dependencies=1
		log "ERROR: You need at least make 4.0 to run the scripts in the Coder repo."
		if isdarwin; then
			log "On darwin:"
			log "- brew install make"
			# shellcheck disable=SC2016
			log '- Add "$(brew --prefix)/opt/make/libexec/gnubin" to your PATH (you should Google this first)'
			log "- Restart your terminal"
		fi
		log
	fi

	if [[ "$libsh_bad_dependencies" == 1 ]]; then
		error "Invalid dependencies, see above for more details."
	fi

	export CODER_LIBSH_NO_CHECK_DEPENDENCIES=true
fi
