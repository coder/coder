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

# When MAKE_TIMED=1, replace SHELL with a wrapper that prints
# elapsed wall-clock time for each recipe. pre-commit and pre-push
# set this on their sub-makes so every parallel job reports its
# duration. Ad-hoc usage: make MAKE_TIMED=1 test
ifdef MAKE_TIMED
SHELL := $(CURDIR)/scripts/lib/timed-shell.sh
.SHELLFLAGS = $@ -ceu
export MAKE_TIMED
endif

# This doesn't work on directories.
# See https://stackoverflow.com/questions/25752543/make-delete-on-error-for-directory-targets
.DELETE_ON_ERROR:

# Protect git-tracked generated files from deletion on interrupt.
# .DELETE_ON_ERROR is desirable for most targets but for files that
# are committed to git and serve as inputs to other rules, deletion
# is worse than a stale file — `git restore` is the recovery path.
.PRECIOUS: \
	coderd/database/dump.sql \
	coderd/database/querier.go \
	coderd/database/unique_constraint.go \
	coderd/database/dbmetrics/querymetrics.go \
	coderd/database/dbauthz/dbauthz.go \
	coderd/database/dbmock/dbmock.go \
	coderd/database/pubsub/psmock/psmock.go \
	agent/agentcontainers/acmock/acmock.go \
	coderd/httpmw/loggermw/loggermock/loggermock.go \
	codersdk/workspacesdk/agentconnmock/agentconnmock.go \
	tailnet/tailnettest/coordinatormock.go \
	tailnet/tailnettest/coordinateemock.go \
	tailnet/tailnettest/workspaceupdatesprovidermock.go \
	tailnet/tailnettest/subscriptionmock.go \
	enterprise/aibridged/aibridgedmock/clientmock.go \
	enterprise/aibridged/aibridgedmock/poolmock.go \
	tailnet/proto/tailnet.pb.go \
	agent/proto/agent.pb.go \
	agent/agentsocket/proto/agentsocket.pb.go \
	agent/boundarylogproxy/codec/boundary.pb.go \
	provisionersdk/proto/provisioner.pb.go \
	provisionerd/proto/provisionerd.pb.go \
	vpn/vpn.pb.go \
	enterprise/aibridged/proto/aibridged.pb.go \
	site/src/api/typesGenerated.ts \
	site/e2e/provisionerGenerated.ts \
	site/src/api/chatModelOptionsGenerated.json \
	site/src/api/rbacresourcesGenerated.ts \
	site/src/api/countriesGenerated.ts \
	site/src/theme/icons.json \
	examples/examples.gen.json \
	docs/manifest.json \
	docs/admin/integrations/prometheus.md \
	docs/admin/security/audit-logs.md \
	docs/reference/cli/index.md \
	coderd/apidoc/swagger.json \
	coderd/rbac/object_gen.go \
	coderd/rbac/scopes_constants_gen.go \
	codersdk/rbacresources_gen.go \
	codersdk/apikey_scopes_gen.go

# atomic_write runs a command, captures stdout into a temp file, and
# atomically replaces $@. An optional second argument is a formatting
# command that receives the temp file path as its argument.
# Usage: $(call atomic_write,GENERATE_CMD[,FORMAT_CMD])
define atomic_write
	tmpdir=$$(mktemp -d -p _gen) && tmpfile=$$(realpath "$$tmpdir")/$(notdir $@) && \
		$(1) > "$$tmpfile" && \
		$(if $(2),$(2) "$$tmpfile" &&) \
		mv "$$tmpfile" "$@" && rm -rf "$$tmpdir"
endef

# Shared temp directory for atomic writes. Lives at the project root
# so all targets share the same filesystem, and is gitignored.
# Order-only prerequisite: recipes that need it depend on | _gen
_gen:
	mkdir -p _gen

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

POSTGRES_VERSION ?= 17
POSTGRES_IMAGE   ?= us-docker.pkg.dev/coder-v2-images-public/public/postgres:$(POSTGRES_VERSION)

# Limit parallel Make jobs in pre-commit/pre-push. Defaults to
# nproc/4 (min 2) since test and lint targets have internal
# parallelism. Override: make pre-push PARALLEL_JOBS=8
PARALLEL_JOBS ?= $(shell n=$$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 8); echo $$(( n / 4 > 2 ? n / 4 : 2 )))

# Use the highest ZSTD compression level in release builds to
# minimize artifact size. For non-release CI builds (e.g. main
# branch preview), use multithreaded level 6 which is ~99% faster
# at the cost of ~30% larger archives.
ifeq ($(CODER_RELEASE),true)
ZSTDFLAGS := -22 --ultra
else
ZSTDFLAGS := -6 -T0
endif

# Common paths to exclude from find commands, this rule is written so
# that it can be it can be used in a chain of AND statements (meaning
# you can simply write `find . $(FIND_EXCLUSIONS) -name thing-i-want`).
# Note, all find statements should be written with `.` or `./path` as
# the search path so that these exclusions match.
FIND_EXCLUSIONS= \
	-not \( \( -path '*/.git/*' -o -path './build/*' -o -path './vendor/*' -o -path './.coderv2/*' -o -path '*/node_modules/*' -o -path '*/out/*' -o -path './coderd/apidoc/*' -o -path '*/.next/*' -o -path '*/.terraform/*' -o -path './_gen/*' \) -prune \)
# Source files used for make targets, evaluated on use.
GO_SRC_FILES := $(shell find . $(FIND_EXCLUSIONS) -type f -name '*.go' -not -name '*_test.go')
# Same as GO_SRC_FILES but excluding certain files that have problematic
# Makefile dependencies (e.g. pnpm).
MOST_GO_SRC_FILES := $(shell \
	find . \
		$(FIND_EXCLUSIONS) \
		-type f \
		-name '*.go' \
		-not -name '*_test.go' \
		-not -wholename './agent/agentcontainers/dcspec/dcspec_gen.go' \
)
# All the shell files in the repo, excluding ignored files.
SHELL_SRC_FILES := $(shell find . $(FIND_EXCLUSIONS) -type f -name '*.sh')

MIGRATION_FILES := $(shell find ./coderd/database/migrations/ -maxdepth 1 $(FIND_EXCLUSIONS) -type f -name '*.sql')
FIXTURE_FILES := $(shell find ./coderd/database/migrations/testdata/fixtures/ $(FIND_EXCLUSIONS) -type f -name '*.sql')

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

		if [[ "$${CODER_SIGN_GPG:-0}" == "1" ]]; then
			cp "$@.asc" "./site/out/bin/coder-$$os-$$arch$$dot_ext.asc"
		fi
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

scripts/apidocgen/node_modules/.installed: scripts/apidocgen/package.json scripts/apidocgen/pnpm-lock.yaml
	(cd scripts/apidocgen && ../../scripts/pnpm_install.sh)
	touch "$@"

SITE_GEN_FILES := \
	site/src/api/typesGenerated.ts \
	site/src/api/rbacresourcesGenerated.ts \
	site/src/api/countriesGenerated.ts \
	site/src/api/chatModelOptionsGenerated.json \
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
ifdef FILE
	# Format single file
	if [[ -f "$(FILE)" ]] && [[ "$(FILE)" == *.go ]] && ! grep -q "DO NOT EDIT" "$(FILE)"; then \
		echo "$(GREEN)==>$(RESET) $(BOLD)fmt/go$(RESET) $(FILE)"; \
		./scripts/format_go_file.sh "$(FILE)"; \
	fi
else
	go mod tidy
	echo "$(GREEN)==>$(RESET) $(BOLD)fmt/go$(RESET)"
	# VS Code users should check out
	# https://github.com/mvdan/gofumpt#visual-studio-code
	find . $(FIND_EXCLUSIONS) -type f -name '*.go' -print0 | \
		xargs -0 grep -E --null -L '^// Code generated .* DO NOT EDIT\.$$' | \
		xargs -0 ./scripts/format_go_file.sh
endif
.PHONY: fmt/go

fmt/ts: site/node_modules/.installed
ifdef FILE
	# Format single TypeScript/JavaScript file
	if [[ -f "$(FILE)" ]] && [[ "$(FILE)" == *.ts ]] || [[ "$(FILE)" == *.tsx ]] || [[ "$(FILE)" == *.js ]] || [[ "$(FILE)" == *.jsx ]]; then \
		echo "$(GREEN)==>$(RESET) $(BOLD)fmt/ts$(RESET) $(FILE)"; \
		(cd site/ && pnpm exec biome format --write "../$(FILE)"); \
	fi
else
	echo "$(GREEN)==>$(RESET) $(BOLD)fmt/ts$(RESET)"
	cd site
# Avoid writing files in CI to reduce file write activity
ifdef CI
	pnpm run check --linter-enabled=false
else
	pnpm run check:fix
endif
endif
.PHONY: fmt/ts

fmt/biome: site/node_modules/.installed
ifdef FILE
	# Format single file with biome
	if [[ -f "$(FILE)" ]] && [[ "$(FILE)" == *.ts ]] || [[ "$(FILE)" == *.tsx ]] || [[ "$(FILE)" == *.js ]] || [[ "$(FILE)" == *.jsx ]]; then \
		echo "$(GREEN)==>$(RESET) $(BOLD)fmt/biome$(RESET) $(FILE)"; \
		(cd site/ && pnpm exec biome format --write "../$(FILE)"); \
	fi
else
	echo "$(GREEN)==>$(RESET) $(BOLD)fmt/biome$(RESET)"
	cd site/
# Avoid writing files in CI to reduce file write activity
ifdef CI
	pnpm run format:check
else
	pnpm run format
endif
endif
.PHONY: fmt/biome

fmt/terraform: $(wildcard *.tf)
ifdef FILE
	# Format single Terraform file
	if [[ -f "$(FILE)" ]] && [[ "$(FILE)" == *.tf ]] || [[ "$(FILE)" == *.tfvars ]]; then \
		echo "$(GREEN)==>$(RESET) $(BOLD)fmt/terraform$(RESET) $(FILE)"; \
		terraform fmt "$(FILE)"; \
	fi
else
	echo "$(GREEN)==>$(RESET) $(BOLD)fmt/terraform$(RESET)"
	terraform fmt -recursive
endif
.PHONY: fmt/terraform

fmt/shfmt: $(SHELL_SRC_FILES)
ifdef FILE
	# Format single shell script
	if [[ -f "$(FILE)" ]] && [[ "$(FILE)" == *.sh ]]; then \
		echo "$(GREEN)==>$(RESET) $(BOLD)fmt/shfmt$(RESET) $(FILE)"; \
		shfmt -w "$(FILE)"; \
	fi
else
	echo "$(GREEN)==>$(RESET) $(BOLD)fmt/shfmt$(RESET)"
# Only do diff check in CI, errors on diff.
ifdef CI
	shfmt -d $(SHELL_SRC_FILES)
else
	shfmt -w $(SHELL_SRC_FILES)
endif
endif
.PHONY: fmt/shfmt

fmt/markdown: node_modules/.installed
ifdef FILE
	# Format single markdown file
	if [[ -f "$(FILE)" ]] && [[ "$(FILE)" == *.md ]]; then \
		echo "$(GREEN)==>$(RESET) $(BOLD)fmt/markdown$(RESET) $(FILE)"; \
		pnpm exec markdown-table-formatter "$(FILE)"; \
	fi
else
	echo "$(GREEN)==>$(RESET) $(BOLD)fmt/markdown$(RESET)"
	pnpm format-docs
endif
.PHONY: fmt/markdown

# Note: we don't run zizmor in the lint target because it takes a while.
# GitHub Actions linters are run in a separate CI job (lint-actions) that only
# triggers when workflow files change, so we skip them here when CI=true.
LINT_ACTIONS_TARGETS := $(if $(CI),,lint/actions/actionlint)
lint: lint/shellcheck lint/go lint/ts lint/examples lint/helm lint/site-icons lint/markdown lint/check-scopes lint/migrations lint/bootstrap $(LINT_ACTIONS_TARGETS)
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
	linter_ver=$$(grep -oE 'GOLANGCI_LINT_VERSION=\S+' dogfood/coder/Dockerfile | cut -d '=' -f 2)
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@v$$linter_ver run
	go tool github.com/coder/paralleltestctx/cmd/paralleltestctx -custom-funcs="testutil.Context" ./...
.PHONY: lint/go

lint/examples:
	go run ./scripts/examplegen/main.go -lint
.PHONY: lint/examples

# Use shfmt to determine the shell files, takes editorconfig into consideration.
lint/shellcheck: $(SHELL_SRC_FILES)
	echo "--- shellcheck"
	shellcheck --external-sources $(SHELL_SRC_FILES)
.PHONY: lint/shellcheck

lint/bootstrap:
	bash scripts/check_bootstrap_quotes.sh
.PHONY: lint/bootstrap


lint/helm:
	cd helm/
	make lint
.PHONY: lint/helm

lint/markdown: node_modules/.installed
	pnpm lint-docs
.PHONY: lint/markdown

lint/actions: lint/actions/actionlint lint/actions/zizmor
.PHONY: lint/actions

lint/actions/actionlint:
	go tool github.com/rhysd/actionlint/cmd/actionlint
.PHONY: lint/actions/actionlint

lint/actions/zizmor:
	./scripts/zizmor.sh \
		--strict-collection \
		--persona=regular \
		.
.PHONY: lint/actions/zizmor

# Verify api_key_scope enum contains all RBAC <resource>:<action> values.
lint/check-scopes: coderd/database/dump.sql
	go run ./scripts/check-scopes
.PHONY: lint/check-scopes

# Verify migrations do not hardcode the public schema.
lint/migrations:
	./scripts/check_pg_schema.sh "Migrations" $(MIGRATION_FILES)
	./scripts/check_pg_schema.sh "Fixtures" $(FIXTURE_FILES)
.PHONY: lint/migrations

TYPOS_VERSION := $(shell grep -oP 'crate-ci/typos@\S+\s+\#\s+v\K[0-9.]+' .github/workflows/ci.yaml)

# Map uname values to typos release asset names.
TYPOS_ARCH := $(shell uname -m)
ifeq ($(shell uname -s),Darwin)
TYPOS_OS := apple-darwin
else
TYPOS_OS := unknown-linux-musl
endif

build/typos-$(TYPOS_VERSION):
	mkdir -p build/
	curl -sSfL "https://github.com/crate-ci/typos/releases/download/v$(TYPOS_VERSION)/typos-v$(TYPOS_VERSION)-$(TYPOS_ARCH)-$(TYPOS_OS).tar.gz" \
		| tar -xzf - -C build/ ./typos
	mv build/typos "$@"

lint/typos: build/typos-$(TYPOS_VERSION)
	build/typos-$(TYPOS_VERSION) --config .github/workflows/typos.toml
.PHONY: lint/typos

# pre-commit and pre-push mirror CI "required" jobs locally.
# See the "required" job's needs list in .github/workflows/ci.yaml.
#
# pre-commit runs checks that don't need external services (Docker,
# Playwright). This is the git pre-commit hook default since test
# and Docker failures in the local environment would otherwise block
# all commits.
#
# pre-push runs the full CI suite including tests. This is the git
# pre-push hook default, catching everything CI would before pushing.
#
# pre-push uses two-phase execution: gen+fmt+test-postgres-docker
# first (writes files, starts Docker), then lint+build+test in
# parallel. pre-commit uses two phases: gen+fmt first, then
# lint+build. This avoids races where gen's `go run` creates
# temporary .go files that lint's find-based checks pick up.
# Within each phase, targets run in parallel via -j. Both fail if
# any tracked files have unstaged changes afterward.
#
# Both pre-commit and pre-push:
#   gen, fmt, lint, lint/typos, slim binary (local arch)
#
# pre-push only (need external services or are slow):
#   site/out/index.html (pnpm build)
#   test-postgres-docker + test (needs Docker)
#   test-js, test-e2e (needs Playwright)
#   sqlc-vet (needs Docker)
#   offlinedocs/check
#
# Omitted:
#   test-go-pg-17 (same tests, different PG version)

define check-unstaged
	unstaged="$$(git diff --name-only)"
	if [[ -n $$unstaged ]]; then
		echo "ERROR: unstaged changes in tracked files:"
		echo "$$unstaged"
		echo
		echo "Review each change (git diff), verify correctness, then stage:"
		echo "  git add -u && git commit"
		exit 1
	fi
	untracked=$$(git ls-files --other --exclude-standard)
	if [[ -n $$untracked ]]; then
		echo "WARNING: untracked files (not in this commit, won't be in CI):"
		echo "$$untracked"
		echo
	fi
endef

pre-commit:
	start=$$(date +%s)
	echo "=== Phase 1/2: gen + fmt ==="
	$(MAKE) -j$(PARALLEL_JOBS) --output-sync=target MAKE_TIMED=1 gen fmt
	$(check-unstaged)
	echo "=== Phase 2/2: lint + build ==="
	$(MAKE) -j$(PARALLEL_JOBS) --output-sync=target MAKE_TIMED=1 \
		lint \
		lint/typos \
		build/coder-slim_$(GOOS)_$(GOARCH)$(GOOS_BIN_EXT)
	$(check-unstaged)
	echo "$(BOLD)$(GREEN)=== pre-commit passed in $$(( $$(date +%s) - $$start ))s ===$(RESET)"
.PHONY: pre-commit

pre-push:
	start=$$(date +%s)
	echo "=== Phase 1/2: gen + fmt + postgres ==="
	$(MAKE) -j$(PARALLEL_JOBS) --output-sync=target MAKE_TIMED=1 gen fmt test-postgres-docker
	$(check-unstaged)
	echo "=== Phase 2/2: lint + build + test ==="
	$(MAKE) -j$(PARALLEL_JOBS) --output-sync=target MAKE_TIMED=1 \
		lint \
		lint/typos \
		build/coder-slim_$(GOOS)_$(GOARCH)$(GOOS_BIN_EXT) \
		site/out/index.html \
		test \
		test-js \
		test-e2e \
		test-race \
		sqlc-vet \
		offlinedocs/check
	$(check-unstaged)
	echo "$(BOLD)$(GREEN)=== pre-push passed in $$(( $$(date +%s) - $$start ))s ===$(RESET)"
.PHONY: pre-push

offlinedocs/check: offlinedocs/node_modules/.installed
	cd offlinedocs/
	pnpm format:check
	pnpm lint
	pnpm export
.PHONY: offlinedocs/check

# All files generated by the database should be added here, and this can be used
# as a target for jobs that need to run after the database is generated.
DB_GEN_FILES := \
	coderd/database/dump.sql \
	coderd/database/querier.go \
	coderd/database/unique_constraint.go \
	coderd/database/dbmetrics/querymetrics.go \
	coderd/database/dbauthz/dbauthz.go \
	coderd/database/dbmock/dbmock.go

TAILNETTEST_MOCKS := \
	tailnet/tailnettest/coordinatormock.go \
	tailnet/tailnettest/coordinateemock.go \
	tailnet/tailnettest/workspaceupdatesprovidermock.go \
	tailnet/tailnettest/subscriptionmock.go

AIBRIDGED_MOCKS := \
	enterprise/aibridged/aibridgedmock/clientmock.go \
	enterprise/aibridged/aibridgedmock/poolmock.go

GEN_FILES := \
	tailnet/proto/tailnet.pb.go \
	agent/proto/agent.pb.go \
	agent/agentsocket/proto/agentsocket.pb.go \
	agent/boundarylogproxy/codec/boundary.pb.go \
	provisionersdk/proto/provisioner.pb.go \
	provisionerd/proto/provisionerd.pb.go \
	vpn/vpn.pb.go \
	enterprise/aibridged/proto/aibridged.pb.go \
	$(DB_GEN_FILES) \
	$(SITE_GEN_FILES) \
	coderd/rbac/object_gen.go \
	codersdk/rbacresources_gen.go \
	coderd/rbac/scopes_constants_gen.go \
	codersdk/apikey_scopes_gen.go \
	docs/admin/integrations/prometheus.md \
	docs/reference/cli/index.md \
	docs/admin/security/audit-logs.md \
	coderd/apidoc/swagger.json \
	docs/manifest.json \
	provisioner/terraform/testdata/version \
	scripts/metricsdocgen/generated_metrics \
	site/e2e/provisionerGenerated.ts \
	examples/examples.gen.json \
	$(TAILNETTEST_MOCKS) \
	coderd/database/pubsub/psmock/psmock.go \
	agent/agentcontainers/acmock/acmock.go \
	agent/agentcontainers/dcspec/dcspec_gen.go \
	coderd/httpmw/loggermw/loggermock/loggermock.go \
	codersdk/workspacesdk/agentconnmock/agentconnmock.go \
	$(AIBRIDGED_MOCKS)

# all gen targets should be added here and to gen/mark-fresh
gen: gen/db gen/golden-files $(GEN_FILES)
.PHONY: gen

gen/db: $(DB_GEN_FILES)
.PHONY: gen/db

gen/golden-files: \
	agent/unit/testdata/.gen-golden \
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
		agent/agentsocket/proto/agentsocket.pb.go \
		agent/boundarylogproxy/codec/boundary.pb.go \
		vpn/vpn.pb.go \
		enterprise/aibridged/proto/aibridged.pb.go \
		coderd/database/dump.sql \
		coderd/database/querier.go \
		coderd/database/unique_constraint.go \
		coderd/database/dbmetrics/querymetrics.go \
		coderd/database/dbauthz/dbauthz.go \
		coderd/database/dbmock/dbmock.go \
		coderd/database/pubsub/psmock/psmock.go \
		site/src/api/typesGenerated.ts \
		coderd/rbac/object_gen.go \
		codersdk/rbacresources_gen.go \
		coderd/rbac/scopes_constants_gen.go \
		codersdk/apikey_scopes_gen.go \
		site/src/api/rbacresourcesGenerated.ts \
		site/src/api/countriesGenerated.ts \
		site/src/api/chatModelOptionsGenerated.json \
		docs/admin/integrations/prometheus.md \
		docs/reference/cli/index.md \
		docs/admin/security/audit-logs.md \
		coderd/apidoc/swagger.json \
		docs/manifest.json \
		site/e2e/provisionerGenerated.ts \
		site/src/theme/icons.json \
		examples/examples.gen.json \
		scripts/metricsdocgen/generated_metrics \
		$(TAILNETTEST_MOCKS) \
		agent/agentcontainers/acmock/acmock.go \
		agent/agentcontainers/dcspec/dcspec_gen.go \
		coderd/httpmw/loggermw/loggermock/loggermock.go \
		codersdk/workspacesdk/agentconnmock/agentconnmock.go \
		$(AIBRIDGED_MOCKS) \
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
#
# NOTE: grouped target (&:) ensures generate.sh runs only once even
# with -j and all outputs are considered produced together. These
# files are all written by generate.sh (via sqlc and scripts/dbgen).
coderd/database/querier.go \
coderd/database/unique_constraint.go \
coderd/database/dbmetrics/querymetrics.go \
coderd/database/dbauthz/dbauthz.go &: \
	coderd/database/sqlc.yaml \
	coderd/database/dump.sql \
	$(wildcard coderd/database/queries/*.sql)
	SKIP_DUMP_SQL=1 ./coderd/database/generate.sh
	touch coderd/database/querier.go coderd/database/unique_constraint.go coderd/database/dbmetrics/querymetrics.go coderd/database/dbauthz/dbauthz.go

coderd/database/dbmock/dbmock.go: coderd/database/db.go coderd/database/querier.go
	go generate ./coderd/database/dbmock/
	touch "$@"

coderd/database/pubsub/psmock/psmock.go: coderd/database/pubsub/pubsub.go
	go generate ./coderd/database/pubsub/psmock
	touch "$@"

agent/agentcontainers/acmock/acmock.go: agent/agentcontainers/containers.go
	go generate ./agent/agentcontainers/acmock/
	touch "$@"

coderd/httpmw/loggermw/loggermock/loggermock.go: coderd/httpmw/loggermw/logger.go
	go generate ./coderd/httpmw/loggermw/loggermock/
	touch "$@"

codersdk/workspacesdk/agentconnmock/agentconnmock.go: codersdk/workspacesdk/agentconn.go
	go generate ./codersdk/workspacesdk/agentconnmock/
	touch "$@"

$(AIBRIDGED_MOCKS): enterprise/aibridged/client.go enterprise/aibridged/pool.go
	go generate ./enterprise/aibridged/aibridgedmock/
	touch "$@"

agent/agentcontainers/dcspec/dcspec_gen.go: \
	node_modules/.installed \
	agent/agentcontainers/dcspec/devContainer.base.schema.json \
	agent/agentcontainers/dcspec/gen.sh \
	agent/agentcontainers/dcspec/doc.go
	DCSPEC_QUIET=true go generate ./agent/agentcontainers/dcspec/
	touch "$@"

$(TAILNETTEST_MOCKS): tailnet/coordinator.go tailnet/service.go
	go generate ./tailnet/tailnettest/
	touch "$@"

tailnet/proto/tailnet.pb.go: tailnet/proto/tailnet.proto
	./scripts/atomic_protoc.sh \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-drpc_out=. \
		--go-drpc_opt=paths=source_relative \
		./tailnet/proto/tailnet.proto

agent/proto/agent.pb.go: agent/proto/agent.proto
	./scripts/atomic_protoc.sh \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-drpc_out=. \
		--go-drpc_opt=paths=source_relative \
		./agent/proto/agent.proto

agent/agentsocket/proto/agentsocket.pb.go: agent/agentsocket/proto/agentsocket.proto agent/proto/agent.proto
	./scripts/atomic_protoc.sh \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-drpc_out=. \
		--go-drpc_opt=paths=source_relative \
		./agent/agentsocket/proto/agentsocket.proto

provisionersdk/proto/provisioner.pb.go: provisionersdk/proto/provisioner.proto
	./scripts/atomic_protoc.sh \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-drpc_out=. \
		--go-drpc_opt=paths=source_relative \
		./provisionersdk/proto/provisioner.proto

provisionerd/proto/provisionerd.pb.go: provisionerd/proto/provisionerd.proto
	./scripts/atomic_protoc.sh \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-drpc_out=. \
		--go-drpc_opt=paths=source_relative \
		./provisionerd/proto/provisionerd.proto

vpn/vpn.pb.go: vpn/vpn.proto
	./scripts/atomic_protoc.sh \
		--go_out=. \
		--go_opt=paths=source_relative \
		./vpn/vpn.proto

agent/boundarylogproxy/codec/boundary.pb.go: agent/boundarylogproxy/codec/boundary.proto agent/proto/agent.proto
	./scripts/atomic_protoc.sh \
		--go_out=. \
		--go_opt=paths=source_relative \
		./agent/boundarylogproxy/codec/boundary.proto

enterprise/aibridged/proto/aibridged.pb.go: enterprise/aibridged/proto/aibridged.proto
	./scripts/atomic_protoc.sh \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-drpc_out=. \
		--go-drpc_opt=paths=source_relative \
		./enterprise/aibridged/proto/aibridged.proto

site/src/api/typesGenerated.ts: site/node_modules/.installed $(wildcard scripts/apitypings/*) $(shell find ./codersdk $(FIND_EXCLUSIONS) -type f -name '*.go') | _gen
	$(call atomic_write,go run -C ./scripts/apitypings main.go,./scripts/biome_format.sh)

site/e2e/provisionerGenerated.ts: site/node_modules/.installed provisionerd/proto/provisionerd.pb.go provisionersdk/proto/provisioner.pb.go
	(cd site/ && pnpm run gen:provisioner)
	touch "$@"

site/src/theme/icons.json: site/node_modules/.installed $(wildcard scripts/gensite/*) $(wildcard site/static/icon/*) | _gen
	tmpdir=$$(mktemp -d -p _gen) && tmpfile=$$(realpath "$$tmpdir")/$(notdir $@) && \
		go run ./scripts/gensite/ -icons "$$tmpfile" && \
		./scripts/biome_format.sh "$$tmpfile" && \
		mv "$$tmpfile" "$@" && rm -rf "$$tmpdir"

examples/examples.gen.json: scripts/examplegen/main.go examples/examples.go $(shell find ./examples/templates) | _gen
	$(call atomic_write,go run ./scripts/examplegen/main.go)

coderd/rbac/object_gen.go: scripts/typegen/rbacobject.gotmpl scripts/typegen/main.go coderd/rbac/object.go coderd/rbac/policy/policy.go | _gen
	$(call atomic_write,go run ./scripts/typegen/main.go rbac object)
	touch "$@"

# NOTE: depends on object_gen.go because `go run` compiles
# coderd/rbac which includes it.
coderd/rbac/scopes_constants_gen.go: scripts/typegen/scopenames.gotmpl scripts/typegen/main.go coderd/rbac/policy/policy.go \
	coderd/rbac/object_gen.go | _gen
	# Write to a temp file first to avoid truncating the package
	# during build since the generator imports the rbac package.
	$(call atomic_write,go run ./scripts/typegen/main.go rbac scopenames)
	touch "$@"

# NOTE: depends on object_gen.go and scopes_constants_gen.go because
# `go run` compiles coderd/rbac which includes both.
codersdk/rbacresources_gen.go: scripts/typegen/codersdk.gotmpl scripts/typegen/main.go coderd/rbac/object.go coderd/rbac/policy/policy.go \
	coderd/rbac/object_gen.go coderd/rbac/scopes_constants_gen.go | _gen
	# Write to a temp file to avoid truncating the target, which
	# would break the codersdk package and any parallel build targets.
	$(call atomic_write,go run scripts/typegen/main.go rbac codersdk)
	touch "$@"

# NOTE: depends on object_gen.go and scopes_constants_gen.go because
# `go run` compiles coderd/rbac which includes both.
codersdk/apikey_scopes_gen.go: scripts/apikeyscopesgen/main.go coderd/rbac/scopes_catalog.go coderd/rbac/scopes.go \
	coderd/rbac/object_gen.go coderd/rbac/scopes_constants_gen.go | _gen
	# Generate SDK constants for external API key scopes.
	$(call atomic_write,go run ./scripts/apikeyscopesgen)
	touch "$@"

# NOTE: depends on object_gen.go and scopes_constants_gen.go because
# `go run` compiles coderd/rbac which includes both.
site/src/api/rbacresourcesGenerated.ts: site/node_modules/.installed scripts/typegen/codersdk.gotmpl scripts/typegen/main.go coderd/rbac/object.go coderd/rbac/policy/policy.go \
	coderd/rbac/object_gen.go coderd/rbac/scopes_constants_gen.go | _gen
	$(call atomic_write,go run scripts/typegen/main.go rbac typescript,./scripts/biome_format.sh)

site/src/api/countriesGenerated.ts: site/node_modules/.installed scripts/typegen/countries.tstmpl scripts/typegen/main.go codersdk/countries.go | _gen
	$(call atomic_write,go run scripts/typegen/main.go countries,./scripts/biome_format.sh)

site/src/api/chatModelOptionsGenerated.json: scripts/modeloptionsgen/main.go codersdk/chats.go | _gen
	$(call atomic_write,go run ./scripts/modeloptionsgen/main.go | tail -n +2,./scripts/biome_format.sh)

scripts/metricsdocgen/generated_metrics: $(GO_SRC_FILES) | _gen
	$(call atomic_write,go run ./scripts/metricsdocgen/scanner)

docs/admin/integrations/prometheus.md: node_modules/.installed scripts/metricsdocgen/main.go scripts/metricsdocgen/metrics scripts/metricsdocgen/generated_metrics | _gen
	tmpdir=$$(mktemp -d -p _gen) && tmpfile=$$(realpath "$$tmpdir")/$(notdir $@) && cp "$@" "$$tmpfile" && \
		go run scripts/metricsdocgen/main.go --prometheus-doc-file="$$tmpfile" && \
		pnpm exec markdownlint-cli2 --fix "$$tmpfile" && \
		pnpm exec markdown-table-formatter "$$tmpfile" && \
		mv "$$tmpfile" "$@" && rm -rf "$$tmpdir"

docs/reference/cli/index.md: node_modules/.installed scripts/clidocgen/main.go examples/examples.gen.json $(GO_SRC_FILES) | _gen
	tmpdir=$$(mktemp -d -p _gen) && \
		tmpdir=$$(realpath "$$tmpdir") && \
		mkdir -p "$$tmpdir/docs/reference/cli" && \
		cp docs/manifest.json "$$tmpdir/docs/manifest.json" && \
		CI=true DOCS_DIR="$$tmpdir/docs" go run ./scripts/clidocgen && \
		pnpm exec markdownlint-cli2 --fix "$$tmpdir/docs/reference/cli/*.md" && \
		pnpm exec markdown-table-formatter "$$tmpdir/docs/reference/cli/*.md" && \
		for f in "$$tmpdir/docs/reference/cli/"*.md; do mv "$$f" "docs/reference/cli/$$(basename "$$f")"; done && \
		rm -rf "$$tmpdir"

docs/admin/security/audit-logs.md: node_modules/.installed coderd/database/querier.go scripts/auditdocgen/main.go enterprise/audit/table.go coderd/rbac/object_gen.go | _gen
	tmpdir=$$(mktemp -d -p _gen) && tmpfile=$$(realpath "$$tmpdir")/$(notdir $@) && cp "$@" "$$tmpfile" && \
		go run scripts/auditdocgen/main.go --audit-doc-file="$$tmpfile" && \
		pnpm exec markdownlint-cli2 --fix "$$tmpfile" && \
		pnpm exec markdown-table-formatter "$$tmpfile" && \
		mv "$$tmpfile" "$@" && rm -rf "$$tmpdir"

coderd/apidoc/.gen: \
	node_modules/.installed \
	scripts/apidocgen/node_modules/.installed \
	$(wildcard coderd/*.go) \
	$(wildcard enterprise/coderd/*.go) \
	$(wildcard codersdk/*.go) \
	$(wildcard enterprise/wsproxy/wsproxysdk/*.go) \
	$(DB_GEN_FILES) \
	coderd/rbac/object_gen.go \
	.swaggo \
	scripts/apidocgen/generate.sh \
	scripts/apidocgen/swaginit/main.go \
	$(wildcard scripts/apidocgen/postprocess/*) \
	$(wildcard scripts/apidocgen/markdown-template/*) | _gen
	tmpdir=$$(mktemp -d -p _gen) && swagtmp=$$(mktemp -d -p _gen) && \
		tmpdir=$$(realpath "$$tmpdir") && swagtmp=$$(realpath "$$swagtmp") && \
		mkdir -p "$$tmpdir/reference/api" && \
		cp docs/manifest.json "$$tmpdir/manifest.json" && \
		SWAG_OUTPUT_DIR="$$swagtmp" APIDOCGEN_DOCS_DIR="$$tmpdir" ./scripts/apidocgen/generate.sh && \
		pnpm exec markdownlint-cli2 --fix "$$tmpdir/reference/api/*.md" && \
		pnpm exec markdown-table-formatter "$$tmpdir/reference/api/*.md" && \
		./scripts/biome_format.sh "$$swagtmp/swagger.json" && \
		for f in "$$tmpdir/reference/api/"*.md; do mv "$$f" "docs/reference/api/$$(basename "$$f")"; done && \
		mv "$$tmpdir/manifest.json" _gen/manifest-staging.json && \
		mv "$$swagtmp/docs.go" coderd/apidoc/docs.go && \
		mv "$$swagtmp/swagger.json" coderd/apidoc/swagger.json && \
		rm -rf "$$tmpdir" "$$swagtmp"
	touch "$@"

docs/manifest.json: site/node_modules/.installed coderd/apidoc/.gen docs/reference/cli/index.md | _gen
	tmpdir=$$(mktemp -d -p _gen) && tmpfile=$$(realpath "$$tmpdir")/$(notdir $@) && \
		cp _gen/manifest-staging.json "$$tmpfile" && \
		./scripts/biome_format.sh "$$tmpfile" && \
		mv "$$tmpfile" "$@" && rm -rf "$$tmpdir"

coderd/apidoc/swagger.json: site/node_modules/.installed coderd/apidoc/.gen
	touch "$@"

update-golden-files:
	echo 'WARNING: This target is deprecated. Use "make gen/golden-files" instead.' >&2
	echo 'Running "make gen/golden-files"' >&2
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

agent/unit/testdata/.gen-golden: $(wildcard agent/unit/testdata/*.golden) $(GO_SRC_FILES) $(wildcard agent/unit/*_test.go)
	TZ=UTC go test ./agent/unit -run="TestGraph" -update
	touch "$@"

cli/testdata/.gen-golden: $(wildcard cli/testdata/*.golden) $(wildcard cli/*.tpl) $(GO_SRC_FILES) $(wildcard cli/*_test.go)
	TZ=UTC go test ./cli -run="Test(CommandHelp|ServerYAML|ErrorExamples|.*Golden)" -update
	touch "$@"

enterprise/cli/testdata/.gen-golden: $(wildcard enterprise/cli/testdata/*.golden) $(wildcard cli/*.tpl) $(GO_SRC_FILES) $(wildcard enterprise/cli/*_test.go)
	TZ=UTC go test ./enterprise/cli -run="TestEnterpriseCommandHelp" -update
	touch "$@"

tailnet/testdata/.gen-golden: $(wildcard tailnet/testdata/*.golden.html) $(GO_SRC_FILES) $(wildcard tailnet/*_test.go)
	TZ=UTC go test ./tailnet -run="TestDebugTemplate" -update
	touch "$@"

enterprise/tailnet/testdata/.gen-golden: $(wildcard enterprise/tailnet/testdata/*.golden.html) $(GO_SRC_FILES) $(wildcard enterprise/tailnet/*_test.go)
	TZ=UTC go test ./enterprise/tailnet -run="TestDebugTemplate" -update
	touch "$@"

helm/coder/tests/testdata/.gen-golden: $(wildcard helm/coder/tests/testdata/*.yaml) $(wildcard helm/coder/tests/testdata/*.golden) $(GO_SRC_FILES) $(wildcard helm/coder/tests/*_test.go)
	if command -v helm >/dev/null 2>&1; then
		TZ=UTC go test ./helm/coder/tests -run=TestUpdateGoldenFiles -update
	else
		echo "WARNING: helm not found; skipping helm/coder golden generation" >&2
	fi
	touch "$@"

helm/provisioner/tests/testdata/.gen-golden: $(wildcard helm/provisioner/tests/testdata/*.yaml) $(wildcard helm/provisioner/tests/testdata/*.golden) $(GO_SRC_FILES) $(wildcard helm/provisioner/tests/*_test.go)
	if command -v helm >/dev/null 2>&1; then
		TZ=UTC go test ./helm/provisioner/tests -run=TestUpdateGoldenFiles -update
	else
		echo "WARNING: helm not found; skipping helm/provisioner golden generation" >&2
	fi
	touch "$@"

coderd/.gen-golden: $(wildcard coderd/testdata/*/*.golden) $(GO_SRC_FILES) $(wildcard coderd/*_test.go)
	TZ=UTC go test ./coderd -run="Test.*Golden$$" -update
	touch "$@"

coderd/notifications/.gen-golden: $(wildcard coderd/notifications/testdata/*/*.golden) $(GO_SRC_FILES) $(wildcard coderd/notifications/*_test.go)
	TZ=UTC go test ./coderd/notifications -run="Test.*Golden$$" -update
	touch "$@"

provisioner/terraform/testdata/.gen-golden: $(wildcard provisioner/terraform/testdata/*/*.golden) $(GO_SRC_FILES) $(wildcard provisioner/terraform/*_test.go)
	TZ=UTC go test ./provisioner/terraform -run="Test.*Golden$$" -update
	touch "$@"

provisioner/terraform/testdata/version:
	if [[ "$(shell cat provisioner/terraform/testdata/version.txt)" != "$(shell terraform version -json | jq -r '.terraform_version')" ]]; then
		./provisioner/terraform/testdata/generate.sh
	fi
.PHONY: provisioner/terraform/testdata/version

# Set the retry flags if TEST_RETRIES is set
ifdef TEST_RETRIES
GOTESTSUM_RETRY_FLAGS := --rerun-fails=$(TEST_RETRIES)
else
GOTESTSUM_RETRY_FLAGS :=
endif

# Default to 8x8 parallelism to avoid overwhelming our workspaces.
# Race detection defaults to 4x4 because the detector adds significant
# CPU overhead. Override via TEST_NUM_PARALLEL_PACKAGES /
# TEST_NUM_PARALLEL_TESTS.
TEST_PARALLEL_PACKAGES := $(or $(TEST_NUM_PARALLEL_PACKAGES),8)
TEST_PARALLEL_TESTS := $(or $(TEST_NUM_PARALLEL_TESTS),8)
RACE_PARALLEL_PACKAGES := $(or $(TEST_NUM_PARALLEL_PACKAGES),4)
RACE_PARALLEL_TESTS := $(or $(TEST_NUM_PARALLEL_TESTS),4)

# Use testsmallbatch tag to reduce wireguard memory allocation in tests
# (from ~18GB to negligible). Recursively expanded so target-specific
# overrides of TEST_PARALLEL_* take effect (e.g. test-race lowers
# parallelism). CI job timeout is 25m (see test-go-pg in ci.yaml),
# keep the Go timeout 5m shorter so tests produce goroutine dumps
# instead of the CI runner killing the process with no output.
GOTEST_FLAGS = -tags=testsmallbatch -v -timeout 20m -p $(TEST_PARALLEL_PACKAGES) -parallel=$(TEST_PARALLEL_TESTS)

# The most common use is to set TEST_COUNT=1 to avoid Go's test cache.
ifdef TEST_COUNT
GOTEST_FLAGS += -count=$(TEST_COUNT)
endif

ifdef TEST_SHORT
GOTEST_FLAGS += -short
endif

ifdef RUN
GOTEST_FLAGS += -run $(RUN)
endif

ifdef TEST_CPUPROFILE
GOTEST_FLAGS += -cpuprofile=$(TEST_CPUPROFILE)
endif

ifdef TEST_MEMPROFILE
GOTEST_FLAGS += -memprofile=$(TEST_MEMPROFILE)
endif

TEST_PACKAGES ?= ./...

test:
	$(GIT_FLAGS) gotestsum --format standard-quiet \
		$(GOTESTSUM_RETRY_FLAGS) \
		--packages="$(TEST_PACKAGES)" \
		-- \
		$(GOTEST_FLAGS)
.PHONY: test

test-race: TEST_PARALLEL_PACKAGES := $(RACE_PARALLEL_PACKAGES)
test-race: TEST_PARALLEL_TESTS := $(RACE_PARALLEL_TESTS)
test-race:
	$(GIT_FLAGS) gotestsum --format standard-quiet \
		--junitfile="gotests.xml" \
		$(GOTESTSUM_RETRY_FLAGS) \
		--packages="$(TEST_PACKAGES)" \
		-- \
		-race \
		$(GOTEST_FLAGS)
.PHONY: test-race

test-cli:
	$(MAKE) test TEST_PACKAGES="./cli..."
.PHONY: test-cli

test-js: site/node_modules/.installed
	cd site/
	pnpm test:ci
.PHONY: test-js

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
	SQLC_DATABASE_URL="postgresql://postgres:postgres@localhost:5432/$$(go run scripts/migrate-ci/main.go)" \
	sqlc push -f coderd/database/sqlc.yaml && echo "Passed sqlc push"
.PHONY: sqlc-push

sqlc-verify: sqlc-cloud-is-setup test-postgres-docker
	echo "--- sqlc verify"
	SQLC_DATABASE_URL="postgresql://postgres:postgres@localhost:5432/$$(go run scripts/migrate-ci/main.go)" \
	sqlc verify -f coderd/database/sqlc.yaml && echo "Passed sqlc verify"
.PHONY: sqlc-verify

sqlc-vet: test-postgres-docker
	echo "--- sqlc vet"
	SQLC_DATABASE_URL="postgresql://postgres:postgres@localhost:5432/$$(go run scripts/migrate-ci/main.go)" \
	sqlc vet -f coderd/database/sqlc.yaml && echo "Passed sqlc vet"
.PHONY: sqlc-vet


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
	# If our container is already running, nothing to do.
	if docker ps --filter "name=test-postgres-docker-${POSTGRES_VERSION}" --format '{{.Names}}' | grep -q .; then \
		echo "test-postgres-docker-${POSTGRES_VERSION} is already running."; \
		exit 0; \
	fi
	# If something else is on 5432, warn but don't fail.
	if pg_isready -h 127.0.0.1 -q 2>/dev/null; then \
		echo "WARNING: PostgreSQL is already running on 127.0.0.1:5432 (not our container)."; \
		echo "Tests will use this instance. To use the Makefile's container, stop it first."; \
		exit 0; \
	fi
	docker rm -f test-postgres-docker-${POSTGRES_VERSION} || true

	# Try pulling up to three times to avoid CI flakes.
	docker pull ${POSTGRES_IMAGE} || {
		retries=2
		for try in $$(seq 1 $${retries}); do
			echo "Failed to pull image, retrying ($${try}/$${retries})..."
			sleep 1
			if docker pull ${POSTGRES_IMAGE}; then
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
		${POSTGRES_IMAGE} \
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
		echo "$$(date) - waiting for database to start"
		sleep 0.5
	done
.PHONY: test-postgres-docker

test-tailnet-integration:
	env \
		CODER_TAILNET_TESTS=true \
		CODER_MAGICSOCK_DEBUG_LOGGING=true \
		TS_DEBUG_NETCHECK=true \
		GOTRACEBACK=single \
		go test \
			-tags=testsmallbatch \
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
	pnpm playwright:install
ifdef CI
	DEBUG=pw:api pnpm playwright:test --forbid-only --workers 1
else
	pnpm playwright:test
endif
.PHONY: test-e2e

dogfood/coder/nix.hash: flake.nix flake.lock
	sha256sum flake.nix flake.lock >./dogfood/coder/nix.hash

# Count the number of test databases created per test package.
count-test-databases:
	PGPASSWORD=postgres psql -h localhost -U postgres -d coder_testing -P pager=off -c 'SELECT test_package, count(*) as count from test_databases GROUP BY test_package ORDER BY count DESC'
.PHONY: count-test-databases
