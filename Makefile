.DEFAULT_GOAL := build

# Use a single bash shell for each job, and immediately exit on failure
SHELL := bash
.SHELLFLAGS = -ceu
.ONESHELL:

# This doesn't work on directories.
# See https://stackoverflow.com/questions/25752543/make-delete-on-error-for-directory-targets
.DELETE_ON_ERROR:

INSTALL_DIR = $(shell go env GOPATH)/bin
GOOS = $(shell go env GOOS)
GOARCH = $(shell go env GOARCH)
VERSION = $(shell ./scripts/version.sh)

# All ${OS}_${ARCH} combos we build for. Windows binaries have the .exe suffix.
OS_ARCHES = \
	linux_amd64 linux_arm64 linux_armv7 \
	darwin_amd64 darwin_arm64 \
	windows_amd64.exe windows_arm64.exe

# Archive formats and their corresponding ${OS}_${ARCH} combos.
ARCHIVE_TAR_GZ = linux_amd64 linux_arm64 linux_armv7
ARCHIVE_ZIP = \
	darwin darwin_arm64 \
	windows_amd64 windows_arm64

# All ${OS}_${ARCH} combos we build packages for.
PACKAGE_OS_ARCHES = linux_amd64 linux_armv7 linux_arm64

# All package formats we build.
PACKAGE_FORMATS = apk deb rpm

bin build-slim: $(addprefix build/coder-slim_$(VERSION)_,$(OS_ARCHES))
.PHONY: bin build-slim

build build-full: $(addprefix build/coder_$(VERSION)_,$(OS_ARCHES))
.PHONY: build build-full

# Redirect from version-less targets to the versioned ones. This is kinda gross
# since it's make shelling out to make, but it's the easiest way less we write
# out every target manually.
$(addprefix build/coder_,$(OS_ARCHES)): site/out/index.html build-slim
	@target="coder_$(VERSION)_$(@:build/coder_%=%)"
	@$(MAKE) \
		--no-print-directory \
		--assume-old site/out/index.html \
		--assume-old build-slim \
		"build/$$target"
	@rm -f "$@"
	@ln -s "$$target" "$@"
.PHONY: $(addprefix build/coder_,$(OS_ARCHES))

$(addprefix build/coder-slim_,$(OS_ARCHES)):
	@target="coder-slim_$(VERSION)_$(@:build/coder-slim_%=%)"
	@echo $(MAKE) \
		--no-print-directory \
		"build/$$target"
	@rm -f "$@"
	@ln -s "$$target" "$@"
.PHONY: $(addprefix build/coder-slim_,$(OS_ARCHES))

# "full" binaries always depend on all "slim" binaries.
$(addprefix build/coder_$(VERSION)_,$(OS_ARCHES)): site/out/index.html build-slim

# This task handles all builds, for both "full" and "slim" binaries. It parses
# the target name to get the metadata for the build, so it must be specified in
# this format:
#     build/coder(-slim)?_$version_$os_$arch(.exe)?
$(addprefix build/coder_$(VERSION)_,$(OS_ARCHES)) $(addprefix build/coder-slim_$(VERSION)_,$(OS_ARCHES)): \
	go.mod go.sum \
	$(shell find . -not -path './vendor/*' -type f -name '*.go') \
	$(shell find ./examples/templates)

	@mkdir -p build
	@mode=$$([[ "$@" = build/coder-slim* ]] && echo "slim" || echo "full")
	@os=$$(echo $@ | cut -d_ -f3)
	@arch=$$(echo $@ | cut -d_ -f4)
	@if [ "$$mode" != "full" ] && [ "$$mode" != "slim" ]; then
		@echo "Invalid build mode: $$mode"
		@exit 1
	@fi
	@if [[ "$$os" == "windows" ]] && [[ "$$arch" == *.exe ]]; then
		@arch=$${arch%.exe}
	@fi

	@build_args=( \
		--os "$$os" \
		--arch "$$arch" \
		--version "$(VERSION)" \
		--output "$@" \
	)
	@if [ "$$mode" == "slim" ]; then
		@build_args+=(--slim)
	@fi

	./scripts/build_go.sh "$${build_args[@]}"

# This task builds all archives. It parses the target name to get the metadata
# for the build, so it must be specified in this format:
#     build/coder_${version}_${os}_${arch}.${format}
#
# The following OS/arch/format combinations are supported:
#     .tar.gz: linux_amd64, linux_arm64, linux_armv7
#     .zip:    darwin_amd64, darwin_arm64, windows_amd64, windows_arm64
#
# This depends on build-full because it's difficult to do dynamic dependencies.
$(foreach os_arch, $(ARCHIVE_TAR_GZ), build/coder_$(VERSION)_$(os_arch).tar.gz) \
$(foreach os_arch, $(ARCHIVE_ZIP), build/coder_$(VERSION)_$(os_arch).zip): \
	build-full

	@mkdir -p build
	@os=$$(echo $@ | cut -d_ -f3)
	@arch_format=$$(echo $@ | cut -d_ -f4)
	@arch=$$(echo $$arch_format | cut -d. -f1)
	@format=$${arch_format#*.}

	@bin_ext=""
	@if [[ "$$os" == "windows" ]]; then
		@bin_ext=".exe"
	@fi

	./script/archive.sh \
		--format "$$format" \
		--output "$@" \
		"build/coder_$(VERSION)_$${os}_$${arch}$${bin_ext}"

# This task builds all packages. It parses the target name to get the metadata
# for the build, so it must be specified in this format:
#     build/coder_${version}_${os}_${arch}.${format}
#
# Supports apk, deb, rpm for all linux targets.
#
# This depends on build-full because it's difficult to do dynamic dependencies.
$(foreach os_arch, $(PACKAGE_OS_ARCHES), $(addprefix build/coder_$(VERSION)_$(os_arch).,$(PACKAGE_FORMATS))): \
	build-full

	@mkdir -p build
	@os=$$(echo $@ | cut -d_ -f3)
	@arch_format=$$(echo $@ | cut -d_ -f4)
	@arch=$$(echo $$arch_format | cut -d. -f1)
	@format=$${arch_format#*.}

	./scripts/package.sh \
		--arch "$$arch" \
		--format "$$format" \
		--version "$(VERSION)" \
		--output "$@" \
		"build/coder_$(VERSION)_$${os}_$${arch}"

clean:
	rm -rf build
.PHONY: clean

# Runs migrations to output a dump of the database.
coderd/database/dump.sql: coderd/database/dump/main.go $(wildcard coderd/database/migrations/*.sql)
	go run coderd/database/dump/main.go

# Generates Go code for querying the database.
coderd/database/querier.go: coderd/database/sqlc.yaml coderd/database/dump.sql $(wildcard coderd/database/queries/*.sql)
	coderd/database/generate.sh

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
	./scripts/check_enterprise_imports.sh
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
