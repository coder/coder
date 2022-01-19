bin/coderd:
	mkdir -p bin
	go build -o bin/coderd cmd/coderd/main.go
.PHONY: bin/coderd

build: site/out bin/coderd
.PHONY: build

# Runs migrations to output a dump of the database.
database/dump.sql: $(wildcard database/migrations/*.sql)
	go run database/dump/main.go

# Generates Go code for querying the database.
database/generate: database/dump.sql database/query.sql
	cd database && sqlc generate && rm db_tmp.go
	cd database && gofmt -w -r 'Querier -> querier' *.go
	cd database && gofmt -w -r 'Queries -> sqlQuerier' *.go
.PHONY: database/generate

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

gen: database/generate peerbroker/proto provisionersdk/proto
.PHONY: gen

# Generates the protocol files.
peerbroker/proto: peerbroker/proto/peerbroker.proto
	cd peerbroker/proto && protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-drpc_out=. \
		--go-drpc_opt=paths=source_relative \
		./peerbroker.proto
.PHONY: peerbroker/proto

# Generates the protocol files.
provisionersdk/proto: provisionersdk/proto/provisioner.proto
	cd provisionersdk/proto && protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-drpc_out=. \
		--go-drpc_opt=paths=source_relative \
		./provisioner.proto
.PHONY: provisionersdk/proto

site/out: 
	yarn build
	yarn export
.PHONY: site/out