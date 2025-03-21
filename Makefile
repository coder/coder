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
POSTGRES_VERSION ?= 16

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
	-not \( \( -path '*/.git/*' -o -path './build/*' -o -path './vendor/*' -o -path './.coderv2/*' -o -path '*/node_modules/*' -o -path '*/out/*' -o -path './coderd/apidoc/*' -o -path '*/.next/*' -o -path '*/.terraform/*' \) -prune \)
# Source files used for make targets, evaluated on use.
GO_SRC_FILES := $(shell find . $(FIND_EXCLUSIONS) -type f -name '*.go' -not -name '*_test.go')
# All the shell files in the repo, excluding ignored files.
SHELL_SRC_FILES := $(shell find . $(FIND_EXCLUSIONS) -type f -name '*.sh')

# Ensure we don't use the user's git configs which might cause side-effects
GIT_FLAGS = GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null

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

# All ${OS}_${ARCH} combos we build the desktop dylib for.
DYLIB_ARCHES := darwin_amd64 darwin_arm64

# Computed variables based on the above.
CODER_SLIM_BINARIES      := $(addprefix build/coder-slim_$(VERSION)_,$(OS_ARCHES))
CODER_DYLIBS             := $(foreach os_arch, $(DYLIB_ARCHES), build/coder-vpn_$(VERSION)_$(os_arch).dylib)
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
	rm -rf build/ site/build/ site/out/
	mkdir -p build/
	git restore site/out/
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
	$(shell find ./examples/templates) \
	site/static/error.html

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

# This task builds Coder Desktop dylibs
$(CODER_DYLIBS): go.mod go.sum $(GO_SRC_FILES)
	@if [ "$(shell uname)" = "Darwin" ]; then
		$(get-mode-os-arch-ext)
		./scripts/build_go.sh \
			--os "$$os" \
			--arch "$$arch" \
			--version "$(VERSION)" \
			--output "$@" \
			--dylib

	else
		echo "ERROR: Can't build dylib on non-Darwin OS" 1>&2
		exit 1
	fi

# This task builds both dylibs
build/coder-dylib: $(CODER_DYLIBS)
.PHONY: build/coder-dylib

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

# Helm charts that are available
charts = coder provisioner

# Shortcut for Helm chart package.
$(foreach chart,$(charts),build/$(chart)_helm.tgz): build/%_helm.tgz: build/%_helm_$(VERSION).tgz
	rm -f "$@"
	ln "$<" "$@"

# Helm chart package.
$(foreach chart,$(charts),build/$(chart)_helm_$(VERSION).tgz): build/%_helm_$(VERSION).tgz:
	./scripts/helm.sh \
		--version "$(VERSION)" \
		--chart $* \
		--output "$@"

node_modules/.installed: package.json pnpm-lock.yaml
	./scripts/pnpm_install.sh
	touch "$@"

offlinedocs/node_modules/.installed: offlinedocs/package.json offlinedocs/pnpm-lock.yaml
	(cd offlinedocs/ && ../scripts/pnpm_install.sh)
	touch "$@"

site/node_modules/.installed: site/package.json site/pnpm-lock.yaml
	(cd site/ && ../scripts/pnpm_install.sh)
	touch "$@"

scripts/apidocgen/.installed: scripts/apidocgen/package.json scripts/apidocgen/pnpm-lock.yaml
	(cd scripts/apidocgen && ../../scripts/pnpm_install.sh)
	touch "$@"

SITE_GEN_FILES := \
	site/src/api/typesGenerated.ts \
	site/src/api/rbacresourcesGenerated.ts \
	site/src/api/countriesGenerated.ts \
	site/src/theme/icons.json

site/out/index.html: \
	site/node_modules/.installed \
	site/static/install.sh \
	$(SITE_GEN_FILES) \
	$(shell find ./site $(FIND_EXCLUSIONS) -type f \( -name '*.ts' -o -name '*.tsx' \))
	cd site/
	# prevents this directory from getting to big, and causing "too much data" errors
	rm -rf out/assets/
	pnpm build

offlinedocs/out/index.html: offlinedocs/node_modules/.installed $(shell find ./offlinedocs $(FIND_EXCLUSIONS) -type f) $(shell find ./docs $(FIND_EXCLUSIONS) -type f | sed 's: :\\ :g')
	cd offlinedocs/
	../scripts/pnpm_install.sh
	pnpm export

build/coder_docs_$(VERSION).tgz: offlinedocs/out/index.html
	tar -czf "$@" -C offlinedocs/out .

install: build/coder_$(VERSION)_$(GOOS)_$(GOARCH)$(GOOS_BIN_EXT)
	install_dir="$$(go env GOPATH)/bin"
	output_file="$${install_dir}/coder$(GOOS_BIN_EXT)"

	mkdir -p "$$install_dir"
	cp "$<" "$$output_file"
.PHONY: install

BOLD := $(shell tput bold 2>/dev/null)
GREEN := $(shell tput setaf 2 2>/dev/null)
RESET := $(shell tput sgr0 2>/dev/null)

fmt: fmt/ts fmt/go fmt/terraform fmt/shfmt fmt/biome fmt/markdown
.PHONY: fmt

fmt/go:
	go mod tidy
	echo "$(GREEN)==>$(RESET) $(BOLD)fmt/go$(RESET)"
	# VS Code users should check out
	# https://github.com/mvdan/gofumpt#visual-studio-code
	find . $(FIND_EXCLUSIONS) -type f -name '*.go' -print0 | \
		xargs -0 grep --null -L "DO NOT EDIT" | \
		xargs -0 go run mvdan.cc/gofumpt@v0.4.0 -w -l
.PHONY: fmt/go

fmt/ts: site/node_modules/.installed
	echo "$(GREEN)==>$(RESET) $(BOLD)fmt/ts$(RESET)"
	cd site
# Avoid writing files in CI to reduce file write activity
ifdef CI
	pnpm run check --linter-enabled=false
else
	pnpm run check:fix
endif
.PHONY: fmt/ts

fmt/biome: site/node_modules/.installed
	echo "$(GREEN)==>$(RESET) $(BOLD)fmt/biome$(RESET)"
	cd site/
# Avoid writing files in CI to reduce file write activity
ifdef CI
	pnpm run format:check
else
	pnpm run format
endif
.PHONY: fmt/biome

fmt/terraform: $(wildcard *.tf)
	echo "$(GREEN)==>$(RESET) $(BOLD)fmt/terraform$(RESET)"
	terraform fmt -recursive
.PHONY: fmt/terraform

fmt/shfmt: $(SHELL_SRC_FILES)
	echo "$(GREEN)==>$(RESET) $(BOLD)fmt/shfmt$(RESET)"
# Only do diff check in CI, errors on diff.
ifdef CI
	shfmt -d $(SHELL_SRC_FILES)
else
	shfmt -w $(SHELL_SRC_FILES)
endif
.PHONY: fmt/shfmt

fmt/markdown: node_modules/.installed
	echo "$(GREEN)==>$(RESET) $(BOLD)fmt/markdown$(RESET)"
	pnpm format-docs
.PHONY: fmt/markdown

lint: lint/shellcheck lint/go lint/ts lint/examples lint/helm lint/site-icons lint/markdown
.PHONY: lint

lint/site-icons:
	./scripts/check_site_icons.sh
.PHONY: lint/site-icons

lint/ts: site/node_modules/.installed
	cd site/
	pnpm lint
.PHONY: lint/ts

lint/go:
	./scripts/check_enterprise_imports.sh
	./scripts/check_codersdk_imports.sh
	linter_ver=$(shell egrep -o 'GOLANGCI_LINT_VERSION=\S+' dogfood/coder/Dockerfile | cut -d '=' -f 2)
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@v$$linter_ver run
.PHONY: lint/go

lint/examples:
	go run ./scripts/examplegen/main.go -lint
.PHONY: lint/examples

# Use shfmt to determine the shell files, takes editorconfig into consideration.
lint/shellcheck: $(SHELL_SRC_FILES)
	echo "--- shellcheck"
	shellcheck --external-sources $(SHELL_SRC_FILES)
.PHONY: lint/shellcheck

lint/helm:
	cd helm/
	make lint
.PHONY: lint/helm

lint/markdown: node_modules/.installed
	pnpm lint-docs
.PHONY: lint/markdown

# All files generated by the database should be added here, and this can be used
# as a target for jobs that need to run after the database is generated.
DB_GEN_FILES := \
	coderd/database/dump.sql \
	coderd/database/querier.go \
	coderd/database/unique_constraint.go \
	coderd/database/dbmem/dbmem.go \
	coderd/database/dbmetrics/dbmetrics.go \
	coderd/database/dbauthz/dbauthz.go \
	coderd/database/dbmock/dbmock.go

TAILNETTEST_MOCKS := \
	tailnet/tailnettest/coordinatormock.go \
	tailnet/tailnettest/coordinateemock.go \
	tailnet/tailnettest/workspaceupdatesprovidermock.go \
	tailnet/tailnettest/subscriptionmock.go

GEN_FILES := \
	tailnet/proto/tailnet.pb.go \
	agent/proto/agent.pb.go \
	provisionersdk/proto/provisioner.pb.go \
	provisionerd/proto/provisionerd.pb.go \
	vpn/vpn.pb.go \
	$(DB_GEN_FILES) \
	$(SITE_GEN_FILES) \
	coderd/rbac/object_gen.go \
	codersdk/rbacresources_gen.go \
	docs/admin/integrations/prometheus.md \
	docs/reference/cli/index.md \
	docs/admin/security/audit-logs.md \
	coderd/apidoc/swagger.json \
	docs/manifest.json \
	provisioner/terraform/testdata/version \
	site/e2e/provisionerGenerated.ts \
	examples/examples.gen.json \
	$(TAILNETTEST_MOCKS) \
	coderd/database/pubsub/psmock/psmock.go \
	agent/agentcontainers/acmock/acmock.go \
	agent/agentcontainers/dcspec/dcspec_gen.go

# all gen targets should be added here and to gen/mark-fresh
gen: gen/db gen/golden-files $(GEN_FILES)
.PHONY: gen

gen/db: $(DB_GEN_FILES)
.PHONY: gen/db

gen/golden-files: \
	cli/testdata/.gen-golden \
	coderd/.gen-golden \
	coderd/notifications/.gen-golden \
	enterprise/cli/testdata/.gen-golden \
	enterprise/tailnet/testdata/.gen-golden \
	helm/coder/tests/testdata/.gen-golden \
	helm/provisioner/tests/testdata/.gen-golden \
	provisioner/terraform/testdata/.gen-golden \
	tailnet/testdata/.gen-golden
.PHONY: gen/golden-files

# Mark all generated files as fresh so make thinks they're up-to-date. This is
# used during releases so we don't run generation scripts.
gen/mark-fresh:
	files="\
		tailnet/proto/tailnet.pb.go \
		agent/proto/agent.pb.go \
		provisionersdk/proto/provisioner.pb.go \
		provisionerd/proto/provisionerd.pb.go \
		vpn/vpn.pb.go \
		coderd/database/dump.sql \
		$(DB_GEN_FILES) \
		site/src/api/typesGenerated.ts \
		coderd/rbac/object_gen.go \
		codersdk/rbacresources_gen.go \
		site/src/api/rbacresourcesGenerated.ts \
		site/src/api/countriesGenerated.ts \
		docs/admin/integrations/prometheus.md \
		docs/reference/cli/index.md \
		docs/admin/security/audit-logs.md \
		coderd/apidoc/swagger.json \
		docs/manifest.json \
		site/e2e/provisionerGenerated.ts \
		site/src/theme/icons.json \
		examples/examples.gen.json \
		$(TAILNETTEST_MOCKS) \
		coderd/database/pubsub/psmock/psmock.go \
		agent/agentcontainers/acmock/acmock.go \
		agent/agentcontainers/dcspec/dcspec_gen.go \
		"

	for file in $$files; do
		echo "$$file"
		if [ ! -f "$$file" ]; then
			echo "File '$$file' does not exist"
			exit 1
		fi

		# touch sets the mtime of the file to the current time
		touch "$$file"
	done
.PHONY: gen/mark-fresh

# Runs migrations to output a dump of the database schema after migrations are
# applied.
coderd/database/dump.sql: coderd/database/gen/dump/main.go $(wildcard coderd/database/migrations/*.sql)
	go run ./coderd/database/gen/dump/main.go
	touch "$@"

# Generates Go code for querying the database.
# coderd/database/queries.sql.go
# coderd/database/models.go
coderd/database/querier.go: coderd/database/sqlc.yaml coderd/database/dump.sql $(wildcard coderd/database/queries/*.sql)
	./coderd/database/generate.sh
	touch "$@"

coderd/database/dbmock/dbmock.go: coderd/database/db.go coderd/database/querier.go
	go generate ./coderd/database/dbmock/
	touch "$@"

coderd/database/pubsub/psmock/psmock.go: coderd/database/pubsub/pubsub.go
	go generate ./coderd/database/pubsub/psmock
	touch "$@"

agent/agentcontainers/acmock/acmock.go: agent/agentcontainers/containers.go
	go generate ./agent/agentcontainers/acmock/
	touch "$@"

agent/agentcontainers/dcspec/dcspec_gen.go: agent/agentcontainers/dcspec/devContainer.base.schema.json
	go generate ./agent/agentcontainers/dcspec/
	touch "$@"

$(TAILNETTEST_MOCKS): tailnet/coordinator.go tailnet/service.go
	go generate ./tailnet/tailnettest/
	touch "$@"

tailnet/proto/tailnet.pb.go: tailnet/proto/tailnet.proto
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-drpc_out=. \
		--go-drpc_opt=paths=source_relative \
		./tailnet/proto/tailnet.proto

agent/proto/agent.pb.go: agent/proto/agent.proto
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-drpc_out=. \
		--go-drpc_opt=paths=source_relative \
		./agent/proto/agent.proto

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

vpn/vpn.pb.go: vpn/vpn.proto
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		./vpn/vpn.proto

site/src/api/typesGenerated.ts: site/node_modules/.installed $(wildcard scripts/apitypings/*) $(shell find ./codersdk $(FIND_EXCLUSIONS) -type f -name '*.go')
	# -C sets the directory for the go run command
	go run -C ./scripts/apitypings main.go > $@
	(cd site/ && pnpm exec biome format --write src/api/typesGenerated.ts)
	touch "$@"

site/e2e/provisionerGenerated.ts: site/node_modules/.installed provisionerd/proto/provisionerd.pb.go provisionersdk/proto/provisioner.pb.go
	(cd site/ && pnpm run gen:provisioner)
	touch "$@"

site/src/theme/icons.json: site/node_modules/.installed $(wildcard scripts/gensite/*) $(wildcard site/static/icon/*)
	go run ./scripts/gensite/ -icons "$@"
	(cd site/ && pnpm exec biome format --write src/theme/icons.json)
	touch "$@"

examples/examples.gen.json: scripts/examplegen/main.go examples/examples.go $(shell find ./examples/templates)
	go run ./scripts/examplegen/main.go > examples/examples.gen.json
	touch "$@"

coderd/rbac/object_gen.go: scripts/typegen/rbacobject.gotmpl scripts/typegen/main.go coderd/rbac/object.go coderd/rbac/policy/policy.go
	tempdir=$(shell mktemp -d /tmp/typegen_rbac_object.XXXXXX)
	go run ./scripts/typegen/main.go rbac object > "$$tempdir/object_gen.go"
	mv -v "$$tempdir/object_gen.go" coderd/rbac/object_gen.go
	rmdir -v "$$tempdir"
	touch "$@"

codersdk/rbacresources_gen.go: scripts/typegen/codersdk.gotmpl scripts/typegen/main.go coderd/rbac/object.go coderd/rbac/policy/policy.go
	# Do no overwrite codersdk/rbacresources_gen.go directly, as it would make the file empty, breaking
 	# the `codersdk` package and any parallel build targets.
	go run scripts/typegen/main.go rbac codersdk > /tmp/rbacresources_gen.go
	mv /tmp/rbacresources_gen.go codersdk/rbacresources_gen.go
	touch "$@"

site/src/api/rbacresourcesGenerated.ts: site/node_modules/.installed scripts/typegen/codersdk.gotmpl scripts/typegen/main.go coderd/rbac/object.go coderd/rbac/policy/policy.go
	go run scripts/typegen/main.go rbac typescript > "$@"
	(cd site/ && pnpm exec biome format --write src/api/rbacresourcesGenerated.ts)
	touch "$@"

site/src/api/countriesGenerated.ts: site/node_modules/.installed scripts/typegen/countries.tstmpl scripts/typegen/main.go codersdk/countries.go
	go run scripts/typegen/main.go countries > "$@"
	(cd site/ && pnpm exec biome format --write src/api/countriesGenerated.ts)
	touch "$@"

docs/admin/integrations/prometheus.md: node_modules/.installed scripts/metricsdocgen/main.go scripts/metricsdocgen/metrics
	go run scripts/metricsdocgen/main.go
	pnpm exec markdownlint-cli2 --fix ./docs/admin/integrations/prometheus.md
	pnpm exec markdown-table-formatter ./docs/admin/integrations/prometheus.md
	touch "$@"

docs/reference/cli/index.md: node_modules/.installed scripts/clidocgen/main.go examples/examples.gen.json $(GO_SRC_FILES)
	CI=true BASE_PATH="." go run ./scripts/clidocgen
	pnpm exec markdownlint-cli2 --fix ./docs/reference/cli/*.md
	pnpm exec markdown-table-formatter ./docs/reference/cli/*.md
	touch "$@"

docs/admin/security/audit-logs.md: node_modules/.installed coderd/database/querier.go scripts/auditdocgen/main.go enterprise/audit/table.go coderd/rbac/object_gen.go
	go run scripts/auditdocgen/main.go
	pnpm exec markdownlint-cli2 --fix ./docs/admin/security/audit-logs.md
	pnpm exec markdown-table-formatter ./docs/admin/security/audit-logs.md
	touch "$@"

coderd/apidoc/.gen: \
	node_modules/.installed \
	scripts/apidocgen/.installed \
	$(wildcard coderd/*.go) \
	$(wildcard enterprise/coderd/*.go) \
	$(wildcard codersdk/*.go) \
	$(wildcard enterprise/wsproxy/wsproxysdk/*.go) \
	$(DB_GEN_FILES) \
	coderd/rbac/object_gen.go \
	.swaggo \
	scripts/apidocgen/generate.sh \
	$(wildcard scripts/apidocgen/postprocess/*) \
	$(wildcard scripts/apidocgen/markdown-template/*)
	./scripts/apidocgen/generate.sh
	pnpm exec markdownlint-cli2 --fix ./docs/reference/api/*.md
	pnpm exec markdown-table-formatter ./docs/reference/api/*.md
	touch "$@"

docs/manifest.json: site/node_modules/.installed coderd/apidoc/.gen docs/reference/cli/index.md
	(cd site/ && pnpm exec biome format --write ../docs/manifest.json)
	touch "$@"

coderd/apidoc/swagger.json: site/node_modules/.installed coderd/apidoc/.gen
	(cd site/ && pnpm exec biome format --write ../coderd/apidoc/swagger.json)
	touch "$@"

update-golden-files:
	echo 'WARNING: This target is deprecated. Use "make gen/golden-files" instead.' 2>&1
	echo 'Running "make gen/golden-files"' 2>&1
	make gen/golden-files
.PHONY: update-golden-files

clean/golden-files:
	find . -type f -name '.gen-golden' -delete
	find \
		cli/testdata \
		coderd/notifications/testdata \
		coderd/testdata \
		enterprise/cli/testdata \
		enterprise/tailnet/testdata \
		helm/coder/tests/testdata \
		helm/provisioner/tests/testdata \
		provisioner/terraform/testdata \
		tailnet/testdata \
		-type f -name '*.golden' -delete
.PHONY: clean/golden-files

cli/testdata/.gen-golden: $(wildcard cli/testdata/*.golden) $(wildcard cli/*.tpl) $(GO_SRC_FILES) $(wildcard cli/*_test.go)
	go test ./cli -run="Test(CommandHelp|ServerYAML|ErrorExamples|.*Golden)" -update
	touch "$@"

enterprise/cli/testdata/.gen-golden: $(wildcard enterprise/cli/testdata/*.golden) $(wildcard cli/*.tpl) $(GO_SRC_FILES) $(wildcard enterprise/cli/*_test.go)
	go test ./enterprise/cli -run="TestEnterpriseCommandHelp" -update
	touch "$@"

tailnet/testdata/.gen-golden: $(wildcard tailnet/testdata/*.golden.html) $(GO_SRC_FILES) $(wildcard tailnet/*_test.go)
	go test ./tailnet -run="TestDebugTemplate" -update
	touch "$@"

enterprise/tailnet/testdata/.gen-golden: $(wildcard enterprise/tailnet/testdata/*.golden.html) $(GO_SRC_FILES) $(wildcard enterprise/tailnet/*_test.go)
	go test ./enterprise/tailnet -run="TestDebugTemplate" -update
	touch "$@"

helm/coder/tests/testdata/.gen-golden: $(wildcard helm/coder/tests/testdata/*.yaml) $(wildcard helm/coder/tests/testdata/*.golden) $(GO_SRC_FILES) $(wildcard helm/coder/tests/*_test.go)
	go test ./helm/coder/tests -run=TestUpdateGoldenFiles -update
	touch "$@"

helm/provisioner/tests/testdata/.gen-golden: $(wildcard helm/provisioner/tests/testdata/*.yaml) $(wildcard helm/provisioner/tests/testdata/*.golden) $(GO_SRC_FILES) $(wildcard helm/provisioner/tests/*_test.go)
	go test ./helm/provisioner/tests -run=TestUpdateGoldenFiles -update
	touch "$@"

coderd/.gen-golden: $(wildcard coderd/testdata/*/*.golden) $(GO_SRC_FILES) $(wildcard coderd/*_test.go)
	go test ./coderd -run="Test.*Golden$$" -update
	touch "$@"

coderd/notifications/.gen-golden: $(wildcard coderd/notifications/testdata/*/*.golden) $(GO_SRC_FILES) $(wildcard coderd/notifications/*_test.go)
	go test ./coderd/notifications -run="Test.*Golden$$" -update
	touch "$@"

provisioner/terraform/testdata/.gen-golden: $(wildcard provisioner/terraform/testdata/*/*.golden) $(GO_SRC_FILES) $(wildcard provisioner/terraform/*_test.go)
	go test ./provisioner/terraform -run="Test.*Golden$$" -update
	touch "$@"

provisioner/terraform/testdata/version:
	if [[ "$(shell cat provisioner/terraform/testdata/version.txt)" != "$(shell terraform version -json | jq -r '.terraform_version')" ]]; then
		./provisioner/terraform/testdata/generate.sh
	fi
.PHONY: provisioner/terraform/testdata/version

test:
	$(GIT_FLAGS) gotestsum --format standard-quiet -- -v -short -count=1 ./... $(if $(RUN),-run $(RUN))
.PHONY: test

test-cli:
	$(GIT_FLAGS) gotestsum --format standard-quiet -- -v -short -count=1 ./cli/...
.PHONY: test-cli

# sqlc-cloud-is-setup will fail if no SQLc auth token is set. Use this as a
# dependency for any sqlc-cloud related targets.
sqlc-cloud-is-setup:
	if [[ "$(SQLC_AUTH_TOKEN)" == "" ]]; then
		echo "ERROR: 'SQLC_AUTH_TOKEN' must be set to auth with sqlc cloud before running verify." 1>&2
		exit 1
	fi
.PHONY: sqlc-cloud-is-setup

sqlc-push: sqlc-cloud-is-setup test-postgres-docker
	echo "--- sqlc push"
	SQLC_DATABASE_URL="postgresql://postgres:postgres@localhost:5432/$(shell go run scripts/migrate-ci/main.go)" \
	sqlc push -f coderd/database/sqlc.yaml && echo "Passed sqlc push"
.PHONY: sqlc-push

sqlc-verify: sqlc-cloud-is-setup test-postgres-docker
	echo "--- sqlc verify"
	SQLC_DATABASE_URL="postgresql://postgres:postgres@localhost:5432/$(shell go run scripts/migrate-ci/main.go)" \
	sqlc verify -f coderd/database/sqlc.yaml && echo "Passed sqlc verify"
.PHONY: sqlc-verify

sqlc-vet: test-postgres-docker
	echo "--- sqlc vet"
	SQLC_DATABASE_URL="postgresql://postgres:postgres@localhost:5432/$(shell go run scripts/migrate-ci/main.go)" \
	sqlc vet -f coderd/database/sqlc.yaml && echo "Passed sqlc vet"
.PHONY: sqlc-vet

# When updating -timeout for this test, keep in sync with
# test-go-postgres (.github/workflows/coder.yaml).
# Do add coverage flags so that test caching works.
test-postgres: test-postgres-docker
	# The postgres test is prone to failure, so we limit parallelism for
	# more consistent execution.
	$(GIT_FLAGS)  DB=ci gotestsum \
		--junitfile="gotests.xml" \
		--jsonfile="gotests.json" \
		--packages="./..." -- \
		-timeout=20m \
		-failfast \
		-count=1
.PHONY: test-postgres

test-migrations: test-postgres-docker
	echo "--- test migrations"
	set -euo pipefail
	COMMIT_FROM=$(shell git log -1 --format='%h' HEAD)
	echo "COMMIT_FROM=$${COMMIT_FROM}"
	COMMIT_TO=$(shell git log -1 --format='%h' origin/main)
	echo "COMMIT_TO=$${COMMIT_TO}"
	if [[ "$${COMMIT_FROM}" == "$${COMMIT_TO}" ]]; then echo "Nothing to do!"; exit 0; fi
	echo "DROP DATABASE IF EXISTS migrate_test_$${COMMIT_FROM}; CREATE DATABASE migrate_test_$${COMMIT_FROM};" | psql 'postgresql://postgres:postgres@localhost:5432/postgres?sslmode=disable'
	go run ./scripts/migrate-test/main.go --from="$$COMMIT_FROM" --to="$$COMMIT_TO" --postgres-url="postgresql://postgres:postgres@localhost:5432/migrate_test_$${COMMIT_FROM}?sslmode=disable"
.PHONY: test-migrations

# NOTE: we set --memory to the same size as a GitHub runner.
test-postgres-docker:
	docker rm -f test-postgres-docker-${POSTGRES_VERSION} || true

	# Try pulling up to three times to avoid CI flakes.
	docker pull gcr.io/coder-dev-1/postgres:${POSTGRES_VERSION} || {
		retries=2
		for try in $(seq 1 ${retries}); do
			echo "Failed to pull image, retrying (${try}/${retries})..."
			sleep 1
			if docker pull gcr.io/coder-dev-1/postgres:${POSTGRES_VERSION}; then
				break
			fi
		done
	}

	# Make sure to not overallocate work_mem and max_connections as each
	# connection will be allowed to use this much memory. Try adjusting
	# shared_buffers instead, if needed.
	#
	# - work_mem=8MB * max_connections=1000 = 8GB
	# - shared_buffers=2GB + effective_cache_size=1GB = 3GB
	#
	# This leaves 5GB for the rest of the system _and_ storing the
	# database in memory (--tmpfs).
	#
	# https://www.postgresql.org/docs/current/runtime-config-resource.html#GUC-WORK-MEM
	docker run \
		--env POSTGRES_PASSWORD=postgres \
		--env POSTGRES_USER=postgres \
		--env POSTGRES_DB=postgres \
		--env PGDATA=/tmp \
		--tmpfs /tmp \
		--publish 5432:5432 \
		--name test-postgres-docker-${POSTGRES_VERSION} \
		--restart no \
		--detach \
		--memory 16GB \
		gcr.io/coder-dev-1/postgres:${POSTGRES_VERSION} \
		-c shared_buffers=2GB \
		-c effective_cache_size=1GB \
		-c work_mem=8MB \
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

# Make sure to keep this in sync with test-go-race from .github/workflows/ci.yaml.
test-race:
	$(GIT_FLAGS) gotestsum --junitfile="gotests.xml" -- -race -count=1 -parallel 4 -p 4 ./...
.PHONY: test-race

test-tailnet-integration:
	env \
		CODER_TAILNET_TESTS=true \
		CODER_MAGICSOCK_DEBUG_LOGGING=true \
		TS_DEBUG_NETCHECK=true \
		GOTRACEBACK=single \
		go test \
			-exec "sudo -E" \
			-timeout=5m \
			-count=1 \
			./tailnet/test/integration
.PHONY: test-tailnet-integration

# Note: we used to add this to the test target, but it's not necessary and we can
# achieve the desired result by specifying -count=1 in the go test invocation
# instead. Keeping it here for convenience.
test-clean:
	go clean -testcache
.PHONY: test-clean

site/e2e/bin/coder: go.mod go.sum $(GO_SRC_FILES)
	go build -o $@ \
		-tags ts_omit_aws,ts_omit_bird,ts_omit_tap,ts_omit_kube \
		./enterprise/cmd/coder

test-e2e: site/e2e/bin/coder site/node_modules/.installed site/out/index.html
	cd site/
ifdef CI
	DEBUG=pw:api pnpm playwright:test --forbid-only --workers 1
else
	pnpm playwright:test
endif
.PHONY: test-e2e

dogfood/coder/nix.hash: flake.nix flake.lock
	sha256sum flake.nix flake.lock >./dogfood/coder/nix.hash
