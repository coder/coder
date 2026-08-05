#!/bin/sh
# Parent-side fixtures for the sandbox boundary probes.
#
# This runs in the workspace container, on the parent side of the boundary,
# before the parent agent starts. It creates harmless markers that stand in
# for the private files a real workspace holds, so the probes inside the
# sandbox can show which of them the child can reach.
#
# Every marker is a plain text file with no secret in it. The ones outside
# the shared project directory must not be visible to the child; the one
# inside it must be.

set -eu

parent_home=${PARENT_HOME:-${HOME:-/home/coder}}
project_dir=${PROJECT_DIR:-/home/coder/project}

# A dotfile, an ssh-style key location, and a private directory: the three
# shapes of parent-only state the probes look for.
printf 'parent-only dotfile marker, not a secret\n' >"$parent_home/.parent-dotfile-marker"

mkdir -p "$parent_home/.ssh"
chmod 700 "$parent_home/.ssh"
printf 'parent-only ssh marker, not a real key\n' >"$parent_home/.ssh/parent-ssh-marker"
chmod 600 "$parent_home/.ssh/parent-ssh-marker"

mkdir -p "$parent_home/parent-private"
chmod 700 "$parent_home/parent-private"
printf 'parent-only private marker, not a secret\n' >"$parent_home/parent-private/parent-private-marker.txt"

# The one marker that is meant to cross the boundary, because the project
# directory is the single declared shared path.
mkdir -p "$project_dir"
printf 'parent wrote this into the shared project directory\n' >"$project_dir/parent-shared-marker.txt"
