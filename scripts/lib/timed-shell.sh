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
# When MAKE_LOGDIR is set, recipe output is captured to a log file.
# Otherwise output goes to stdout/stderr as normal.
#
# $(shell ...) uses SHELL but passes -c directly, not .SHELLFLAGS.
# Detect this and delegate to bash without timing output.
if [[ $1 == -* ]]; then
	exec bash "$@"
fi

set -eu

target=$1
shift

dim=$(tput dim 2>/dev/null) || dim=$(tput setaf 8 2>/dev/null) || true
green=$(tput setaf 2 2>/dev/null) || true
red=$(tput setaf 1 2>/dev/null) || true
reset=$(tput sgr0 2>/dev/null) || true

start=$(date +%s)

set +e
if [[ -n ${MAKE_LOGDIR:-} ]]; then
	logfile="${MAKE_LOGDIR}/${target//\//-}.log"
	bash "$@" >"$logfile" 2>&1
else
	printf '%s○%s %s\n' "$dim" "$reset" "$target"
	bash "$@"
fi
rc=$?
set -e

elapsed=$(($(date +%s) - start))
if ((rc == 0)); then
	printf '%s✓%s %s (%ds)\n' "$green" "$reset" "$target" "$elapsed"
else
	if [[ -n ${MAKE_LOGDIR:-} ]]; then
		printf '%s○%s %s\n' "$dim" "$reset" "$target"
		tail -n20 "$logfile" | sed 's/^/    /'
		printf '%s✗%s %s (%ds) → %s\n' "$red" "$reset" "$target" "$elapsed" "$logfile"
	else
		printf '%s✗%s %s (%ds)\n' "$red" "$reset" "$target" "$elapsed"
	fi
	exit "$rc"
fi
