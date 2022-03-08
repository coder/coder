#!/usr/bin/env bash

set -euo pipefail

EMAIL=${EMAIL:-admin@coder.com}
USERNAME=${USERNAME:-admin}
ORGANIZATION=${ORGANIZATION:-ACME-Corp}
PASSWORD=${PASSWORD:-password}
PORT=${PORT:-8000}

# Helper to create an initial user
function create_initial_user() {
  # TODO: We need to wait for `coderd` to spin up -
  # need to replace with a deterministic strategy
  sleep 5s

  curl -X POST \
    -d '{"email": "'"$EMAIL"'", "username": "'"$USERNAME"'", "organization": "'"$ORGANIZATION"'", "password": "'"$PASSWORD"'"}' \
    -H 'Content-Type:application/json' \
    "http://localhost:$PORT/api/v2/users/first"
}

# This is a way to run multiple processes in parallel, and have Ctrl-C work correctly
# to kill both at the same time. For more details, see:
# https://stackoverflow.com/questions/3004811/how-do-you-run-multiple-programs-in-parallel-from-a-bash-script
(
  trap 'kill 0' SIGINT
  create_initial_user &
  /coder daemon --address=":$PORT"
)
