#!/bin/sh
# Sandbox boundary probes for the bubblewrap nested subagent template.
#
# This script runs as the child workload inside the sandbox, so it may only
# use the BusyBox applets the reference driver exposes and the paths the
# sandbox provides. It records what the child can and cannot reach, writes a
# plain-text report plus a small HTML page into the one shared project
# directory, and then serves that page with the BusyBox HTTP server.
#
# Two environment variables make the same script testable outside a sandbox:
#
#   PROBE_ROOT   prefix prepended to every absolute path the probes inspect,
#                so a synthetic directory tree can stand in for the sandbox.
#                Empty by default, which means the real paths.
#   PROBE_SERVE  set to 0 to skip the HTTP server and exit nonzero when any
#                probe failed. Any other value serves the page and keeps the
#                exit status of a successful server.
#   PROBE_SHARED path of the shared project directory inside the sandbox,
#                before PROBE_ROOT is applied.
#
# The child's own auth token is never read: the token probes only look at
# the file's mode bits, and the environment probe uses parameter expansion
# that reports whether the variable is set without expanding its value.

set -u

probe_root=${PROBE_ROOT-}
probe_serve=${PROBE_SERVE-1}
shared_dir="$probe_root${PROBE_SHARED-/workspace/project}"

# The fixed paths the driver builds the sandbox from, and the parent-side
# markers that must not be reachable from inside it.
child_home="$probe_root/home/coder"
child_tmp="$probe_root/tmp"
child_runtime="$probe_root/run/user/1000"
child_token="$probe_root/run/coder/token"
parent_dotfile="$child_home/.parent-dotfile-marker"
parent_ssh_key="$child_home/.ssh/parent-ssh-marker"
parent_private="$child_home/parent-private/parent-private-marker.txt"
shared_marker="$shared_dir/parent-shared-marker.txt"

report_path="$shared_dir/probe-results.txt"
html_path="$shared_dir/index.html"

pass_count=0
fail_count=0

record() {
	record_status=$1
	shift
	printf '%s: %s\n' "$record_status" "$*" >>"$report_path"
	if [ "$record_status" = PASS ]; then
		pass_count=$((pass_count + 1))
	else
		fail_count=$((fail_count + 1))
	fi
}

# note writes a line that is not a probe result, so it never affects the
# summary counts.
note() {
	printf 'NOTE: %s\n' "$*" >>"$report_path"
}

# check runs the remaining arguments as a command and records the outcome
# under the given description.
check() {
	check_description=$1
	shift
	if "$@"; then
		record PASS "$check_description"
	else
		record FAIL "$check_description"
	fi
}

# absent succeeds when nothing exists at the path, following the same rule
# for files, directories, sockets, and dangling symlinks.
absent() {
	if [ -e "$1" ] || [ -L "$1" ]; then
		return 1
	fi
	return 0
}

# writable succeeds when a new file can be created in the directory and
# removed again, which is a stronger statement than the mode bits.
writable() {
	writable_probe="$1/.probe-write-test.$$"
	if : >"$writable_probe" 2>/dev/null; then
		rm -f "$writable_probe"
		return 0
	fi
	return 1
}

# cannot_open_for_write succeeds only when the kernel refuses to open the file
# for writing. Unlike `test -w`, this observes a read-only bind mount rather
# than only the source file's mode bits. The empty append writes no token data
# even if an unexpected writable mount makes the open succeed.
cannot_open_for_write() {
	if (: >>"$1") 2>/dev/null; then
		return 1
	fi
	return 0
}

# equals compares an environment value against the exact path the driver
# sets, so a mount that moved is a failure rather than a silent difference.
equals() {
	[ "$1" = "$2" ]
}

# The shared directory has to exist before anything can be written to it,
# and it is also where the report itself lands.
if [ ! -d "$shared_dir" ]; then
	printf 'probe: shared project directory does not exist: %s\n' "$shared_dir" >&2
	exit 1
fi

: >"$report_path"

printf 'sandbox probe report\n' >>"$report_path"
printf 'generated: %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" >>"$report_path"
printf 'user: %s (uid %s)\n' "$(whoami)" "$(id -u)" >>"$report_path"
printf '\n' >>"$report_path"

# The one directory the human owner and the child share, read-write, with
# the marker the parent side left in it.
check "shared project directory exists: $shared_dir" test -d "$shared_dir"
check "shared project directory is writable" writable "$shared_dir"
check "parent shared marker is visible: $shared_marker" test -f "$shared_marker"

# Everything else the parent owns stays on the parent side of the boundary.
check "parent dotfile marker is absent: $parent_dotfile" absent "$parent_dotfile"
check "parent ssh marker is absent: $parent_ssh_key" absent "$parent_ssh_key"
check "parent private marker is absent: $parent_private" absent "$parent_private"

# The sandbox is not a Docker client: neither socket path is bound in.
check "docker socket is absent: $probe_root/var/run/docker.sock" absent "$probe_root/var/run/docker.sock"
check "docker socket is absent: $probe_root/run/docker.sock" absent "$probe_root/run/docker.sock"

# The sandbox root is bubblewrap's private tmpfs, so the host's system
# directories are not there at all.
check "host /usr is absent" absent "$probe_root/usr"
check "host /lib is absent" absent "$probe_root/lib"
check "host /root is absent" absent "$probe_root/root"
check "host /var is absent" absent "$probe_root/var"

# The environment the driver sets, and the directories it points at.
check "HOME is $child_home" equals "${HOME-}" "$child_home"
check "HOME is writable" writable "$child_home"
check "TMPDIR is $child_tmp" equals "${TMPDIR-}" "$child_tmp"
check "TMPDIR is writable" writable "$child_tmp"
check "XDG_RUNTIME_DIR is $child_runtime" equals "${XDG_RUNTIME_DIR-}" "$child_runtime"
check "XDG_RUNTIME_DIR is writable" writable "$child_runtime"

# A fresh /proc and bubblewrap's minimal /dev, rather than the host's.
check "/proc/self/status is readable" test -r "$probe_root/proc/self/status"
check "/dev/null is present" test -c "$probe_root/dev/null"
check "/dev/urandom is present" test -c "$probe_root/dev/urandom"
check "/dev/kmsg is absent" absent "$probe_root/dev/kmsg"

# The token file is bound read-only at a fixed path. Its contents are never
# read here. The write probe opens it for an empty append, which verifies the
# read-only mount without modifying the credential.
check "child token file is readable: $child_token" test -r "$child_token"
check "child token file is not writable" cannot_open_for_write "$child_token"

# The token value is not in the environment either. The expansion below
# yields the word "set" when the variable exists and nothing when it does
# not, so the value itself is never expanded.
check "CODER_AGENT_TOKEN is unset" test -z "${CODER_AGENT_TOKEN+set}"

printf '\n' >>"$report_path"
note 'network isolation is untested: the driver deliberately shares the host network namespace, so these probes make no claim about it.'
printf '\n' >>"$report_path"
printf 'SUMMARY: %s passed, %s failed, %s total\n' \
	"$pass_count" "$fail_count" "$((pass_count + fail_count))" >>"$report_path"

# The HTML page is the same report, escaped so a path or a marker name can
# never inject markup into the page the owner opens.
{
	printf '<!doctype html>\n'
	printf '<html>\n'
	printf '  <head><title>Coder sandbox probes</title></head>\n'
	printf '  <body>\n'
	printf '    <h1>Sandbox boundary probes</h1>\n'
	printf '    <p>Written from inside the bubblewrap sandbox into the one shared project directory.</p>\n'
	printf '    <pre>\n'
	sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g' -e 's/"/\&quot;/g' "$report_path"
	printf '    </pre>\n'
	printf '  </body>\n'
	printf '</html>\n'
} >"$html_path"

cat "$report_path"

if [ "$probe_serve" = 0 ]; then
	if [ "$fail_count" -ne 0 ]; then
		exit 1
	fi
	exit 0
fi

# BusyBox httpd daemonizes by default. Letting the startup script return is
# required for the child agent to transition from STARTING to READY; the
# bubblewrap PID namespace still tears the server down with the child agent.
busybox httpd -p 3000 -h "$shared_dir"
