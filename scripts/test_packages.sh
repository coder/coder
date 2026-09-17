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

# Test packages by CI wall time (test-go-pg ubuntu, Sep 2026), relative to the
# module path. The heavy packages (100s+) start first and run for most of the
# job; the light packages fill the remaining slots and their builds overlap
# with the heavy runs instead of forming a tail; the medium packages (20-100s)
# start once heavy slots free up. Roughly right is good enough: packages
# missing from the pattern set are skipped and everything else keeps go list
# order.
heavy="
coderd
enterprise/coderd
coderd/database
cli
coderd/database/migrations
"
medium="
coderd/x/chatd
enterprise/wsproxy
enterprise/cli
enterprise
codersdk/toolsdk
coderd/database/dbauthz
enterprise/coderd/prebuilds
coderd/notifications
coderd/x/chatd/chatstate
tailnet
coderd/database/dbpurge
scaletest/workspacebuild
coderd/httpapi
provisioner/terraform
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

# BSD awk rejects -v values that contain newlines.
heavy="${heavy//$'\n'/ }"
medium="${medium//$'\n'/ }"

go list -tags=testsmallbatch "$@" | awk -v heavy="$heavy" -v medium="$medium" -v module="$module" '
BEGIN {
	nh = split(heavy, hlist, /[[:space:]]+/)
	for (i = 1; i <= nh; i++) {
		if (hlist[i] != "") {
			hrank[module "/" hlist[i]] = i
		}
	}
	nm = split(medium, mlist, /[[:space:]]+/)
	for (i = 1; i <= nm; i++) {
		if (mlist[i] != "") {
			mrank[module "/" mlist[i]] = i
		}
	}
}
{
	if ($0 in hrank) {
		hfound[hrank[$0]] = $0
	} else if ($0 in mrank) {
		mfound[mrank[$0]] = $0
	} else {
		rest[++nr] = $0
	}
}
END {
	for (i = 1; i <= nh; i++) {
		if (i in hfound) {
			print hfound[i]
		}
	}
	for (i = 1; i <= nr; i++) {
		print rest[i]
	}
	for (i = 1; i <= nm; i++) {
		if (i in mfound) {
			print mfound[i]
		}
	}
}'
