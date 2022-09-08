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

	# Dump the updated schema.
	go run gen/dump/main.go
	# The logic below depends on the exact version being correct :(
	go run github.com/kyleconroy/sqlc/cmd/sqlc@v1.13.0 generate

	first=true
	for fi in queries/*.sql.go; do
		# Find the last line from the imports section and add 1.
		cut=$(grep -n ')' "$fi" | head -n 1 | cut -d: -f1)
		cut=$((cut + 1))

		# Copy the header from the first file only, ignoring the source comment.
		if $first; then
			head -n 6 <"$fi" | grep -v "source" >queries.sql.go
			first=false
		fi

		# Append the file past the imports section into queries.sql.go.
		tail -n "+$cut" <"$fi" >>queries.sql.go
	done

	# Move the files we want.
	mv queries/querier.go .
	mv queries/models.go .

	# Remove temporary go files.
	rm -f queries/*.go

	# Fix struct/interface names.
	gofmt -w -r 'Querier -> querier' -- *.go
	gofmt -w -r 'Queries -> sqlQuerier' -- *.go

	# Ensure correct imports exist. Modules must all be downloaded so we get correct
	# suggestions.
	go mod download
	go run golang.org/x/tools/cmd/goimports@latest -w queries.sql.go

	# Generate enums (e.g. unique constraints).
	go run gen/enum/main.go
)
