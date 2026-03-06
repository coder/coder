#!/usr/bin/env bash

# This script turns many *.sql.go files into a single queries.sql.go file. This
# is due to sqlc's behavior when using multiple sql files to output them to
# multiple Go files. We decided it would be cleaner to move these to a single
# file for readability. We should probably contribute the option to do this
# upstream instead, because this is quite janky.

set -euo pipefail

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")

(
	cd "$SCRIPT_DIR"

	echo generate 1>&2

	# Dump the updated schema (use make to utilize caching).
	if [[ "${SKIP_DUMP_SQL:-0}" != 1 ]]; then
		make -C ../.. --no-print-directory coderd/database/dump.sql
	fi
	# The logic below depends on the exact version being correct :(
	sqlc generate

	# Work directory for formatting before atomic replacement of
	# generated files, ensuring the source tree is never left in a
	# partially written state.
	mkdir -p ../../_gen
	workdir=$(mktemp -d ../../_gen/.dbgen.XXXXXX)
	trap 'rm -rf "$workdir"' EXIT

	first=true
	files=$(find ./queries/ -type f -name "*.sql.go" | LC_ALL=C sort)
	for fi in $files; do
		# Find the last line from the imports section and add 1. We have to
		# disable pipefail temporarily to avoid ERRPIPE errors when piping into
		# `head -n1`.
		set +o pipefail
		cut=$(grep -n ')' "$fi" | head -n 1 | cut -d: -f1)
		set -o pipefail
		cut=$((cut + 1))

		# Copy the header from the first file only, ignoring the source comment.
		if $first; then
			head -n 6 <"$fi" | grep -v "source" >"$workdir/queries.sql.go"
			first=false
		fi

		# Append the file past the imports section into queries.sql.go.
		tail -n "+$cut" <"$fi" >>"$workdir/queries.sql.go"
	done

	# Move sqlc outputs into workdir for formatting.
	mv queries/querier.go "$workdir/querier.go"
	mv queries/models.go "$workdir/models.go"

	# Remove temporary go files.
	rm -f queries/*.go

	# Fix struct/interface names in the workdir (not the source tree).
	gofmt -w -r 'Querier -> sqlcQuerier' -- "$workdir"/*.go
	gofmt -w -r 'Queries -> sqlQuerier' -- "$workdir"/*.go

	# Ensure correct imports exist. Modules must all be downloaded so we
	# get correct suggestions.
	go mod download
	go tool golang.org/x/tools/cmd/goimports -w "$workdir/queries.sql.go"

	# Atomically replace all three target files.
	mv "$workdir/queries.sql.go" queries.sql.go
	mv "$workdir/querier.go" querier.go
	mv "$workdir/models.go" models.go

	go run ../../scripts/dbgen
	# This will error if a view is broken. This is in it's own package to avoid
	# stuff like dbmock breaking builds if it's out of date from the interface.
	go test ./gentest
)
