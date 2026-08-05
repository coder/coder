#!/bin/sh
# Harness test for the sandbox boundary probes.
#
# It runs scripts/probe.sh against synthetic directory trees instead of a
# real sandbox, so the probe logic can be exercised anywhere: one tree that
# satisfies every probe, and several that violate one boundary each.
#
# Usage:
#
#   ./scripts/probe_harness_test.sh                 # run probes with /bin/sh
#   ./scripts/probe_harness_test.sh /bin/busybox sh # run probes with BusyBox
#
# The shell to run the probes with is passed as separate arguments, exactly
# as it would be executed. Quoting it as a single argument does not work,
# because "/bin/busybox sh" is not the name of any program.
#
# The probes distinguish readable from writable by mode bits, which root
# ignores, so the harness refuses to run as root.

set -eu

script_dir=$(dirname "$0")
probe_script="$script_dir/probe.sh"

# The value seeded into the fake token file. No probe reads a token, so this
# string must not appear anywhere in the generated report or page.
seeded_token='SEEDED-CHILD-TOKEN-must-not-be-reported'

if [ ! -f "$probe_script" ]; then
	printf 'harness: probe script not found: %s\n' "$probe_script" >&2
	exit 1
fi

if [ "$(id -u)" = 0 ]; then
	printf 'harness: refusing to run as root: the read-only and unwritable probes cannot fail for root\n' >&2
	exit 1
fi

if [ "$#" -eq 0 ]; then
	set -- /bin/sh
fi

if [ ! -x "$1" ]; then
	printf 'harness: probe shell is not executable: %s\n' "$1" >&2
	exit 1
fi

work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT INT TERM

failures=0

ok() {
	printf 'harness: ok    %s\n' "$*"
}

bad() {
	printf 'harness: FAIL  %s\n' "$*" >&2
	failures=$((failures + 1))
}

# make_root builds a synthetic tree that satisfies every probe. Character
# devices are symlinks to the host's, because an unprivileged user cannot
# create device nodes and the probes follow symlinks.
make_root() {
	make_root_path="$work_dir/$1"
	mkdir -p \
		"$make_root_path/workspace/project" \
		"$make_root_path/home/coder" \
		"$make_root_path/tmp" \
		"$make_root_path/run/user/1000" \
		"$make_root_path/run/coder" \
		"$make_root_path/proc/self" \
		"$make_root_path/dev"

	printf 'parent wrote this into the shared project directory\n' \
		>"$make_root_path/workspace/project/parent-shared-marker.txt"
	printf 'Name:\tprobe\nPid:\t1\n' >"$make_root_path/proc/self/status"
	ln -s /dev/null "$make_root_path/dev/null"
	ln -s /dev/urandom "$make_root_path/dev/urandom"

	printf '%s\n' "$seeded_token" >"$make_root_path/run/coder/token"
	chmod 400 "$make_root_path/run/coder/token"

	printf '%s' "$make_root_path"
}

# run_probe runs the probe script against a synthetic root with the exact
# environment the driver sets inside the sandbox, minus the child token
# which the driver never puts in the environment.
run_probe() {
	run_root=$1
	shift
	(
		unset CODER_AGENT_TOKEN
		HOME="$run_root/home/coder" \
			TMPDIR="$run_root/tmp" \
			XDG_RUNTIME_DIR="$run_root/run/user/1000" \
			PROBE_ROOT="$run_root" \
			PROBE_SERVE=0 \
			"$@" "$probe_script"
	) >"$run_root/probe.stdout" 2>"$run_root/probe.stderr"
}

count_lines() {
	grep -c "$1" "$2" 2>/dev/null || true
}

# assert_report checks the parts of the report that must hold for every run:
# both artifacts exist, the summary agrees with the recorded lines, the
# network caveat is stated, and the seeded token never appears.
assert_report() {
	assert_case=$1
	assert_root=$2
	assert_report_path="$assert_root/workspace/project/probe-results.txt"
	assert_html_path="$assert_root/workspace/project/index.html"

	if [ ! -f "$assert_report_path" ]; then
		bad "$assert_case: report was not written"
		return 0
	fi
	if [ ! -f "$assert_html_path" ]; then
		bad "$assert_case: page was not written"
		return 0
	fi
	ok "$assert_case: report and page were written"

	counted_pass=$(count_lines '^PASS: ' "$assert_report_path")
	counted_fail=$(count_lines '^FAIL: ' "$assert_report_path")
	summary_line=$(sed -n 's/^SUMMARY: //p' "$assert_report_path")

	if [ -z "$summary_line" ]; then
		bad "$assert_case: report has no summary line"
		return 0
	fi

	summary_pass=$(printf '%s\n' "$summary_line" | awk '{print $1}')
	summary_fail=$(printf '%s\n' "$summary_line" | awk '{print $3}')
	summary_total=$(printf '%s\n' "$summary_line" | awk '{print $5}')

	if [ "$summary_pass" != "$counted_pass" ] || [ "$summary_fail" != "$counted_fail" ]; then
		bad "$assert_case: summary ($summary_line) disagrees with $counted_pass PASS and $counted_fail FAIL lines"
	elif [ "$summary_total" != "$((counted_pass + counted_fail))" ]; then
		bad "$assert_case: summary total $summary_total is not the sum of its parts"
	elif [ "$summary_total" -lt 20 ]; then
		bad "$assert_case: only $summary_total probes ran"
	else
		ok "$assert_case: summary is consistent ($summary_line)"
	fi

	if grep -F -q 'network isolation is untested' "$assert_report_path"; then
		ok "$assert_case: report states that network isolation is untested"
	else
		bad "$assert_case: report does not state that network isolation is untested"
	fi

	if grep -F -q "$seeded_token" "$assert_report_path" || grep -F -q "$seeded_token" "$assert_html_path"; then
		bad "$assert_case: the seeded child token leaked into the output"
	else
		ok "$assert_case: the seeded child token is absent from the output"
	fi
}

assert_status() {
	if [ "$2" = "$3" ]; then
		ok "$1: exit status $3"
	else
		bad "$1: expected exit status $3, got $2"
	fi
}

assert_result() {
	assert_result_case=$1
	assert_result_root=$2
	assert_result_status=$3
	assert_result_text=$4
	if grep -q "^$assert_result_status: $assert_result_text" \
		"$assert_result_root/workspace/project/probe-results.txt"; then
		ok "$assert_result_case: $assert_result_status $assert_result_text"
	else
		bad "$assert_result_case: expected a $assert_result_status line for $assert_result_text"
	fi
}

# A tree that satisfies every probe: the run must succeed and report no
# failures.
pass_root=$(make_root pass)
if run_probe "$pass_root" "$@"; then
	pass_status=0
else
	pass_status=$?
fi
assert_status 'clean sandbox' "$pass_status" 0
assert_report 'clean sandbox' "$pass_root"
assert_result 'clean sandbox' "$pass_root" PASS 'shared project directory is writable'
assert_result 'clean sandbox' "$pass_root" PASS 'parent shared marker is visible'
assert_result 'clean sandbox' "$pass_root" PASS 'child token file is not writable'
assert_result 'clean sandbox' "$pass_root" PASS 'CODER_AGENT_TOKEN is unset'
if grep -q '^FAIL: ' "$pass_root/workspace/project/probe-results.txt"; then
	bad 'clean sandbox: a probe failed on a tree that satisfies all of them'
else
	ok 'clean sandbox: no probe failed'
fi

# A parent dotfile that leaked into the child's home.
dotfile_root=$(make_root dotfile-leak)
printf 'leaked\n' >"$dotfile_root/home/coder/.parent-dotfile-marker"
if run_probe "$dotfile_root" "$@"; then
	dotfile_status=0
else
	dotfile_status=$?
fi
assert_status 'leaked parent dotfile' "$dotfile_status" 1
assert_report 'leaked parent dotfile' "$dotfile_root"
assert_result 'leaked parent dotfile' "$dotfile_root" FAIL 'parent dotfile marker is absent'

# A Docker socket bound into the sandbox.
docker_root=$(make_root docker-socket)
mkdir -p "$docker_root/var/run"
printf 'not really a socket\n' >"$docker_root/var/run/docker.sock"
if run_probe "$docker_root" "$@"; then
	docker_status=0
else
	docker_status=$?
fi
assert_status 'exposed docker socket' "$docker_status" 1
assert_report 'exposed docker socket' "$docker_root"
assert_result 'exposed docker socket' "$docker_root" FAIL 'docker socket is absent'

# A host system directory visible inside the sandbox.
usr_root=$(make_root host-usr)
mkdir -p "$usr_root/usr/bin"
if run_probe "$usr_root" "$@"; then
	usr_status=0
else
	usr_status=$?
fi
assert_status 'host /usr present' "$usr_status" 1
assert_report 'host /usr present' "$usr_root"
assert_result 'host /usr present' "$usr_root" FAIL 'host /usr is absent'

if [ "$failures" -ne 0 ]; then
	printf 'harness: %s assertion(s) failed\n' "$failures" >&2
	exit 1
fi

printf 'harness: all assertions passed\n'
