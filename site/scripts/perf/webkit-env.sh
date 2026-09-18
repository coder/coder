#!/usr/bin/env bash
# Runs a command with an environment where Playwright's WebKit can launch.
#
# The prebuilt WebKit build expects Ubuntu 24.04-era shared libraries, but
# this image is Ubuntu 26.04, which ships newer sonames (ICU 78,
# libxml2.so.16, libvpx.so.12, harfbuzz built against ICU 78). Installing the
# older copies system-wide would change libraries for every other process, so
# they are extracted into a private prefix instead and exposed through
# LD_LIBRARY_PATH for this command only. The WebKit bundle's own lib/ also
# receives copies, because its launcher resets LD_LIBRARY_PATH.
#
# Usage: ./webkit-env.sh node scripts/perf/chat-bench.mjs --trials 3
set -euo pipefail

WK_ROOT="${WK_ROOT:-$HOME/.cache/ms-playwright/webkit-2203}"
LIBS_ROOT="/home/coder/wklibs-root/usr/lib/x86_64-linux-gnu"

if [ ! -d "$WK_ROOT" ]; then
	echo "WebKit is not installed at $WK_ROOT; run: npx playwright install webkit" >&2
	exit 1
fi

# Idempotently make the expected sonames resolvable inside the bundle.
if [ -d "$LIBS_ROOT" ]; then
	for lib in "$LIBS_ROOT"/lib{icu,xml2,vpx,event,woff,webpmux,flite,harfbuzz}*; do
		[ -e "$lib" ] || continue
		dest="$WK_ROOT/minibrowser-wpe/lib/$(basename "$lib")"
		[ -e "$dest" ] || cp "$lib" "$dest"
	done
fi

export LD_LIBRARY_PATH="$LIBS_ROOT:$WK_ROOT/minibrowser-wpe/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
export PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS=1
exec "$@"
