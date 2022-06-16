#!/bin/sh
set -eu

USER="coder"

# Add a Coder user to run as in systemd.
if ! id -u $USER >/dev/null 2>&1; then
	useradd \
		--create-home \
		--system \
		--user-group \
		--shell /bin/false \
		$USER
fi
