#!/usr/bin/env bash
# Prints the outcome of the latest finished doc-check review when that review
# covered exactly HEAD_SHA, and nothing otherwise.
#
# The workflow passes the trace of the latest finished review on stdin. A
# clean or finding outcome means that review read the head, so a run for the
# same head can skip its own review.
#
# Environment:
#   HEAD_SHA   The pull request's live head commit: 40 hex characters.
#
# The outcome goes to stdout and the reason to stderr. The script always exits
# 0, and prints no outcome when the trace does not qualify. Tested by
# doc-check-reviewed-head.test.sh.

set -uo pipefail

log() {
	echo "$*" >&2
}

# Prints the value of the first line that starts with "<key>:" in the trace.
# A value is cut at its closing quote, or at a " #" comment. Prints nothing
# when no such line exists, or when a quote is never closed.
field() {
	printf '%s\n' "${trace}" | awk -v key="$1" '
		$0 ~ ("^[ \t]*" key ":") {
			v = $0
			sub("^[ \t]*" key ":[ \t]*", "", v)
			if (v ~ /^"/) {
				v = match(v, /^"[^"]*"/) ? substr(v, 2, RLENGTH - 2) : ""
			} else if (v ~ /^'\''/) {
				v = match(v, /^'\''[^'\'']*'\''/) ? substr(v, 2, RLENGTH - 2) : ""
			} else {
				sub(/[ \t]+#.*$/, "", v)
				sub(/[ \t]+$/, "", v)
			}
			print v
			exit
		}'
}

main() {
	# A value written on Windows can end in a carriage return.
	local head_sha="${HEAD_SHA:-}"
	head_sha="${head_sha%$'\r'}"
	if [[ ! "${head_sha}" =~ ^[0-9a-fA-F]{40}$ ]]; then
		log "HEAD_SHA is not a full commit SHA. The review runs."
		return 0
	fi
	head_sha="${head_sha,,}"

	local trace trace_sha outcome
	trace=$(tr -d '\r')
	trace_sha=$(field head_sha | tr 'A-F' 'a-f')
	outcome=$(field outcome)

	if [[ -z "${trace_sha}" ]]; then
		log "The trace has no head_sha. The review runs."
		return 0
	fi
	if [[ "${trace_sha}" != "${head_sha}" ]]; then
		log "The latest review covered ${trace_sha}, not ${head_sha}. The review runs."
		return 0
	fi
	case "${outcome}" in
	clean | finding)
		log "The latest finished review covered ${head_sha} and ended with ${outcome}."
		printf '%s\n' "${outcome}"
		;;
	*)
		log "The latest review covered ${head_sha} but its outcome is '${outcome}', not clean or finding. The review runs."
		;;
	esac
	return 0
}

main "$@"
