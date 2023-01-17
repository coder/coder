#!/bin/sh

VERSION=$1
MODE=$2
shift 2

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
	case "$MODE" in
	apply)
		echo "init"
		;;
	init)
		sleep 10 &
		sleep_pid=$!

		trap 'echo exit; kill -9 $sleep_pid 2>/dev/null' EXIT
		trap 'echo interrupt; exit 1' INT
		trap 'echo terminate; exit 2' TERM

		echo init_start
		wait
		echo init_end
		;;
	esac
	;;
apply)
	sleep 10 &
	sleep_pid=$!

	trap 'json_print exit; kill -9 $sleep_pid 2>/dev/null' EXIT
	trap 'json_print interrupt; exit 1' INT
	trap 'json_print terminate; exit 2' TERM

	json_print apply_start
	wait
	json_print apply_end
	;;
plan)
	echo "plan not supported"
	exit 1
	;;
esac

exit 0
