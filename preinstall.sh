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

	# Add the Coder user to the Docker group.
	# Coder is frequently used with Docker, so
	# this prevents failures when building.
	#
	# It's fine if this fails!
	usermod \
		--append \
		--groups docker \
		$USER 2>/dev/null || true
fi
