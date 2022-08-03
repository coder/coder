#!/bin/sh
set -eu

USER="coder"

if id -u $USER >/dev/null 2>&1; then
	# If the coder user already exists, we don't need to add it.
	exit 0
fi

if command -V useradd >/dev/null 2>&1; then
	useradd \
		--create-home \
		--system \
		--user-group \
		--shell /bin/false \
		$USER

	usermod \
		--append \
		--groups docker \
		$USER >/dev/null 2>&1 || true
elif command -V adduser >/dev/null 2>&1; then
	# On alpine distributions useradd does not exist.
	# This is a backup!
	addgroup -S $USER
	adduser -G coder -S coder
	adduser coder docker >/dev/null 2>&1 || true
fi
