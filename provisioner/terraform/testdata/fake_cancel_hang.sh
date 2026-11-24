#!/usr/bin/env sh

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
	echo "init"
	exit 0
	;;
plan)
	case "$MODE" in
	hang)
		trap 'json_print interrupt' INT

		json_print plan_start
		sleep 10 2>/dev/null >/dev/null
		json_print plan_end
		;;
	second-interrupt)
		# Like Terraform, keep running after the first interrupt and exit
		# after the second.
		interrupts=0
		trap 'interrupts=$((interrupts + 1)); if [ "$interrupts" -ge 2 ]; then exit 1; fi' INT

		sleep 10 2>/dev/null >/dev/null &
		sleep_pid=$!
		trap 'kill $sleep_pid 2>/dev/null' EXIT

		json_print plan_start
		while kill -0 "$sleep_pid" 2>/dev/null; do
			wait "$sleep_pid"
		done
		json_print plan_end
		;;
	esac

	exit 0
	;;
apply)
	echo "apply not supported"
	exit 1
	;;
esac

exit 10
