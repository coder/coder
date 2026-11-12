#!/bin/sh

# A stand-in for the terraform binary. Tests link to this script from a
# directory of their own (see fakeTerraform in provision_test.go), and the
# script takes its instructions from the files in that directory:
#
#   commands     the arguments of each command, appended when it starts
#   plan.json    printed by `terraform show` for a plan file
#   state.json   printed by `terraform show` for the state
#   graph.N.dot  printed by the Nth `terraform graph`, which fails without it
#   real         path of a terraform binary that runs every command instead
#   hang         makes plan and graph wait to be interrupted

dir=${0%/*}
command=$1

if [ -e "$dir/hang" ]; then
	trap 'echo "interrupted $command" >>"$dir/commands"; exit 1' INT
fi
echo "$*" >>"$dir/commands"

if [ -e "$dir/real" ]; then
	exec "$(cat "$dir/real")" "$@"
fi

wait_for_interrupt() {
	[ -e "$dir/hang" ] || return 0
	# Give up after a minute, so that a process nothing interrupts ends.
	i=0
	while [ "$i" -lt 600 ]; do
		sleep 0.1
		i=$((i + 1))
	done
	exit 1
}

case "$command" in
version)
	echo '{"terraform_version":"1.15.0","platform":"linux_amd64","provider_selections":{}}'
	;;
init) ;;
plan)
	wait_for_interrupt
	;;
show)
	case "$*" in
	*.tfplan) cat "$dir/plan.json" ;;
	*) cat "$dir/state.json" ;;
	esac
	;;
graph)
	wait_for_interrupt
	cat "$dir/graph.$(grep -c '^graph' "$dir/commands").dot"
	;;
*)
	exit 1
	;;
esac
