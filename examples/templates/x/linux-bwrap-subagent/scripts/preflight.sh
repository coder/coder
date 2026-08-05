#!/bin/sh
# Read-only preflight for the bubblewrap nested subagent demo.
#
# It answers one question: is this checkout and this Docker host in a state
# where the end-to-end demo has a chance of working? It changes nothing. It
# builds nothing, it starts no container, it writes no file, and it never
# prints an environment variable, a token, or any other credential.
#
# Usage, from the template directory or from the repository root:
#
#	./scripts/preflight.sh
#	examples/templates/x/linux-bwrap-subagent/scripts/preflight.sh
#
# Every line is one of:
#
#	PASS  the requirement is satisfied here
#	WARN  the requirement could not be decided from this machine
#	FAIL  the requirement is not satisfied and the demo will not work
#	NOTE  context, never a result
#
# The exit status is 0 when nothing failed and 1 otherwise. A missing, stale,
# or dynamically linked agent artifact and an unusable Docker daemon are hard
# failures. Kernel and daemon settings that this process cannot observe from
# where it runs are warnings, because the setting that matters is the one
# inside the workspace container, not the one around this shell.
#
# What this script cannot do: it cannot prove that bubblewrap will build a
# nested sandbox inside a workspace container. Nothing short of a real launch
# does that. See the NOTE lines at the end for the two ways to find out.

set -u

pass_count=0
warn_count=0
fail_count=0

pass() {
	printf 'PASS: %s\n' "$*"
	pass_count=$((pass_count + 1))
}

warn() {
	printf 'WARN: %s\n' "$*"
	warn_count=$((warn_count + 1))
}

fail() {
	printf 'FAIL: %s\n' "$*"
	fail_count=$((fail_count + 1))
}

note() {
	printf 'NOTE: %s\n' "$*"
}

section() {
	printf '\n== %s ==\n' "$*"
}

# The template directory is derived from this script's own location, so the
# script works from the template directory, from the repository root, or from
# anywhere else. The repository root is five levels above the script
# directory: scripts -> linux-bwrap-subagent -> x -> templates -> examples.
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
template_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../../../../.." && pwd)

# The build target refreshes both of these. The first is the make output, the
# second is the copy coderd serves to agents, so a workspace agent runs the
# second one.
build_artifact="$repo_root/build/coder-slim_linux_amd64"
served_artifact="$repo_root/site/out/bin/coder-linux-amd64"
build_target='make build/coder-slim_linux_amd64'

section 'repository layout'

if [ -f "$repo_root/go.mod" ] && [ -f "$repo_root/Makefile" ]; then
	pass "repository root resolved: $repo_root"
else
	fail "repository root does not look like a coder checkout: $repo_root"
	note 'the remaining artifact checks will be wrong if this failed'
fi

for required in \
	README.md \
	Dockerfile \
	main.tf \
	drivers/bwrap.sh \
	scripts/parent-fixtures.sh \
	scripts/probe.sh \
	scripts/probe_harness_test.sh \
	scripts/preflight.sh; do
	if [ -f "$template_dir/$required" ]; then
		pass "template file present: $required"
	else
		fail "template file missing: $template_dir/$required"
	fi
done

section 'agent artifact'

# head_sha is the commit the artifact has to have been built from. The version
# string embeds an abbreviated SHA, so the comparison is a prefix test against
# the full commit.
head_sha=''
if command -v git >/dev/null 2>&1; then
	head_sha=$(git -C "$repo_root" rev-parse HEAD 2>/dev/null || true)
fi
if [ -z "$head_sha" ]; then
	warn 'git HEAD could not be read, so artifact freshness cannot be checked'
fi

# check_static reports whether the artifact at $1 is a static executable,
# using file(1) and ldd(1) independently: file describes what was linked, ldd
# describes what the loader would need.
check_static() {
	static_path=$1
	static_label=$2

	if command -v file >/dev/null 2>&1; then
		static_file_out=$(file -b -- "$static_path" 2>/dev/null || true)
		case "$static_file_out" in
		*'statically linked'*)
			pass "$static_label is statically linked according to file(1)"
			;;
		*'dynamically linked'*)
			fail "$static_label is dynamically linked according to file(1); rebuild with CGO_ENABLED=0 and without BoringCrypto"
			;;
		*)
			warn "$static_label linkage is not stated by file(1)"
			;;
		esac
	else
		warn "file(1) is not installed, so $static_label linkage was not checked with it"
	fi

	if command -v ldd >/dev/null 2>&1; then
		static_ldd_out=$(ldd -- "$static_path" 2>&1 || true)
		case "$static_ldd_out" in
		*'not a dynamic executable'* | *'statically linked'*)
			pass "$static_label needs no shared libraries according to ldd(1)"
			;;
		*)
			fail "$static_label requests shared libraries; the sandbox root has no library directories"
			;;
		esac
	else
		warn "ldd(1) is not installed, so $static_label loader needs were not checked"
	fi
}

# check_version runs the artifact's own version subcommand and compares the
# embedded SHA against HEAD. Running the binary is the only way to read the
# version that was compiled into it.
check_version() {
	version_path=$1
	version_label=$2

	if [ -z "$head_sha" ]; then
		return 0
	fi
	if [ "$(uname -s)" != Linux ] || [ "$(uname -m)" != x86_64 ]; then
		warn "$version_label cannot be executed on this host, so its commit was not checked"
		return 0
	fi

	version_out=$("$version_path" version 2>/dev/null | head -n 1)
	if [ -z "$version_out" ]; then
		warn "$version_label did not report a version, so its commit was not checked"
		return 0
	fi

	version_sha=$(printf '%s\n' "$version_out" | sed -n 's/.*+\([0-9a-f]\{7,\}\).*/\1/p')
	if [ -z "$version_sha" ]; then
		warn "$version_label version string carries no commit, so its commit was not checked"
		return 0
	fi

	case "$head_sha" in
	"$version_sha"*)
		pass "$version_label was built from HEAD ($version_sha)"
		;;
	*)
		fail "$version_label was built from $version_sha, not from HEAD; rerun '$build_target'"
		;;
	esac
}

for artifact_entry in "build/coder-slim_linux_amd64|$build_artifact" "site/out/bin/coder-linux-amd64|$served_artifact"; do
	artifact_label=${artifact_entry%%|*}
	artifact_path=${artifact_entry#*|}

	if [ ! -f "$artifact_path" ]; then
		fail "agent artifact missing: $artifact_path; run '$build_target'"
		continue
	fi
	if [ ! -x "$artifact_path" ]; then
		fail "agent artifact is not executable: $artifact_path"
		continue
	fi
	pass "agent artifact present: $artifact_label"
	check_static "$artifact_path" "$artifact_label"
	check_version "$artifact_path" "$artifact_label"
done

note "coderd serves $served_artifact to agents, so restart coderd after $build_target and before creating a workspace"

section 'docker'

docker_info=''
if ! command -v docker >/dev/null 2>&1; then
	fail 'docker CLI is not on PATH'
else
	pass 'docker CLI is on PATH'
	docker_info=$(docker info --format '{{.OSType}}|{{.SecurityOptions}}' 2>/dev/null || true)
	if [ -z "$docker_info" ]; then
		fail 'docker daemon is not reachable; the template creates a container'
	else
		pass 'docker daemon is reachable'
	fi
fi

if [ -n "$docker_info" ]; then
	docker_ostype=${docker_info%%|*}
	docker_security=${docker_info#*|}

	if [ "$docker_ostype" = linux ]; then
		pass 'docker daemon OSType is linux'
	else
		fail "docker daemon OSType is $docker_ostype; the template needs a Linux daemon"
	fi

	case "$docker_security" in
	*'name=rootless'*)
		fail 'docker daemon is rootless; the workspace container is then already inside a user namespace it cannot subdivide'
		;;
	*)
		pass 'docker daemon is not rootless'
		;;
	esac

	case "$docker_security" in
	*'name=userns'*)
		fail 'docker daemon uses userns-remap; bwrap cannot set up a uid map inside a remapped container'
		;;
	*)
		pass 'docker daemon does not use userns-remap'
		;;
	esac
fi

section 'user namespaces'

max_user_namespaces_path=/proc/sys/user/max_user_namespaces
if [ -r "$max_user_namespaces_path" ]; then
	max_user_namespaces=$(head -n 1 "$max_user_namespaces_path" 2>/dev/null)
	case "$max_user_namespaces" in
	'' | 0)
		fail "$max_user_namespaces_path is ${max_user_namespaces:-empty}; unprivileged user namespaces are disabled"
		;;
	*)
		pass "$max_user_namespaces_path is $max_user_namespaces"
		;;
	esac
else
	warn "$max_user_namespaces_path is unreadable, so user namespace availability is undecided"
fi

userns_clone_path=/proc/sys/kernel/unprivileged_userns_clone
if [ -r "$userns_clone_path" ]; then
	userns_clone=$(head -n 1 "$userns_clone_path" 2>/dev/null)
	case "$userns_clone" in
	1)
		pass "$userns_clone_path is 1"
		;;
	0)
		fail "$userns_clone_path is 0; unprivileged user namespace creation is blocked"
		;;
	*)
		warn "$userns_clone_path reports an unexpected value, so it is undecided"
		;;
	esac
else
	note "$userns_clone_path is absent, which is normal on kernels without that knob"
fi

apparmor_userns_path=/proc/sys/kernel/apparmor_restrict_unprivileged_userns
if [ -r "$apparmor_userns_path" ]; then
	apparmor_userns=$(head -n 1 "$apparmor_userns_path" 2>/dev/null)
	case "$apparmor_userns" in
	0)
		pass "$apparmor_userns_path is 0"
		;;
	1)
		warn "$apparmor_userns_path is 1; the template relies on apparmor=unconfined to get past it, and a daemon that refuses that option cannot run this template"
		;;
	*)
		warn "$apparmor_userns_path reports an unexpected value, so it is undecided"
		;;
	esac
else
	note "$apparmor_userns_path is absent, which is normal off Ubuntu 23.10 and later"
fi

uid_map_path=/proc/self/uid_map
if [ -r "$uid_map_path" ]; then
	# The columns are padded, so the whitespace is squeezed before the
	# comparison. A full identity map is the only mapping a workspace
	# container can subdivide; anything else means this process is already
	# inside a namespace with a narrower range.
	uid_map=$(head -n 1 "$uid_map_path" 2>/dev/null | tr -s ' \t' ' ' | sed -e 's/^ //' -e 's/ $//')
	if [ "$uid_map" = '0 0 4294967295' ]; then
		pass "$uid_map_path of this process is a full identity map"
	else
		warn "$uid_map_path of this process is '$uid_map', not a full identity map, so this shell is already inside a user namespace; the file that decides the demo is the one inside the workspace container"
	fi
else
	warn "$uid_map_path is unreadable, so namespace nesting is undecided"
fi

printf '\n'
note 'this script cannot prove that bubblewrap will build a nested sandbox: it inspects settings, it does not create a namespace'
note "the gated integration test is the cheapest real check: go test ./agent/subagentexec -run TestBwrapDriverSandboxIsolation -v (it skips, with a reason, on a host that cannot host a sandbox)"
note 'the only complete check is creating a workspace from this template and reading the probe report the child writes into the shared project directory'

printf '\nSUMMARY: %s passed, %s warned, %s failed\n' \
	"$pass_count" "$warn_count" "$fail_count"

if [ "$fail_count" -ne 0 ]; then
	exit 1
fi
exit 0
