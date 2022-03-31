#!/usr/bin/env bash

set -euo pipefail

cd "$(dirname "$0")"

sqlc generate

first=true
for fi in queries/*.sql.go; do
    cut=$(grep -n ')' "$fi" | head -n 1 | cut -d: -f1)
    cut=$((cut + 1))

    if $first; then
        head -n 4 < "$fi" | grep -v "source" > queries.sql.go
        first=false
    fi

    tail -n "+$cut" < "$fi" >> queries.sql.go
done

rm -f queries/*.go

goimports -w queries.sql.go
gofmt -w -r 'Querier -> querier' -- *.go
gofmt -w -r 'Queries -> sqlQuerier' -- *.go
