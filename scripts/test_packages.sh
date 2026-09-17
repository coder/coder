#!/usr/bin/env bash

# Prints the Go packages matching the given go list patterns with the slowest
# test packages first.
#
# go test starts packages in argument order, so with ./... (alphabetical) the
# heaviest packages start late and set the end of the run: in CI
# enterprise/coderd (110s) did not start until t=313s of a 423s run. Listing the
# slow packages first lets the -p scheduler pack the long tail behind them.
#
# Usage: ./scripts/test_packages.sh [go list patterns...]   (default: ./...)

set -euo pipefail

# Slowest test packages by CI wall time (test-go-pg ubuntu, Sep 2026), slowest
# first, relative to the module path. Roughly right is good enough: packages
# missing from the pattern set are skipped and everything else keeps go list
# order.
heavy="
coderd
coderd/database
cli
coderd/database/migrations
enterprise/coderd
enterprise
enterprise/cli
coderd/x/chatd
codersdk/toolsdk
enterprise/wsproxy
coderd/notifications
coderd/database/dbauthz
tailnet
coderd/database/dbpurge
scaletest/workspacebuild
coderd/httpapi
provisioner/terraform
enterprise/coderd/prebuilds
coderd/x/chatd/chatstate
scaletest/createworkspaces
coderd/database/pubsub
agent
enterprise/tailnet
provisionersdk
coderd/oauth2provider
scaletest/dashboard
coderd/x/nats
agent/agentcontainers
"

if [[ $# -eq 0 ]]; then
	set -- ./...
fi

module="$(go list -m)"

go list -tags=testsmallbatch "$@" | awk -v heavy="$heavy" -v module="$module" '
BEGIN {
	n = split(heavy, list, /[[:space:]]+/)
	for (i = 1; i <= n; i++) {
		if (list[i] != "") {
			rank[module "/" list[i]] = i
		}
	}
}
{
	if ($0 in rank) {
		found[rank[$0]] = $0
	} else {
		rest[++m] = $0
	}
}
END {
	for (i = 1; i <= n; i++) {
		if (i in found) {
			print found[i]
		}
	}
	for (i = 1; i <= m; i++) {
		print rest[i]
	}
}'
