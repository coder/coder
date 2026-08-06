#!/bin/sh
# bwrap.sh is the vetted Linux reference driver for Coder subagent execution
# isolation. It speaks driver protocol version 1, so the launcher invokes it
# as exactly:
#
#	bwrap.sh run     <input-json-path>
#	bwrap.sh cleanup <input-json-path>
#
# The launcher runs it directly, never through a shell, with an environment
# built from scratch and a controlled PATH. It reads its input from the JSON
# document at <input-json-path> with jq: nothing is hand-parsed, every
# expansion is quoted, and there is no eval, no `sh -c`, and no command
# string built from an input value. Every input value therefore stays one
# argv element.
#
# The child's auth token never reaches this script. The document carries the
# path of the launcher's private 0600 token file, and the only thing the
# script does with it is bind it read-only into the sandbox at a fixed path
# the child agent reads through CODER_AGENT_TOKEN_FILE.
#
# Sandbox policy for `run`:
#   - user, pid, ipc, uts, and (where supported) cgroup namespaces are
#     unshared, all capabilities are dropped, the environment is cleared,
#     and the sandbox gets a new session so it cannot inject into the
#     launcher's terminal;
#   - the network namespace is deliberately shared with the host. The child
#     agent has to reach the deployment, and network egress isolation is out
#     of scope for this driver;
#   - the root filesystem is bubblewrap's private empty tmpfs. The host root
#     is never bound, so /usr, /lib, the parent's home directory, the
#     launcher's private state root, and host sockets such as the Docker or
#     SSH agent socket are simply absent;
#   - /bin is a minimal BusyBox layout, /etc holds generated account files
#     plus the host's resolver and CA files when they exist, and the Coder
#     binary is read-only at a fixed path;
#   - the child's home, temporary, and runtime directories are the private
#     per-execution directories the launcher created;
#   - exactly one host directory is writable: the shared project directory
#     the deployment declared, at the child path it declared.
#
# Deployment prerequisites: unprivileged user namespaces must be permitted,
# and the Coder binary the launcher passes must be statically linked,
# because no host library directory is mounted into the sandbox.

# Globbing is disabled for the whole script. Two fixed, driver-owned lists
# below are expanded unquoted so the shell splits them into words, and
# disabling pathname expansion keeps a word such as the `[` applet from ever
# being treated as a glob. Every input value is quoted regardless.
set -euf

# Protocol version this driver implements.
readonly protocol_version=1

# The child-side layout. It is fixed rather than derived from the input, so
# the paths the child sees cannot be influenced by a declaration. The
# launcher's own path policy reserves all of these from the declared shared
# child path.
readonly child_uid=1000
readonly child_gid=1000
readonly child_user=coder
readonly child_group=coder
readonly child_hostname=coder-subagent
readonly child_bin=/bin
readonly child_busybox=/bin/busybox
readonly child_shell=/bin/sh
readonly child_home=/home/coder
readonly child_tmp=/tmp
readonly child_runtime=/run/user/1000
readonly child_token=/run/coder/token
readonly child_coder=/opt/coder/coder
# The agent's own log, script, and nested-execution state directories live
# under the child's private home, named the same way on both sides so the
# directory the driver creates is the one the agent is pointed at.
readonly agent_state_dir=.coder-agent

# The BusyBox applets the sandbox exposes: enough for an interactive shell
# and terminal session, ordinary file and text handling, and the BusyBox
# HTTP server the reference template's example workload runs.
readonly busybox_applets='[ ash awk basename cat chmod clear cp cut date dirname
	df du echo env expr false find grep gunzip gzip head hostname httpd id kill
	killall less ln ls mkdir mktemp more mv nc od printenv printf ps pwd
	readlink realpath reset rm rmdir sed seq sh sleep sort stat stty tail tar
	tee test top touch tr true tty uname uniq vi wc wget which whoami xargs'

# CA bundle locations, in the order they are tried. Only the first one that
# exists on the host is mounted; a host without any of them leaves the
# sandbox without a bundle rather than being rejected, because the child may
# well be talking to a deployment whose certificate it trusts another way.
readonly ca_bundle_candidates='/etc/ssl/certs/ca-certificates.crt
	/etc/pki/tls/certs/ca-bundle.crt
	/etc/ssl/ca-bundle.pem
	/etc/ssl/cert.pem'

fail() {
	printf 'bwrap driver: %s\n' "$*" >&2
	exit 1
}

log() {
	printf 'bwrap driver: %s\n' "$*"
}

require_tool() {
	command -v "$1" >/dev/null 2>&1 || fail "required tool not found on PATH: $1"
}

# tool_path prints the absolute path of a required tool. A tool that
# resolves to a relative path is refused: the result is bound into the
# sandbox, so it has to be unambiguous.
tool_path() {
	resolved=$(command -v "$1")
	case "$resolved" in
	/*) ;;
	*) fail "tool $1 did not resolve to an absolute path: $resolved" ;;
	esac
	printf '%s' "$resolved"
}

# json_string prints the named field of the input document when it is a
# string, and nothing otherwise. jq owns the parsing, so a value containing
# spaces, quotes, or shell metacharacters is returned verbatim and stays one
# argv element.
json_string() {
	jq -r --arg field "$1" 'if (.[$field] | type) == "string" then .[$field] else "" end' "$input"
}

# json_number prints the named field when it is a number, and nothing
# otherwise.
json_number() {
	jq -r --arg field "$1" 'if (.[$field] | type) == "number" then (.[$field] | tostring) else "" end' "$input"
}

require_nonempty() {
	[ -n "$2" ] || fail "input field $1 must be a non-empty string"
}

require_absolute() {
	require_nonempty "$1" "$2"
	case "$2" in
	/*) ;;
	*) fail "input field $1 must be an absolute path" ;;
	esac
	case "$2" in
	*/../* | */.. | */./* | */.) fail "input field $1 must not contain traversal components" ;;
	esac
}

require_directory() {
	[ -d "$2" ] || fail "input field $1 does not name an existing directory"
}

[ "$#" -eq 2 ] || fail 'usage: bwrap.sh <run|cleanup> <input-json-path>'
operation="$1"
input="$2"

case "$operation" in
run | cleanup) ;;
*) fail "unsupported operation: $operation" ;;
esac

# jq is needed to read anything at all, including the fields the validation
# below reports on.
require_tool jq
[ -f "$input" ] || fail "input document does not exist: $input"

document_version=$(json_number protocol_version)
[ "$document_version" = "$protocol_version" ] ||
	fail "unsupported driver protocol $document_version: this driver speaks protocol $protocol_version"

# The operation is carried in both the argument list and the document. They
# must agree: a mismatch means the launcher and this driver disagree about
# what is being asked, which is never safe to guess at.
document_operation=$(json_string operation)
[ "$document_operation" = "$operation" ] ||
	fail "operation $operation does not match the input document operation $document_operation"

execution_id=$(json_string execution_id)
generation=$(json_string generation)
child_agent_id=$(json_string child_agent_id)
child_agent_name=$(json_string child_agent_name)
coder_url=$(json_string coder_url)
coder_binary_path=$(json_string coder_binary_path)
token_file_path=$(json_string token_file_path)
shared_host_path=$(json_string shared_host_path)
shared_child_path=$(json_string shared_child_path)
state_path=$(json_string state_path)
home_path=$(json_string home_path)
tmp_path=$(json_string tmp_path)
runtime_path=$(json_string runtime_path)

require_nonempty execution_id "$execution_id"
require_nonempty generation "$generation"
require_nonempty child_agent_id "$child_agent_id"
require_nonempty child_agent_name "$child_agent_name"
require_nonempty coder_url "$coder_url"
require_absolute coder_binary_path "$coder_binary_path"
require_absolute token_file_path "$token_file_path"
require_absolute shared_host_path "$shared_host_path"
require_absolute shared_child_path "$shared_child_path"
require_absolute state_path "$state_path"
require_absolute home_path "$home_path"
require_absolute tmp_path "$tmp_path"
require_absolute runtime_path "$runtime_path"

# The launcher already validated the declared child path against the reserved
# child directories, and it derives every private path from its own state
# root. This driver still refuses the filesystem root for all of them: as a
# mount source or destination it would expose the host, and as the private
# runtime directory it would make cleanup remove host files.
for guarded_path in \
	"$coder_binary_path" \
	"$token_file_path" \
	"$shared_host_path" \
	"$shared_child_path" \
	"$state_path" \
	"$home_path" \
	"$tmp_path" \
	"$runtime_path"; do
	[ "$guarded_path" != "/" ] || fail 'input path fields must not be the filesystem root'
done

# Paths the driver generates support files at, and the private runtime
# directory the child sees. Keeping the generated account files in a sibling
# of the child's runtime directory means the child never has them on a
# writable mount, and cleanup has an exact list of files to remove.
readonly generated_etc="$runtime_path/etc"
readonly generated_passwd="$generated_etc/passwd"
readonly generated_group="$generated_etc/group"
readonly generated_nsswitch="$generated_etc/nsswitch.conf"
readonly generated_os_release="$generated_etc/os-release"
readonly child_runtime_source="$runtime_path/xdg"

# write_generated_etc creates the minimal account and resolution files the
# sandbox needs. Nothing in them comes from the input document, so a
# declaration cannot inject an account or a resolution rule.
write_generated_etc() {
	mkdir -p "$generated_etc"
	# These files are not secret, and the child has to read them.
	umask 022

	{
		printf 'root:x:0:0:root:/root:%s\n' "$child_shell"
		printf '%s:x:%s:%s:Coder Subagent:%s:%s\n' \
			"$child_user" "$child_uid" "$child_gid" "$child_home" "$child_shell"
		printf 'nobody:x:65534:65534:nobody:/:%s\n' "$child_shell"
	} >"$generated_passwd"

	{
		printf 'root:x:0:\n'
		printf '%s:x:%s:\n' "$child_group" "$child_gid"
		printf 'nogroup:x:65534:\n'
	} >"$generated_group"

	{
		printf 'passwd: files\n'
		printf 'group: files\n'
		printf 'shadow: files\n'
		printf 'hosts: files dns\n'
	} >"$generated_nsswitch"

	# The Coder agent's resource monitor identifies Linux through
	# /etc/os-release. The empty-root sandbox has no distribution metadata, so
	# provide a minimal synthetic identity rather than exposing the host's file.
	{
		printf 'NAME="Coder Sandbox"\n'
		printf 'PRETTY_NAME="Coder Sandbox"\n'
		printf 'ID=coder-sandbox\n'
		printf 'ID_LIKE=debian\n'
		printf 'VERSION="1"\n'
		printf 'VERSION_ID="1"\n'
	} >"$generated_os_release"
}

# find_ca_bundle prints the first CA bundle that exists on the host, or
# nothing when the host has none.
find_ca_bundle() {
	for candidate in $ca_bundle_candidates; do
		if [ -f "$candidate" ]; then
			printf '%s' "$candidate"
			return 0
		fi
	done
	return 0
}

# do_run builds the bubblewrap argument list and replaces this process with
# it, so the launcher supervises the sandbox directly. The list is built with
# the positional parameters: every value is appended as its own element and
# never spliced into a string.
do_run() {
	require_tool bwrap
	bwrap_path=$(tool_path bwrap)
	# BusyBox must be statically linked, because no host library directory is
	# mounted into the sandbox. Distributions ship the static build under
	# either name, so both are accepted; whichever is found has to be the
	# static build.
	if command -v busybox >/dev/null 2>&1; then
		busybox_path=$(tool_path busybox)
	elif command -v busybox-static >/dev/null 2>&1; then
		busybox_path=$(tool_path busybox-static)
	else
		fail 'required tool not found on PATH: busybox'
	fi

	if [ ! -f "$coder_binary_path" ] || [ ! -x "$coder_binary_path" ]; then
		fail "coder binary is not an executable file: $coder_binary_path"
	fi
	[ -f "$token_file_path" ] || fail 'child token file does not exist'
	require_directory shared_host_path "$shared_host_path"
	require_directory home_path "$home_path"
	require_directory tmp_path "$tmp_path"
	require_directory runtime_path "$runtime_path"

	write_generated_etc
	ca_bundle=$(find_ca_bundle)

	# The child's private runtime directory and the agent's own state
	# directories are created here so the sandboxed agent finds them already
	# present. All of them live under the launcher's private per-execution
	# state, never in the shared project directory.
	umask 077
	mkdir -p "$child_runtime_source"
	mkdir -p "$home_path/$agent_state_dir/log"
	mkdir -p "$home_path/$agent_state_dir/scripts"
	mkdir -p "$home_path/$agent_state_dir/subagent-exec"

	# Namespaces, privileges, and environment. The network namespace is
	# intentionally absent from this list.
	set -- \
		--unshare-user \
		--unshare-pid \
		--unshare-ipc \
		--unshare-uts \
		--unshare-cgroup-try \
		--hostname "$child_hostname" \
		--uid "$child_uid" \
		--gid "$child_gid" \
		--die-with-parent \
		--new-session \
		--cap-drop ALL \
		--clearenv

	# A fresh /proc and bubblewrap's minimal /dev. Neither is bound from the
	# host, so the sandbox sees only its own processes and a small device set.
	set -- "$@" --proc /proc --dev /dev

	# A minimal /bin: the static BusyBox binary read-only, with one symlink
	# per applet. The symlinks are created inside the sandbox, so no host
	# directory is exposed to provide them.
	set -- "$@" --ro-bind "$busybox_path" "$child_busybox"
	for applet in $busybox_applets; do
		set -- "$@" --symlink "$child_busybox" "$child_bin/$applet"
	done

	# A minimal /etc: generated account files, plus the host's resolver and
	# CA files read-only when the host has them. A missing optional file is
	# simply not mounted.
	set -- "$@" \
		--ro-bind "$generated_passwd" /etc/passwd \
		--ro-bind "$generated_group" /etc/group \
		--ro-bind "$generated_nsswitch" /etc/nsswitch.conf \
		--ro-bind "$generated_os_release" /etc/os-release
	if [ -f /etc/resolv.conf ]; then
		set -- "$@" --ro-bind /etc/resolv.conf /etc/resolv.conf
	fi
	if [ -f /etc/hosts ]; then
		set -- "$@" --ro-bind /etc/hosts /etc/hosts
	fi
	if [ -n "$ca_bundle" ]; then
		set -- "$@" --ro-bind "$ca_bundle" "$ca_bundle"
	fi

	# The Coder binary is read-only, so the child cannot replace the
	# executable the launcher chose.
	set -- "$@" --ro-bind "$coder_binary_path" "$child_coder"

	# The child's private home, temporary, and runtime directories.
	set -- "$@" \
		--bind "$home_path" "$child_home" \
		--bind "$tmp_path" "$child_tmp" \
		--bind "$child_runtime_source" "$child_runtime"

	# The token file is read-only at a fixed path. Its value is never read
	# here, placed in the argument list, or exported.
	set -- "$@" --ro-bind "$token_file_path" "$child_token"

	# The one writable host directory: the declared shared project.
	set -- "$@" --bind "$shared_host_path" "$shared_child_path"
	set -- "$@" --chdir "$shared_child_path"

	# The child's environment, built from nothing by --clearenv above.
	set -- "$@" \
		--setenv PATH "/opt/coder:$child_bin" \
		--setenv HOME "$child_home" \
		--setenv USER "$child_user" \
		--setenv LOGNAME "$child_user" \
		--setenv SHELL "$child_shell" \
		--setenv TMPDIR "$child_tmp" \
		--setenv XDG_RUNTIME_DIR "$child_runtime" \
		--setenv TERM xterm-256color \
		--setenv CODER_AGENT_URL "$coder_url" \
		--setenv CODER_AGENT_AUTH token \
		--setenv CODER_AGENT_TOKEN_FILE "$child_token"

	# The child agent itself, in the foreground. Its diagnostics listeners
	# are disabled, devcontainer detection is off because there is no
	# container runtime in the sandbox, and every state directory it writes
	# to is private to this execution.
	set -- "$@" -- \
		"$child_coder" agent \
		--log-dir "$child_home/$agent_state_dir/log" \
		--script-data-dir "$child_home/$agent_state_dir/scripts" \
		--subagent-exec-state-dir "$child_home/$agent_state_dir/subagent-exec" \
		--pprof-address= \
		--prometheus-address= \
		--debug-address= \
		--devcontainers-enable=false

	log "starting sandboxed child agent for execution $execution_id"
	exec "$bwrap_path" "$@"
}

# do_cleanup removes what run generated and nothing else.
#
# There are no namespaces or mounts to tear down: bubblewrap's namespaces and
# mounts are scoped to the sandbox process, so they are gone the moment the
# run exits. What is left is the support files this driver wrote, all of them
# under the private runtime directory. The shared project directory and the
# token file are never touched: the launcher removes the private state,
# including the token, after cleanup returns.
#
# Cleanup is idempotent and succeeds when there is nothing left to remove.
do_cleanup() {
	rm -f "$generated_passwd" "$generated_group" "$generated_nsswitch" "$generated_os_release"
	# The directory only ever holds the generated support files, so a
	# non-empty rmdir failure means something else put a file there and is
	# left alone.
	rmdir "$generated_etc" 2>/dev/null || true
	log "cleaned up runtime support files for execution $execution_id"
}

case "$operation" in
run) do_run ;;
cleanup) do_cleanup ;;
esac
