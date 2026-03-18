#!/bin/bash
# Adds launch flags to all Google Chrome .desktop files so that Chrome
# works correctly in headless / GPU-less environments (e.g. Coder
# workspaces running inside Docker containers).
#
# This script is idempotent.

set -euo pipefail

CHROME_FLAGS=(
	--use-gl=angle
	--use-angle=swiftshader
	--disable-dev-shm-usage
	--no-first-run
	--no-default-browser-check
	--disable-background-networking
	--disable-sync
	--start-maximized
)

FLAGS_STR="${CHROME_FLAGS[*]}"

for desktop_file in /usr/share/applications/google-chrome*.desktop /usr/share/applications/com.google.Chrome*.desktop; do
	[ -f "$desktop_file" ] || continue
	# Skip if flags are already present.
	if grep -q -- '--use-gl=angle' "$desktop_file"; then
		continue
	fi
	# Insert flags after the binary path on every Exec= line.
	sed -i "s|Exec=/usr/bin/google-chrome-stable|Exec=/usr/bin/google-chrome-stable ${FLAGS_STR}|" "$desktop_file"
done
