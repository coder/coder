#!/usr/bin/env bash
# render-diagram.sh: capture a docs page rendering for SVG diagram review.
#
# Usage:
#   render-diagram.sh <docs-url> <output-png> [viewport-width] [page-height]
#
# Defaults: viewport-width=1280, page-height=2400.
#
# Requires google-chrome (or chromium) in PATH and a docs dev server
# running at the URL.
#
# Why this exists: rendering the SVG directly with `chrome --headless
# file://.../foo.svg` does NOT match what the docs page produces. The
# docs frontend scales SVGs to a fixed prose-column width, and that is
# what readers see. Always test inside the docs page.

set -euo pipefail

URL="${1:-}"
OUT="${2:-}"
WIDTH="${3:-1280}"
HEIGHT="${4:-2400}"

if [[ -z "$URL" || -z "$OUT" ]]; then
	echo "usage: $0 <docs-url> <output-png> [viewport-width] [page-height]" >&2
	exit 2
fi

BROWSER=""
for c in google-chrome chromium chromium-browser; do
	if command -v "$c" >/dev/null 2>&1; then
		BROWSER="$c"
		break
	fi
done
if [[ -z "$BROWSER" ]]; then
	echo "no chrome or chromium found in PATH" >&2
	exit 1
fi

# Use a fresh user-data-dir per invocation to avoid clashing with any
# existing Chrome instance and to bypass disk cache.
TMP_PROFILE="$(mktemp -d -t chrome-render.XXXXXX)"
trap 'rm -rf "$TMP_PROFILE"' EXIT

# Cache-bust the URL so the dev server returns the latest asset.
SEP="?"
[[ "$URL" == *\?* ]] && SEP="&"
URL_CB="${URL}${SEP}t=$(date +%s%N)"

"$BROWSER" \
	--headless=new \
	--disable-gpu \
	--no-sandbox \
	--disable-dev-shm-usage \
	--hide-scrollbars \
	--user-data-dir="$TMP_PROFILE" \
	--window-size="${WIDTH},${HEIGHT}" \
	--screenshot="$OUT" \
	"$URL_CB" 2>&1 |
	grep -vE 'dbus|GPU process|registration_request|TensorFlow|XNNPACK' || true

if [[ ! -s "$OUT" ]]; then
	echo "screenshot failed: $OUT is empty or missing" >&2
	exit 1
fi

echo "saved $OUT"
md5sum "$OUT" 2>/dev/null || true
