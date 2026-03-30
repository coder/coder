#!/bin/bash
# bwrap-agent.sh: Start the Coder agent inside a bubblewrap sandbox.
#
# This script wraps the agent binary and all its children in a bwrap
# mount namespace with almost all capabilities dropped.
#
# Sandbox policy:
#   - Root filesystem is read-only (prevents system modification)
#   - /home/coder is read-write (project files, shared with dev agent)
#   - /tmp is read-write (scratch space, bind from container /tmp)
#   - /proc is bind-mounted from host (needed by CLI tools)
#   - /dev is bind-mounted from host (devices)
#   - Outbound TCP is restricted to the control-plane endpoint
#   - All capabilities dropped except DAC_OVERRIDE
#
# DAC_OVERRIDE is retained so the sandbox process (running as root)
# can read and write files owned by uid 1000 (coder) on the shared
# home volume without chowning them. This preserves correct
# ownership for the dev agent, which runs as the coder user.
#
# The container must run as root with CAP_SYS_ADMIN and CAP_NET_ADMIN
# so bwrap can create the mount namespace and this wrapper can install
# iptables rules. bwrap then drops all caps except DAC_OVERRIDE before
# exec'ing the child process.

set -euo pipefail

fail() {
	echo "bwrap-agent: $*" >&2
	exit 1
}

discover_control_plane_url() {
	if [ -n "${CODER_SANDBOX_CONTROL_PLANE_URL:-}" ]; then
		printf '%s\n' "$CODER_SANDBOX_CONTROL_PLANE_URL"
		return 0
	fi

	local arg url
	for arg in "$@"; do
		if [ -f "$arg" ]; then
			url=$(grep -aoE "https?://[^\"'[:space:]]+" "$arg" | head -n1 || true)
			if [ -n "$url" ]; then
				printf '%s\n' "$url"
				return 0
			fi
		fi
	done

	return 1
}

parse_control_plane_host_port() {
	local url="$1"
	local host_port host port

	host_port="${url#*://}"
	host_port="${host_port%%/*}"
	if [ -z "$host_port" ]; then
		fail "control-plane URL is missing a host: $url"
	fi

	if [[ "$host_port" == *'['* || "$host_port" == *']'* ]]; then
		fail "IPv6 control-plane URLs are not supported: $url"
	fi

	if [[ "$host_port" == *:* ]]; then
		host="${host_port%%:*}"
		port="${host_port##*:}"
	else
		host="$host_port"
		case "$url" in
		https://*) port=443 ;;
		http://*) port=80 ;;
		*) fail "unsupported control-plane URL scheme: $url" ;;
		esac
	fi

	if [[ -z "$host" || -z "$port" || ! "$port" =~ ^[0-9]+$ ]]; then
		fail "failed to parse control-plane host and port from: $url"
	fi

	printf '%s %s\n' "$host" "$port"
}

install_tcp_egress_rules() {
	local url="$1"
	local host port
	local -a control_plane_ips
	local -a iptables_cmd=(iptables -w 5)
	local chain="CODER_CHAT_SANDBOX_OUT"
	local ip

	read -r host port < <(parse_control_plane_host_port "$url")
	mapfile -t control_plane_ips < <(getent ahostsv4 "$host" | awk '{print $1}' | sort -u)
	if [ "${#control_plane_ips[@]}" -eq 0 ]; then
		fail "failed to resolve IPv4 address for control-plane host: $host"
	fi

	"${iptables_cmd[@]}" -N "$chain" 2>/dev/null || true
	"${iptables_cmd[@]}" -F "$chain"
	while "${iptables_cmd[@]}" -C OUTPUT -j "$chain" >/dev/null 2>&1; do
		"${iptables_cmd[@]}" -D OUTPUT -j "$chain"
	done
	"${iptables_cmd[@]}" -I OUTPUT 1 -j "$chain"

	"${iptables_cmd[@]}" -A "$chain" -o lo -j ACCEPT
	"${iptables_cmd[@]}" -A "$chain" -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
	for ip in "${control_plane_ips[@]}"; do
		"${iptables_cmd[@]}" -A "$chain" -p tcp -d "$ip" --dport "$port" -j ACCEPT
	done
	"${iptables_cmd[@]}" -A "$chain" -p tcp -j REJECT --reject-with tcp-reset
	"${iptables_cmd[@]}" -A "$chain" -j RETURN
}

command -v bwrap >/dev/null 2>&1 || fail "bubblewrap not found"
command -v getent >/dev/null 2>&1 || fail "getent not found"
command -v iptables >/dev/null 2>&1 || fail "iptables not found"

control_plane_url=$(discover_control_plane_url "$@" || true)
if [ -z "$control_plane_url" ]; then
	fail "failed to determine control-plane URL"
fi

install_tcp_egress_rules "$control_plane_url"

exec bwrap \
	--ro-bind / / \
	--bind /home/coder /home/coder \
	--bind /tmp /tmp \
	--bind /proc /proc \
	--dev-bind /dev /dev \
	--die-with-parent \
	--cap-drop ALL \
	--cap-add cap_dac_override \
	"$@"
