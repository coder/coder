#!/usr/bin/env bash

# Usage: ./develop.sh [flags...] [-- extra server args...]
#
# This is a thin wrapper that delegates to the Go development orchestrator
# at scripts/develop. See that package for the full implementation.

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

# Prototype branch only: the coder-ephemeral workspace runs this script with
# no extra server flags, so turn on the automations prototype here.
export CODER_EXPERIMENTS=${CODER_EXPERIMENTS:-chat-automations,chat-projects}
export CODER_CHAT_MIN_AUTOMATION_SCHEDULE_INTERVAL=${CODER_CHAT_MIN_AUTOMATION_SCHEDULE_INTERVAL:-1m}

make -j MAKE_TIMED=1 build/.bin/develop
exec build/.bin/develop "$@"
