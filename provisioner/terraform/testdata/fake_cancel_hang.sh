#!/usr/bin/env sh

VERSION=$1
shift 1

json_print() {
	echo "{\"@level\":\"error\",\"@message\":\"$*\"}"
}

case "$1" in
version)
	cat <<-EOF
		{
			"terraform_version": "${VERSION}",
			"platform": "linux_amd64",
			"provider_selections": {},
			"terraform_outdated": false
		}
	EOF
	exit 0
	;;
init)
	echo "init"
	exit 0
	;;
plan)
	trap 'json_print interrupt' INT

	json_print plan_start
	# Background sleep inherits SIG_IGN for SIGINT in
	# non-interactive shells, so group-wide interrupts
	# won't kill it.  Redirect output so the background
	# process doesn't hold Go's pipes open.  The
	# while-loop re-waits after the trap handler runs,
	# keeping the script alive.
	sleep 10 >/dev/null 2>&1 &
	CHILD=$!
	while kill -0 $CHILD 2>/dev/null; do
		wait $CHILD 2>/dev/null
	done
	json_print plan_end

	exit 0
	;;
apply)
	echo "apply not supported"
	exit 1
	;;
esac

exit 10
