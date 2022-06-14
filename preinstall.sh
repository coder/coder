#!/bin/sh
set -eu

USER="coder"

if ! id -u $USER > /dev/null 2>&1; then
	useradd \
		--system \
		--user-group \
		--shell /bin/false \
		$USER
fi
