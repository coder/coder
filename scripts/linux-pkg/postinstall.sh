#!/bin/sh
set -eu

# Coder's config directory moved from /etc/coder.d/ to /etc/coder/ to drop the
# misleading drop-in ".d" suffix. This script preserves user edits to env files
# in the old location by symlinking them into the new directory.
#
# Behaviour summary:
#   - Always make sure /etc/coder/ exists and is owned by coder:coder.
#   - For each *.env in /etc/coder.d/, link it into /etc/coder/ when no real
#     file with the same name already lives there.
#   - Tell systemd to reload unit files so the new EnvironmentFile= path takes
#     effect on the next start.

OLD_DIR="/etc/coder.d"
NEW_DIR="/etc/coder"

if [ ! -d "$NEW_DIR" ]; then
	mkdir -p "$NEW_DIR"
fi

# preinstall.sh creates the coder user; chown best-effort so packages still
# install on systems where useradd silently failed.
if id -u coder >/dev/null 2>&1; then
	chown coder:coder "$NEW_DIR" 2>/dev/null || true
fi
chmod 0755 "$NEW_DIR" 2>/dev/null || true

# Migrate env files from the legacy directory.
if [ -d "$OLD_DIR" ]; then
	for old_file in "$OLD_DIR"/*.env; do
		# Glob may not match anything; guard with -e.
		[ -e "$old_file" ] || continue
		name=$(basename "$old_file")
		new_file="$NEW_DIR/$name"

		# Already linked here, nothing to do.
		if [ -L "$new_file" ]; then
			continue
		fi

		# Only migrate when the new location has no real config yet, or when
		# the file the package just dropped in is the unmodified template. The
		# shipped coder.env is ~291 bytes; treat anything <= 512 bytes that
		# contains no value assignments as the default placeholder.
		if [ -f "$new_file" ]; then
			if [ "$name" != "coder.env" ]; then
				continue
			fi
			size=$(wc -c <"$new_file" 2>/dev/null | tr -d ' ')
			if [ -z "$size" ] || [ "$size" -gt 512 ]; then
				continue
			fi
			# Skip if the user already filled in any value.
			if grep -Eq '^[[:space:]]*CODER_[A-Z_]+=[^[:space:]]' "$new_file"; then
				continue
			fi
			rm -f "$new_file"
		fi

		ln -s "$old_file" "$new_file"
	done
fi

# Refresh systemd's view of the unit files now that EnvironmentFile= points at
# the new directory. Restarting the service is left to the operator.
if command -v systemctl >/dev/null 2>&1; then
	systemctl daemon-reload >/dev/null 2>&1 || true
fi
