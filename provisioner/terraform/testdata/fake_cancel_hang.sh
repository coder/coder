#!/bin/sh

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
apply)
	trap 'json_print interrupt' INT

	json_print apply_start
	sleep 10 2>/dev/null >/dev/null
	json_print apply_end

	exit 0
	;;
plan)
	echo "plan not supported"
	exit 1
	;;
esac

exit 0
