#!/usr/bin/env bash
set -u

if [[ -n "${FAKE_PULUMI_EXIT_CODE:-}" ]]; then
	exit "$FAKE_PULUMI_EXIT_CODE"
fi

cmd="${1:-}"
case "$cmd" in
version)
	printf 'v3.100.0\n'
	;;
login)
	printf 'Logged in to backend\n'
	;;
package)
	subcmd="${2:-}"
	case "$subcmd" in
	add)
		printf '%s\n' "$*" >>.pulumi-package-add-ran
		printf 'Generated package SDK\n'
		;;
	*)
		printf 'unknown command: %s\n' "$cmd" >&2
		exit 1
		;;
	esac
	;;
stack)
	subcmd="${2:-}"
	case "$subcmd" in
	init)
		printf "Created stack 'coder'\n"
		;;
	import)
		file=""
		shift 2
		while [[ $# -gt 0 ]]; do
			if [[ "$1" == "--file" && $# -ge 2 ]]; then
				file="$2"
				break
			fi
			shift
		done
		if [[ -n "$file" ]]; then
			cat "$file" >/dev/null
		fi
		printf 'Import complete.\n'
		;;
	export)
		cat <<'EOF'
{
  "version": 3,
  "deployment": {
    "manifest": {
      "time": "2024-01-01T00:00:00Z",
      "magic": "abc",
      "version": "v3.100.0"
    },
    "resources": [
      {
        "urn": "urn:pulumi:coder::project::pulumi:pulumi:Stack::project",
        "type": "pulumi:pulumi:Stack",
        "custom": false,
        "inputs": {},
        "outputs": {}
      },
      {
        "urn": "urn:pulumi:coder::project::docker:index/container:Container::workspace",
        "type": "docker:index/container:Container",
        "custom": true,
        "id": "container-abc123",
        "inputs": {
          "name": "workspace"
        },
        "outputs": {
          "id": "container-abc123",
          "name": "workspace"
        },
        "parent": "urn:pulumi:coder::project::pulumi:pulumi:Stack::project",
        "dependencies": []
      },
      {
        "urn": "urn:pulumi:coder::project::coder:index/agent:Agent::dev",
        "type": "coder:index/agent:Agent",
        "custom": true,
        "inputs": {},
        "outputs": {
          "id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
          "auth": "token",
          "token": "test-token",
          "os": "linux",
          "arch": "amd64",
          "dir": "/workspace"
        },
        "parent": "urn:pulumi:coder::project::pulumi:pulumi:Stack::project",
        "dependencies": [
          "urn:pulumi:coder::project::docker:index/container:Container::workspace"
        ]
      },
      {
        "urn": "urn:pulumi:coder::project::coder:index/app:App::code-server",
        "type": "coder:index/app:App",
        "custom": true,
        "inputs": {},
        "outputs": {
          "id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
          "agentId": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
          "slug": "code-server",
          "displayName": "Code Server",
          "url": "http://localhost:8080"
        },
        "parent": "urn:pulumi:coder::project::pulumi:pulumi:Stack::project",
        "dependencies": [
          "urn:pulumi:coder::project::coder:index/agent:Agent::dev"
        ]
      },
      {
        "urn": "urn:pulumi:coder::project::coder:index/parameter:Parameter::region",
        "type": "coder:index/parameter:Parameter",
        "custom": true,
        "inputs": {},
        "outputs": {
          "name": "region",
          "type": "string",
          "default": "us-east-1",
          "description": "Cloud region"
        },
        "parent": "urn:pulumi:coder::project::pulumi:pulumi:Stack::project",
        "dependencies": []
      }
    ]
  }
}
EOF
		;;
	*)
		printf 'unknown command: %s\n' "$cmd" >&2
		exit 1
		;;
	esac
	;;
preview)
	printf '{"steps":[],"changeSummary":{"same":0}}\n'
	;;
up)
	printf 'Updating (coder):\n Resources:\n    0 to create\n Update duration: 1s\n'
	;;
destroy)
	if [[ " $* " == *" --preview-only "* && " $* " == *" --json "* ]]; then
		printf '{"steps":[],"changeSummary":{"same":0}}\n'
	else
		printf 'Destroying (coder):\n Resources:\n    0 to delete\n Destroy duration: 1s\n'
	fi
	;;
*)
	printf 'unknown command: %s\n' "$cmd" >&2
	exit 1
	;;
esac
