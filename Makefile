.DEFAULT_GOAL := build

# Use a single bash shell for each job, and immediately exit on failure
SHELL := bash
.SHELLFLAGS = -ceu
.ONESHELL:

# This doesn't work on directories.
# See https://stackoverflow.com/questions/25752543/make-delete-on-error-for-directory-targets
.DELETE_ON_ERROR:

INSTALL_DIR=$(shell go env GOPATH)/bin
GOOS=$(shell go env GOOS)
GOARCH=$(shell go env GOARCH)
VERSION=$(shell ./scripts/version.sh)

bin: $(shell find . -not -path './vendor/*' -type f -name '*.go') go.mod go.sum $(shell find ./examples/templates)
	@echo "== This builds slim binaries for command-line usage."
	@echo "== Use \"make build\" to embed the site."

	mkdir -p ./dist
	rm -rf ./dist/coder-slim_*
	rm -f ./site/out/bin/coder*
	./scripts/build_go_slim.sh \
		--compress 6 \
		--version "$(VERSION)" \
		--output ./dist/ \
		linux:amd64,armv7,arm64 \
		windows:amd64,arm64 \
		darwin:amd64,arm64
.PHONY: bin

build: site/out/index.html $(shell find . -not -path './vendor/*' -type f -name '*.go') go.mod go.sum $(shell find ./examples/templates)
	rm -rf ./dist
	mkdir -p ./dist
	rm -f ./site/out/bin/coder*

	# build slim artifacts and copy them to the site output directory
	./scripts/build_go_slim.sh \
		--version "$(VERSION)" \
		--compress 6 \
		--output ./dist/ \
		linux:amd64,armv7,arm64 \
		windows:amd64,arm64 \
		darwin:amd64,arm64

	# build not-so-slim artifacts with the default name format
	./scripts/build_go_matrix.sh \
		--version "$(VERSION)" \
		--output ./dist/ \
		--archive \
		--package-linux \
		linux:amd64,armv7,arm64 \
		windows:amd64,arm64 \
		darwin:amd64,arm64
.PHONY: build

# Runs migrations to output a dump of the database.
coderd/database/dump.sql: $(wildcard coderd/database/migrations/*.sql)
	go run coderd/database/dump/main.go

# Generates Go code for querying the database.
coderd/database/querier.go: coderd/database/sqlc.yaml coderd/database/dump.sql $(wildcard coderd/database/queries/*.sql)
	coderd/database/generate.sh

# This target is deprecated, as GNU make has issues passing signals to subprocesses.
dev:
	@echo Please run ./scripts/develop.sh manually.
.PHONY: dev

fmt/prettier:
	@echo "--- prettier"
	cd site
# Avoid writing files in CI to reduce file write activity
ifdef CI
	yarn run format:check
else
	yarn run format:write
endif
.PHONY: fmt/prettier

fmt/terraform: $(wildcard *.tf)
	terraform fmt -recursive
.PHONY: fmt/terraform

fmt/shfmt: $(shell shfmt -f .)
	@echo "--- shfmt"
# Only do diff check in CI, errors on diff.
ifdef CI
	shfmt -d $(shell shfmt -f .)
else
	shfmt -w $(shell shfmt -f .)
endif
.PHONY: fmt/shfmt

fmt: fmt/prettier fmt/terraform fmt/shfmt
.PHONY: fmt

gen: coderd/database/querier.go peerbroker/proto/peerbroker.pb.go provisionersdk/proto/provisioner.pb.go provisionerd/proto/provisionerd.pb.go site/src/api/typesGenerated.ts
.PHONY: gen

install: site/out/index.html $(shell find . -not -path './vendor/*' -type f -name '*.go') go.mod go.sum $(shell find ./examples/templates)
	@output_file="$(INSTALL_DIR)/coder"

	@if [[ "$(GOOS)" == "windows" ]]; then
		@output_file="$${output_file}.exe"
	@fi

	@echo "-- Building CLI for $(GOOS) $(GOARCH) at $$output_file"

	./scripts/build_go.sh \
		--version "$(VERSION)" \
		--output "$$output_file" \
		--os "$(GOOS)" \
		--arch "$(GOARCH)"

	@echo
.PHONY: install

lint: lint/shellcheck lint/go
.PHONY: lint

lint/go:
	golangci-lint run
.PHONY: lint/go

# Use shfmt to determine the shell files, takes editorconfig into consideration.
lint/shellcheck: $(shell shfmt -f .)
	@echo "--- shellcheck"
	shellcheck --external-sources $(shell shfmt -f .)
.PHONY: lint/shellcheck

peerbroker/proto/peerbroker.pb.go: peerbroker/proto/peerbroker.proto
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-drpc_out=. \
		--go-drpc_opt=paths=source_relative \
		./peerbroker/proto/peerbroker.proto

provisionerd/proto/provisionerd.pb.go: provisionerd/proto/provisionerd.proto
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-drpc_out=. \
		--go-drpc_opt=paths=source_relative \
		./provisionerd/proto/provisionerd.proto

provisionersdk/proto/provisioner.pb.go: provisionersdk/proto/provisioner.proto
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-drpc_out=. \
		--go-drpc_opt=paths=source_relative \
		./provisionersdk/proto/provisioner.proto

site/out/index.html: $(shell find ./site -not -path './site/node_modules/*' -type f -name '*.tsx') $(shell find ./site -not -path './site/node_modules/*' -type f -name '*.ts') site/package.json
	./scripts/yarn_install.sh
	cd site
	yarn typegen
	yarn build
	# Restores GITKEEP files!
	git checkout HEAD out

site/src/api/typesGenerated.ts: scripts/apitypings/main.go $(shell find codersdk -type f -name '*.go')
	go run scripts/apitypings/main.go > site/src/api/typesGenerated.ts
	cd site
	yarn run format:types

test: test-clean
	gotestsum -- -v -short ./...
.PHONY: test

# When updating -timeout for this test, keep in sync with
# test-go-postgres (.github/workflows/coder.yaml).
test-postgres: test-clean test-postgres-docker
	DB=ci DB_FROM=$(shell go run scripts/migrate-ci/main.go) gotestsum --junitfile="gotests.xml" --packages="./..." -- \
		-covermode=atomic -coverprofile="gotests.coverage" -timeout=20m \
		-coverpkg=./... \
		-count=1 -race -failfast
.PHONY: test-postgres

test-postgres-docker:
	docker rm -f test-postgres-docker || true
	docker run \
		--env POSTGRES_PASSWORD=postgres \
		--env POSTGRES_USER=postgres \
		--env POSTGRES_DB=postgres \
		--env PGDATA=/tmp \
		--tmpfs /tmp \
		--publish 5432:5432 \
		--name test-postgres-docker \
		--restart no \
		--detach \
		postgres:13 \
		-c shared_buffers=1GB \
		-c max_connections=1000 \
		-c fsync=off \
		-c synchronous_commit=off \
		-c full_page_writes=off
	while ! pg_isready -h 127.0.0.1
	do
		echo "$(date) - waiting for database to start"
		sleep 0.5
	done
.PHONY: test-postgres-docker

test-clean:
	go clean -testcache
.PHONY: test-clean
