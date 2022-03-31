#!/usr/bin/env bash

set -euo pipefail

cd "$(dirname "$0")"

sqlc generate

first=true
for fi in queries/*.sql.go; do
    # Find the last line from the imports section and add 1.
    cut=$(grep -n ')' "$fi" | head -n 1 | cut -d: -f1)
    cut=$((cut + 1))

    # Copy the header from the first file only, ignoring the source comment.
    if $first; then
        head -n 4 < "$fi" | grep -v "source" > queries.sql.go
        first=false
    fi

    # Append the file past the imports section into queries.sql.go.
    tail -n "+$cut" < "$fi" >> queries.sql.go
done

# Remove temporary go files.
rm -f queries/*.go

go mod tidy
# Ensure correct imports exist.
goimports -w queries.sql.go

# Fix struct/interface names.
gofmt -w -r 'Querier -> querier' -- *.go
gofmt -w -r 'Queries -> sqlQuerier' -- *.go
