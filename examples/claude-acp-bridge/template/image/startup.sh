#!/bin/sh
set -eu
umask 077

# Native CLIs are pinned in the image. The init script has already downloaded
# this deployment's Coder binary into BINARY_DIR, which is on PATH.
# Configure only prepares project/auth files and records the literal prompt.
# The agent restores sessions and starts the initial job after scripts finish.
exec coder native-agent configure --setup
