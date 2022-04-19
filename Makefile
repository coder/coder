INSTALL_DIR=$(shell go env GOPATH)/bin
GOOS=$(shell go env GOOS)
GOARCH=$(shell go env GOARCH)

bin:
	goreleaser build --snapshot --rm-dist
.PHONY: bin

build: site/out bin
.PHONY: build

# Runs migrations to output a dump of the database.
coderd/database/dump.sql: $(wildcard coderd/database/migrations/*.sql)
	go run coderd/database/dump/main.go
.PHONY: coderd/database/dump.sql

# Generates Go code for querying the database.
coderd/database/generate: fmt/sql coderd/database/dump.sql $(wildcard coderd/database/queries/*.sql)
	coderd/database/generate.sh
.PHONY: coderd/database/generate

apitypings/generate: site/src/api/types.ts
	go run scripts/apitypings/main.go > site/src/api/types-generated.ts
	cd site && yarn run format:types
.PHONY: apitypings/generate

fmt/prettier:
	@echo "--- prettier"
# Avoid writing files in CI to reduce file write activity
ifdef CI
	cd site && yarn run format:check
else
	cd site && yarn run format:write
endif
.PHONY: fmt/prettier

fmt/sql: $(wildcard coderd/database/queries/*.sql)
	for fi in coderd/database/queries/*.sql; do \
		npx sql-formatter \
			--language postgresql \
			--lines-between-queries 2 \
			--tab-indent \
			$$fi \
			--output $$fi; \
	done

	sed -i 's/@ /@/g' ./coderd/database/queries/*.sql

fmt/terraform: $(wildcard *.tf)
	terraform fmt -recursive

fmt: fmt/prettier fmt/sql fmt/terraform
.PHONY: fmt

gen: coderd/database/generate peerbroker/proto provisionersdk/proto provisionerd/proto apitypings/generate
.PHONY: gen

install: bin
	@echo "--- Copying from bin to $(INSTALL_DIR)"
	cp -r ./dist/coder_$(GOOS)_$(GOARCH)/* $(INSTALL_DIR)
	@echo "-- CLI available at $(shell ls $(INSTALL_DIR)/coder*)"
.PHONY: install

lint:
	golangci-lint run
.PHONY: lint

peerbroker/proto: peerbroker/proto/peerbroker.proto
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-drpc_out=. \
		--go-drpc_opt=paths=source_relative \
		./peerbroker/proto/peerbroker.proto
.PHONY: peerbroker/proto

provisionerd/proto: provisionerd/proto/provisionerd.proto
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-drpc_out=. \
		--go-drpc_opt=paths=source_relative \
		./provisionerd/proto/provisionerd.proto
.PHONY: provisionerd/proto

provisionersdk/proto: provisionersdk/proto/provisioner.proto
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-drpc_out=. \
		--go-drpc_opt=paths=source_relative \
		./provisionersdk/proto/provisioner.proto
.PHONY: provisionersdk/proto

release:
	goreleaser release --snapshot --rm-dist --skip-sign
.PHONY: release

site/out:
	./scripts/yarn_install.sh
	cd site && yarn typegen
	cd site && yarn build
	# Restores GITKEEP files!
	git checkout HEAD site/out
.PHONY: site/out

test:
	gotestsum -- -v -short ./...
