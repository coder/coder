#!/usr/bin/env bash
# timed-shell.sh wraps bash with per-target wall-clock timing.
#
# Recipe invocation:    timed-shell.sh <target> -ceu <recipe>
# $(shell ...) calls:   timed-shell.sh -c <command>
#
# Enable via Makefile:
#   SHELL := $(CURDIR)/scripts/lib/timed-shell.sh
#   .SHELLFLAGS = $@ -ceu
#
# $(shell ...) uses SHELL but passes -c directly, not .SHELLFLAGS.
# Detect this and delegate to bash without timing output.
if [[ $1 == -* ]]; then
	exec bash "$@"
fi

set -eu

target=$1
shift

bold=$(tput bold 2>/dev/null) || true
green=$(tput setaf 2 2>/dev/null) || true
red=$(tput setaf 1 2>/dev/null) || true
reset=$(tput sgr0 2>/dev/null) || true

start=$(date +%s)
echo "${bold}==> ${target}${reset}"

set +e
bash "$@"
rc=$?
set -e

elapsed=$(($(date +%s) - start))
if ((rc == 0)); then
	echo "${bold}${green}==> ${target} completed in ${elapsed}s${reset}"
else
	echo "${bold}${red}==> ${target} FAILED after ${elapsed}s${reset}" >&2
	exit $rc
fi
