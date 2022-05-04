.DEFAULT_GOAL := build

INSTALL_DIR=$(shell go env GOPATH)/bin
GOOS=$(shell go env GOOS)
GOARCH=$(shell go env GOARCH)

bin: $(shell find . -not -path './vendor/*' -type f -name '*.go') go.mod go.sum
	@echo "== This builds binaries for command-line usage."
	@echo "== Use \"make build\" to embed the site."
	goreleaser build --snapshot --rm-dist --single-target

build: dist/artifacts.json
.PHONY: build

# Runs migrations to output a dump of the database.
coderd/database/dump.sql: $(wildcard coderd/database/migrations/*.sql)
	go run coderd/database/dump/main.go

# Generates Go code for querying the database.
coderd/database/querier.go: coderd/database/dump.sql $(wildcard coderd/database/queries/*.sql)
	coderd/database/generate.sh

dist/artifacts.json: site/out/index.html $(shell find . -not -path './vendor/*' -type f -name '*.go') go.mod go.sum
	goreleaser release --snapshot --rm-dist --skip-sign

fmt/prettier:
	@echo "--- prettier"
# Avoid writing files in CI to reduce file write activity
ifdef CI
	cd site && yarn run format:check
else
	cd site && yarn run format:write
endif
.PHONY: fmt/prettier

fmt/terraform: $(wildcard *.tf)
	terraform fmt -recursive
.PHONY: fmt/terraform

fmt: fmt/prettier fmt/terraform
.PHONY: fmt

gen: coderd/database/querier.go peerbroker/proto/peerbroker.pb.go provisionersdk/proto/provisioner.pb.go provisionerd/proto/provisionerd.pb.go site/src/api/typesGenerated.ts

install: build
	@echo "--- Copying from bin to $(INSTALL_DIR)"
	cp -r ./dist/coder-$(GOOS)_$(GOOS)_$(GOARCH)*/* $(INSTALL_DIR)
	@echo "-- CLI available at $(shell ls $(INSTALL_DIR)/coder*)"
.PHONY: install

lint:
	golangci-lint run
.PHONY: lint

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
	cd site && yarn typegen
	cd site && yarn build
	# Restores GITKEEP files!
	git checkout HEAD site/out

site/src/api/typesGenerated.ts: $(shell find codersdk -type f -name '*.go')
	go run scripts/apitypings/main.go > site/src/api/typesGenerated.ts
	cd site && yarn run format:types

test:
	gotestsum -- -v -short ./...
