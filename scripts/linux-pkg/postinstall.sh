#!/bin/sh
# Ensures /etc/coder.d/coder.env is owned by the coder user and readable
# only by its owner, since it may contain secrets such as the PostgreSQL
# connection URL. The file is marked noreplace, so package upgrades keep
# the existing copy and its old permissions unless tightened here. Every
# step is best-effort so this script never fails the package install.
set -eu

USER="coder"
ENV_FILE="/etc/coder.d/coder.env"

if [ -f "$ENV_FILE" ]; then
	chmod 0600 "$ENV_FILE" || true
	if id -u "$USER" >/dev/null 2>&1 && getent group "$USER" >/dev/null 2>&1; then
		chown "$USER:$USER" "$ENV_FILE" || true
	fi
fi

exit 0
