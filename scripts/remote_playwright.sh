#!/usr/bin/env bash
set -euo pipefail

workspace=${1:-}
coder_repo=${2:-.}
port=${3:-3000}

if [[ -z "${workspace}" ]]; then
	echo "Usage: $0 <workspace> [workspace coder/coder dir] [e2e port]"
	exit 1
fi

main() {
	# Check the Playwright version from the workspace so we have a 1-to-1 match
	# between the current branch and what we're going to run locally. This is
	# necessary because Playwright uses their own protocol for communicating
	# between the server and client, and the protocol changes between versions.
	echo "Checking Playwright version from \"${workspace}\"..."
	# shellcheck disable=SC2029 # This is intended to expand client-side.
	playwright_version="$(ssh "coder.${workspace}" "cat '${coder_repo}'/site/pnpm-lock.yaml | grep '^  /@playwright/test@' | cut -d '@' -f 3 | tr -d ':'")"

	echo "Found Playwright version ${playwright_version}..."

	# Let's store it in cache because, why not, this is ephemeral.
	dest=~/.cache/coder-remote-playwright
	echo "Initializing Playwright server in ${dest}..."
	mkdir -p "${dest}"
	cd "${dest}"
	echo '{"dependencies":{"@playwright/test":"'"${playwright_version}"'"}}' >package.json
	cat <<-EOF >server.mjs
		import { chromium } from "@playwright/test";

		const server = await chromium.launchServer({ headless: false });
		console.log(server.wsEndpoint());
	EOF

	npm_cmd=npm
	if command -v pnpm >/dev/null; then
		npm_cmd=pnpm
	fi
	echo "Running \"${npm_cmd} install\" to ensure local and remote are up-to-date..."
	"${npm_cmd}" install

	echo "Running \"${npm_cmd} exec playwright install\" for browser binaries..."
	"${npm_cmd}" exec playwright install

	playwright_out="$(mktemp -t playwright_server_out.XXXXXX)"

	rm "${playwright_out}"
	mkfifo "${playwright_out}"
	exec 3<>"${playwright_out}"

	echo "Starting Playwright server..."
	${npm_cmd} exec node server.mjs 1>&3 &
	playwright_pid=$!

	trap '
	kill -INT ${playwright_pid}
	exec 3>&-
	rm "${playwright_out}"
	wait ${playwright_pid}
	' EXIT

	echo "Waiting for Playwright to start..."
	read -r ws_endpoint <&3
	if [[ ${ws_endpoint} != ws://* ]]; then
		echo "Playwright failed to start."
		echo "${ws_endpoint}"
		cat "${playwright_out}"
		exit 1
	fi
	echo "Playwright started at ${ws_endpoint}"

	ws_port=${ws_endpoint##*:}
	ws_port=${ws_port%/*}

	port_args=(
		-R "${ws_port}:127.0.0.1:${ws_port}"
		-L "${port}:127.0.0.1:${port}"
	)

	# Also forward prometheus, pprof, and gitauth ports.
	for p in 2114 6061 50515 50516; do
		port_args+=(-L "${p}:127.0.0.1:${p}")
	done

	echo
	echo "Starting SSH tunnel, run test via \"pnpm run playwright:test\"..."
	# shellcheck disable=SC2029 # This is intended to expand client-side.
	ssh -t "${port_args[@]}" coder."${workspace}" "export CODER_E2E_PORT='${port}'; export CODER_E2E_WS_ENDPOINT='${ws_endpoint}'; [[ -d '${coder_repo}/site' ]] && cd '${coder_repo}/site'; exec \"\$(grep \"\${USER}\": /etc/passwd | cut -d: -f7)\" -i -l"
}

main
