# Runs migrations to output a dump of the database.
database/dump.sql: $(wildcard database/migrations/*.sql)
	go run database/dump/main.go

# Generates Go code for querying the database.
.PHONY: database/generate
database/generate: database/dump.sql database/query.sql
	cd database && sqlc generate && rm db_tmp.go
	cd database && gofmt -w -r 'Querier -> querier' *.go
	cd database && gofmt -w -r 'Queries -> sqlQuerier' *.go

fmt/prettier:
	@echo "--- prettier"
# Avoid writing files in CI to reduce file write activity
ifdef CI
	yarn run format:check
else
	yarn run format:write
endif
.PHONY: fmt/prettier

fmt: fmt/prettier
.PHONY: fmt
