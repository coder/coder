#!/bin/bash
set -e

cleanup() {
	coder tokens remove scaletest_runner >/dev/null 2>&1 || true
}
trap cleanup EXIT

"${SCRIPTS_DIR}/cleanup.sh" shutdown
