# This is the Coder Makefile. The build directory for most tasks is `build/`.
#
# These are the targets you're probably looking for:
# - clean
# - build-fat: builds all "fat" binaries for all architectures
# - build-slim: builds all "slim" binaries (no frontend or slim binaries
#   embedded) for all architectures
# - release: simulate a release (mostly, does not push images)
# - build/coder(-slim)?_${os}_${arch}(.exe)?: build a single fat binary
# - build/coder_${os}_${arch}.(zip|tar.gz): build a release archive
# - build/coder_linux_${arch}.(apk|deb|rpm): build a release Linux package
# - build/coder_${version}_linux_${arch}.tag: build a release Linux Docker image
# - build/coder_helm.tgz: build a release Helm chart

.DEFAULT_GOAL := build-fat

# Use a single bash shell for each job, and immediately exit on failure
SHELL := bash
.SHELLFLAGS := -ceu
.ONESHELL:

# This doesn't work on directories.
# See https://stackoverflow.com/questions/25752543/make-delete-on-error-for-directory-targets
.DELETE_ON_ERROR:

# Don't print the commands in the file unless you specify VERBOSE. This is
# essentially the same as putting "@" at the start of each line.
ifndef VERBOSE
.SILENT:
endif

# Create the output directories if they do not exist.
$(shell mkdir -p build site/out/bin)

GOOS         := $(shell go env GOOS)
GOARCH       := $(shell go env GOARCH)
GOOS_BIN_EXT := $(if $(filter windows, $(GOOS)),.exe,)
VERSION      := $(shell ./scripts/version.sh)

# Use the highest ZSTD compression level in CI.
ifdef CI
ZSTDFLAGS := -22 --ultra
else
ZSTDFLAGS := -6
endif

# Common paths to exclude from find commands, this rule is written so
# that it can be it can be used in a chain of AND statements (meaning
# you can simply write `find . $(FIND_EXCLUSIONS) -name thing-i-want`).
# Note, all find statements should be written with `.` or `./path` as
# the search path so that these exclusions match.
FIND_EXCLUSIONS= \
	-not \( \( -path '*/.git/*' -o -path './build/*' -o -path './vendor/*' -o -path './.coderv2/*' -o -path '*/node_modules/*' -o -path './site/out/*' \) -prune \)
# Source files used for make targets, evaluated on use.
GO_SRC_FILES = $(shell find . $(FIND_EXCLUSIONS) -type f -name '*.go')
# All the shell files in the repo, excluding ignored files.
SHELL_SRC_FILES = $(shell find . $(FIND_EXCLUSIONS) -type f -name '*.sh')

# All ${OS}_${ARCH} combos we build for. Windows binaries have the .exe suffix.
OS_ARCHES := \
	linux_amd64 linux_arm64 linux_armv7 \
	darwin_amd64 darwin_arm64 \
	windows_amd64.exe windows_arm64.exe

# Archive formats and their corresponding ${OS}_${ARCH} combos.
ARCHIVE_TAR_GZ := linux_amd64 linux_arm64 linux_armv7
ARCHIVE_ZIP    := \
	darwin_amd64 darwin_arm64 \
	windows_amd64 windows_arm64

# All package formats we build and the ${OS}_${ARCH} combos we build them for.
PACKAGE_FORMATS   := apk deb rpm
PACKAGE_OS_ARCHES := linux_amd64 linux_armv7 linux_arm64

# All architectures we build Docker images for (Linux only).
DOCKER_ARCHES := amd64 arm64 armv7

# Computed variables based on the above.
CODER_SLIM_BINARIES      := $(addprefix build/coder-slim_$(VERSION)_,$(OS_ARCHES))
CODER_FAT_BINARIES       := $(addprefix build/coder_$(VERSION)_,$(OS_ARCHES))
CODER_ALL_BINARIES       := $(CODER_SLIM_BINARIES) $(CODER_FAT_BINARIES)
CODER_TAR_GZ_ARCHIVES    := $(foreach os_arch, $(ARCHIVE_TAR_GZ), build/coder_$(VERSION)_$(os_arch).tar.gz)
CODER_ZIP_ARCHIVES       := $(foreach os_arch, $(ARCHIVE_ZIP), build/coder_$(VERSION)_$(os_arch).zip)
CODER_ALL_ARCHIVES       := $(CODER_TAR_GZ_ARCHIVES) $(CODER_ZIP_ARCHIVES)
CODER_ALL_PACKAGES       := $(foreach os_arch, $(PACKAGE_OS_ARCHES), $(addprefix build/coder_$(VERSION)_$(os_arch).,$(PACKAGE_FORMATS)))
CODER_ARCH_IMAGES        := $(foreach arch, $(DOCKER_ARCHES), build/coder_$(VERSION)_linux_$(arch).tag)
CODER_ARCH_IMAGES_PUSHED := $(addprefix push/, $(CODER_ARCH_IMAGES))
CODER_MAIN_IMAGE         := build/coder_$(VERSION)_linux.tag

CODER_SLIM_NOVERSION_BINARIES     := $(addprefix build/coder-slim_,$(OS_ARCHES))
CODER_FAT_NOVERSION_BINARIES      := $(addprefix build/coder_,$(OS_ARCHES))
CODER_ALL_NOVERSION_IMAGES        := $(foreach arch, $(DOCKER_ARCHES), build/coder_linux_$(arch).tag) build/coder_linux.tag
CODER_ALL_NOVERSION_IMAGES_PUSHED := $(addprefix push/, $(CODER_ALL_NOVERSION_IMAGES))

# If callers are only building Docker images and not the packages and archives,
# we can skip those prerequisites as they are not actually required and only
# specified to avoid concurrent write failures.
ifdef DOCKER_IMAGE_NO_PREREQUISITES
CODER_ARCH_IMAGE_PREREQUISITES :=
else
CODER_ARCH_IMAGE_PREREQUISITES := \
	build/coder_$(VERSION)_%.apk \
	build/coder_$(VERSION)_%.deb \
	build/coder_$(VERSION)_%.rpm \
	build/coder_$(VERSION)_%.tar.gz
endif


clean:
	rm -rf build site/out
	mkdir -p build site/out/bin
	git restore site/out
.PHONY: clean

build-slim: $(CODER_SLIM_BINARIES)
.PHONY: build-slim

build-fat build-full build: $(CODER_FAT_BINARIES)
.PHONY: build-fat build-full build

release: $(CODER_FAT_BINARIES) $(CODER_ALL_ARCHIVES) $(CODER_ALL_PACKAGES) $(CODER_ARCH_IMAGES) build/coder_helm_$(VERSION).tgz
.PHONY: release

build/coder-slim_$(VERSION)_checksums.sha1: site/out/bin/coder.sha1
	cp "$<" "$@"

site/out/bin/coder.sha1: $(CODER_SLIM_BINARIES)
	pushd ./site/out/bin
		openssl dgst -r -sha1 coder-* | tee coder.sha1
	popd

build/coder-slim_$(VERSION).tar: build/coder-slim_$(VERSION)_checksums.sha1 $(CODER_SLIM_BINARIES)
	pushd ./site/out/bin
		tar cf "../../../build/$(@F)" coder-*
	popd

	# delete the uncompressed binaries from the embedded dir
	rm -f site/out/bin/coder-*

site/out/bin/coder.tar.zst: build/coder-slim_$(VERSION).tar.zst
	cp "$<" "$@"

build/coder-slim_$(VERSION).tar.zst: build/coder-slim_$(VERSION).tar
	zstd $(ZSTDFLAGS) \
		--force \
		--long \
		--no-progress \
		-o "build/coder-slim_$(VERSION).tar.zst" \
		"build/coder-slim_$(VERSION).tar"

# Redirect from version-less targets to the versioned ones. There is a similar
# target for slim binaries below.
#
# Called like this:
#   make build/coder_linux_amd64
#   make build/coder_windows_amd64.exe
$(CODER_FAT_NOVERSION_BINARIES): build/coder_%: build/coder_$(VERSION)_%
	rm -f "$@"
	ln "$<" "$@"

# Same as above, but for slim binaries.
#
# Called like this:
#   make build/coder-slim_linux_amd64
#   make build/coder-slim_windows_amd64.exe
$(CODER_SLIM_NOVERSION_BINARIES): build/coder-slim_%: build/coder-slim_$(VERSION)_%
	rm -f "$@"
	ln "$<" "$@"

# "fat" binaries always depend on the site and the compressed slim binaries.
$(CODER_FAT_BINARIES): \
	site/out/index.html \
	site/out/bin/coder.sha1 \
	site/out/bin/coder.tar.zst

# This is a handy block that parses the target to determine whether it's "slim"
# or "fat", which OS was specified and which architecture was specified.
#
# It populates the following variables: mode, os, arch_ext, arch, ext (without
# dot).
define get-mode-os-arch-ext =
	mode="$$([[ "$@" = build/coder-slim* ]] && echo "slim" || echo "fat")"
	os="$$(echo $@ | cut -d_ -f3)"
	arch_ext="$$(echo $@ | cut -d_ -f4)"
	if [[ "$$arch_ext" == *.* ]]; then
		arch="$$(echo $$arch_ext | cut -d. -f1)"
		ext="$${arch_ext#*.}"
	else
		arch="$$arch_ext"
		ext=""
	fi
endef

# This task handles all builds, for both "fat" and "slim" binaries. It parses
# the target name to get the metadata for the build, so it must be specified in
# this format:
#     build/coder(-slim)?_${version}_${os}_${arch}(.exe)?
#
# You should probably use the non-version targets above instead if you're
# calling this manually.
$(CODER_ALL_BINARIES): go.mod go.sum \
	$(GO_SRC_FILES) \
	$(shell find ./examples/templates)

	$(get-mode-os-arch-ext)
	if [[ "$$os" != "windows" ]] && [[ "$$ext" != "" ]]; then
		echo "ERROR: Invalid build binary extension" 1>&2
		exit 1
	fi
	if [[ "$$os" == "windows" ]] && [[ "$$ext" != exe ]]; then
		echo "ERROR: Windows binaries must have an .exe extension." 1>&2
		exit 1
	fi

	build_args=( \
		--os "$$os" \
		--arch "$$arch" \
		--version "$(VERSION)" \
		--output "$@" \
	)
	if [ "$$mode" == "slim" ]; then
		build_args+=(--slim)
	fi

	./scripts/build_go.sh "$${build_args[@]}"

	if [[ "$$mode" == "slim" ]]; then
		dot_ext=""
		if [[ "$$ext" != "" ]]; then
			dot_ext=".$$ext"
		fi

		cp "$@" "./site/out/bin/coder-$$os-$$arch$$dot_ext"
	fi

# This task builds all archives. It parses the target name to get the metadata
# for the build, so it must be specified in this format:
#     build/coder_${version}_${os}_${arch}.${format}
#
# The following OS/arch/format combinations are supported:
#     .tar.gz: linux_amd64, linux_arm64, linux_armv7
#     .zip:    darwin_amd64, darwin_arm64, windows_amd64, windows_arm64
#
# This depends on all fat binaries because it's difficult to do dynamic
# dependencies due to the .exe requirement on Windows. These targets are
# typically only used during release anyways.
$(CODER_ALL_ARCHIVES): $(CODER_FAT_BINARIES)
	$(get-mode-os-arch-ext)
	bin_ext=""
	if [[ "$$os" == "windows" ]]; then
		bin_ext=".exe"
	fi

	./scripts/archive.sh \
		--format "$$ext" \
		--os "$$os" \
		--output "$@" \
		"build/coder_$(VERSION)_$${os}_$${arch}$${bin_ext}"

# This task builds all packages. It parses the target name to get the metadata
# for the build, so it must be specified in this format:
#     build/coder_${version}_linux_${arch}.${format}
#
# Supports apk, deb, rpm for all linux targets.
#
# This depends on all Linux fat binaries and archives because it's difficult to
# do dynamic dependencies due to the extensions in the filenames. These targets
# are typically only used during release anyways.
#
# Packages need to run after the archives are built, otherwise they cause tar
# errors like "file changed as we read it".
CODER_PACKAGE_DEPS := $(foreach os_arch, $(PACKAGE_OS_ARCHES), build/coder_$(VERSION)_$(os_arch) build/coder_$(VERSION)_$(os_arch).tar.gz)
$(CODER_ALL_PACKAGES): $(CODER_PACKAGE_DEPS)
	$(get-mode-os-arch-ext)

	./scripts/package.sh \
		--arch "$$arch" \
		--format "$$ext" \
		--version "$(VERSION)" \
		--output "$@" \
		"build/coder_$(VERSION)_$${os}_$${arch}"

# This task builds a Windows amd64 installer. Depends on makensis.
build/coder_$(VERSION)_windows_amd64_installer.exe: build/coder_$(VERSION)_windows_amd64.exe
	./scripts/build_windows_installer.sh \
		--version "$(VERSION)" \
		--output "$@" \
		"$<"

# Redirect from version-less Docker image targets to the versioned ones.
#
# Called like this:
#   make build/coder_linux_amd64.tag
$(CODER_ALL_NOVERSION_IMAGES): build/coder_%: build/coder_$(VERSION)_%
.PHONY: $(CODER_ALL_NOVERSION_IMAGES)

# Redirect from version-less push Docker image targets to the versioned ones.
#
# Called like this:
#   make push/build/coder_linux_amd64.tag
$(CODER_ALL_NOVERSION_IMAGES_PUSHED): push/build/coder_%: push/build/coder_$(VERSION)_%
.PHONY: $(CODER_ALL_NOVERSION_IMAGES_PUSHED)

# This task builds all Docker images. It parses the target name to get the
# metadata for the build, so it must be specified in this format:
#     build/coder_${version}_${os}_${arch}.tag
#
# Supports linux_amd64, linux_arm64, linux_armv7.
#
# Images need to run after the archives and packages are built, otherwise they
# cause errors like "file changed as we read it".
$(CODER_ARCH_IMAGES): build/coder_$(VERSION)_%.tag: build/coder_$(VERSION)_% $(CODER_ARCH_IMAGE_PREREQUISITES)
	$(get-mode-os-arch-ext)

	image_tag="$$(./scripts/image_tag.sh --arch "$$arch" --version "$(VERSION)")"
	./scripts/build_docker.sh \
		--arch "$$arch" \
		--target "$$image_tag" \
		--version "$(VERSION)" \
		"build/coder_$(VERSION)_$${os}_$${arch}"

	echo "$$image_tag" > "$@"

# Multi-arch Docker image. This requires all architecture-specific images to be
# built AND pushed.
$(CODER_MAIN_IMAGE): $(CODER_ARCH_IMAGES_PUSHED)
	image_tag="$$(./scripts/image_tag.sh --version "$(VERSION)")"
	./scripts/build_docker_multiarch.sh \
		--target "$$image_tag" \
		--version "$(VERSION)" \
		$(foreach img, $^, "$$(cat "$(img:push/%=%)")")

	echo "$$image_tag" > "$@"

# Push a Docker image.
$(CODER_ARCH_IMAGES_PUSHED): push/%: %
	image_tag="$$(cat "$<")"
	docker push "$$image_tag"
.PHONY: $(CODER_ARCH_IMAGES_PUSHED)

# Push the multi-arch Docker manifest.
push/$(CODER_MAIN_IMAGE): $(CODER_MAIN_IMAGE)
	image_tag="$$(cat "$<")"
	docker manifest push "$$image_tag"
.PHONY: push/$(CODER_MAIN_IMAGE)

# Shortcut for Helm chart package.
build/coder_helm.tgz: build/coder_helm_$(VERSION).tgz
	rm -f "$@"
	ln "$<" "$@"

# Helm chart package.
build/coder_helm_$(VERSION).tgz:
	./scripts/helm.sh \
		--version "$(VERSION)" \
		--output "$@"

site/out/index.html: site/package.json $(shell find ./site $(FIND_EXCLUSIONS) -type f \( -name '*.ts' -o -name '*.tsx' \))
	./scripts/yarn_install.sh
	cd site
	yarn build

install: build/coder_$(VERSION)_$(GOOS)_$(GOARCH)$(GOOS_BIN_EXT)
	install_dir="$$(go env GOPATH)/bin"
	output_file="$${install_dir}/coder$(GOOS_BIN_EXT)"

	mkdir -p "$$install_dir"
	cp "$<" "$$output_file"
.PHONY: install

fmt: fmt/prettier fmt/terraform fmt/shfmt fmt/go
.PHONY: fmt

fmt/go:
	# VS Code users should check out
	# https://github.com/mvdan/gofumpt#visual-studio-code
	go run mvdan.cc/gofumpt@v0.4.0 -w -l .
.PHONY: fmt/go

fmt/prettier:
	echo "--- prettier"
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

fmt/shfmt: $(SHELL_SRC_FILES)
	echo "--- shfmt"
# Only do diff check in CI, errors on diff.
ifdef CI
	shfmt -d $(SHELL_SRC_FILES)
else
	shfmt -w $(SHELL_SRC_FILES)
endif
.PHONY: fmt/shfmt

lint: lint/shellcheck lint/go
.PHONY: lint

lint/go:
	./scripts/check_enterprise_imports.sh
	golangci-lint run
.PHONY: lint/go

# Use shfmt to determine the shell files, takes editorconfig into consideration.
lint/shellcheck: $(SHELL_SRC_FILES)
	echo "--- shellcheck"
	shellcheck --external-sources $(SHELL_SRC_FILES)
.PHONY: lint/shellcheck

# all gen targets should be added here and to gen/mark-fresh
gen: \
	coderd/database/dump.sql \
	coderd/database/querier.go \
	provisionersdk/proto/provisioner.pb.go \
	provisionerd/proto/provisionerd.pb.go \
	site/src/api/typesGenerated.ts \
	docs/admin/prometheus.md \
	docs/cli.md \
	docs/admin/audit-logs.md \
	coderd/apidoc/swagger.json \
	.prettierignore.include \
	.prettierignore \
	site/.prettierrc.yaml \
	site/.prettierignore \
	site/.eslintignore
.PHONY: gen

# Mark all generated files as fresh so make thinks they're up-to-date. This is
# used during releases so we don't run generation scripts.
gen/mark-fresh:
	files="\
		coderd/database/dump.sql \
		coderd/database/querier.go \
		provisionersdk/proto/provisioner.pb.go \
		provisionerd/proto/provisionerd.pb.go \
		site/src/api/typesGenerated.ts \
		docs/admin/prometheus.md \
		docs/cli.md \
		docs/admin/audit-logs.md \
		coderd/apidoc/swagger.json \
		.prettierignore.include \
		.prettierignore \
		site/.prettierrc.yaml \
		site/.prettierignore \
		site/.eslintignore \
	"
	for file in $$files; do
		echo "$$file"
		if [ ! -f "$$file" ]; then
			echo "File '$$file' does not exist"
			exit 1
		fi

		# touch sets the mtime of the file to the current time
		touch $$file
	done
.PHONY: gen/mark-fresh

# Runs migrations to output a dump of the database schema after migrations are
# applied.
coderd/database/dump.sql: coderd/database/gen/dump/main.go $(wildcard coderd/database/migrations/*.sql)
	go run ./coderd/database/gen/dump/main.go

# Generates Go code for querying the database.
coderd/database/querier.go: coderd/database/sqlc.yaml coderd/database/dump.sql $(wildcard coderd/database/queries/*.sql) coderd/database/gen/enum/main.go
	./coderd/database/generate.sh

provisionersdk/proto/provisioner.pb.go: provisionersdk/proto/provisioner.proto
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-drpc_out=. \
		--go-drpc_opt=paths=source_relative \
		./provisionersdk/proto/provisioner.proto

provisionerd/proto/provisionerd.pb.go: provisionerd/proto/provisionerd.proto
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-drpc_out=. \
		--go-drpc_opt=paths=source_relative \
		./provisionerd/proto/provisionerd.proto

site/src/api/typesGenerated.ts: scripts/apitypings/main.go $(shell find ./codersdk $(FIND_EXCLUSIONS) -type f -name '*.go')
	go run scripts/apitypings/main.go > site/src/api/typesGenerated.ts
	cd site
	yarn run format:types

docs/admin/prometheus.md: scripts/metricsdocgen/main.go scripts/metricsdocgen/metrics
	go run scripts/metricsdocgen/main.go
	cd site
	yarn run format:write:only ../docs/admin/prometheus.md

docs/cli.md: scripts/clidocgen/main.go $(GO_SRC_FILES) docs/manifest.json
	rm -rf ./docs/cli/*.md
	BASE_PATH="." go run ./scripts/clidocgen
	cd site
	yarn run format:write:only ../docs/cli.md ../docs/cli/*.md ../docs/manifest.json

docs/admin/audit-logs.md: scripts/auditdocgen/main.go enterprise/audit/table.go
	go run scripts/auditdocgen/main.go
	cd site
	yarn run format:write:only ../docs/admin/audit-logs.md

coderd/apidoc/swagger.json: $(shell find ./scripts/apidocgen $(FIND_EXCLUSIONS) -type f) $(wildcard coderd/*.go) $(wildcard enterprise/coderd/*.go) $(wildcard codersdk/*.go) .swaggo docs/manifest.json
	./scripts/apidocgen/generate.sh
	yarn run --cwd=site format:write:only ../docs/api ../docs/manifest.json ../coderd/apidoc/swagger.json

update-golden-files: cli/testdata/.gen-golden
.PHONY: update-golden-files

cli/testdata/.gen-golden: $(wildcard cli/testdata/*.golden) $(GO_SRC_FILES)
	go test ./cli -run=TestCommandHelp -update
	touch "$@"

# Generate a prettierrc for the site package that uses relative paths for
# overrides. This allows us to share the same prettier config between the
# site and the root of the repo.
site/.prettierrc.yaml: .prettierrc.yaml
	. ./scripts/lib.sh
	dependencies yq

	echo "# Code generated by Makefile (../$<). DO NOT EDIT." > "$@"
	echo "" >> "$@"

	# Replace all listed override files with relative paths inside site/.
	# - ./ -> ../
	# - ./site -> ./
	yq \
		'.overrides[].files |= map(. | sub("^./"; "") | sub("^"; "../") | sub("../site/"; "./"))' \
		"$<" >> "$@"

# Combine .gitignore with .prettierignore.include to generate .prettierignore.
.prettierignore: .gitignore .prettierignore.include
	echo "# Code generated by Makefile ($^). DO NOT EDIT." > "$@"
	echo "" >> "$@"
	for f in $^; do
		echo "# $${f}:" >> "$@"
		cat "$$f" >> "$@"
	done

# Generate ignore files based on gitignore into the site directory. We turn all
# rules into relative paths for the `site/` directory (where applicable),
# following the pattern format defined by git:
# https://git-scm.com/docs/gitignore#_pattern_format
#
# This is done for compatibility reasons, see:
# https://github.com/prettier/prettier/issues/8048
# https://github.com/prettier/prettier/issues/8506
# https://github.com/prettier/prettier/issues/8679
site/.eslintignore site/.prettierignore: .prettierignore Makefile
	rm -f "$@"
	touch "$@"
	# Skip generated by header, inherit `.prettierignore` header as-is.
	while read -r rule; do
		# Remove leading ! if present to simplify rule, added back at the end.
		tmp="$${rule#!}"
		ignore="$${rule%"$$tmp"}"
		rule="$$tmp"
		case "$$rule" in
			# Comments or empty lines (include).
			\#*|'') ;;
			# Generic rules (include).
			\*\**) ;;
			# Site prefixed rules (include).
			site/*) rule="$${rule#site/}";;
			./site/*) rule="$${rule#./site/}";;
			# Rules that are non-generic and don't start with site (rewrite).
			/*) rule=.."$$rule";;
			*/?*) rule=../"$$rule";;
			*) ;;
		esac
		echo "$${ignore}$${rule}" >> "$@"
	done < "$<"

test: test-clean
	gotestsum -- -v -short ./...
.PHONY: test

# When updating -timeout for this test, keep in sync with
# test-go-postgres (.github/workflows/coder.yaml).
test-postgres: test-clean test-postgres-docker
	# The postgres test is prone to failure, so we limit parallelism for
	# more consistent execution.
	DB=ci DB_FROM=$(shell go run scripts/migrate-ci/main.go) gotestsum \
		--junitfile="gotests.xml" \
		--packages="./..." -- \
		-covermode=atomic -coverprofile="gotests.coverage" -timeout=20m \
		-parallel=4 \
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
		-c full_page_writes=off \
		-c log_statement=all
	while ! pg_isready -h 127.0.0.1
	do
		echo "$(date) - waiting for database to start"
		sleep 0.5
	done
.PHONY: test-postgres-docker

test-clean:
	go clean -testcache
.PHONY: test-clean
