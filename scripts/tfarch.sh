#!/bin/sh

set -e
arch=$(arch)
if [ "$arch" = "x86_64" ]; then
	arch="amd64"
elif [ "$arch" = "aarch64" ]; then
	arch="arm64"
fi
printf "%s" "$arch"
