# Use the branch name to differentiate test builds from actual pulled images,
# replacing forward slashes with hyphens, as forward slashes are not valid in
# tag names.
build_tag ?= $(shell git rev-parse --abbrev-ref HEAD | sed "s/\\//-/")

# The base Dockerfile consumes the repo root as build context so it can
# reach the distro-specific files/ tree and configure-chrome-flags.sh
# under dogfood/coder/ubuntu-<release>/.
REPO_ROOT := $(shell git rev-parse --show-toplevel)

# Pick a container runtime. On macOS we prefer Apple's `container` CLI
# when present (it produces a Linux VM-backed amd64 image without
# Docker Desktop); otherwise fall back to docker. Linux always uses
# docker.
OS := $(shell uname -s)
ifeq ($(OS),Darwin)
  CONTAINER_RUNTIME ?= $(shell command -v container >/dev/null 2>&1 && echo container || echo docker)
else
  CONTAINER_RUNTIME ?= docker
endif

# Apple's `container` defaults to the host arch; the dogfood image is
# amd64-only, so pin it.
ifeq ($(CONTAINER_RUNTIME),container)
  PLATFORM_ARG := --platform linux/amd64
else
  PLATFORM_ARG :=
endif

ifeq ($(OS),Linux)
  # `mise oci build` packages already-installed tools; the install
  # has to run first. The macOS wrapper does this inside the
  # container; on Linux we chain it here.
  MISE_OCI := mise install --yes && MISE_EXPERIMENTAL=1 mise oci
else
  MISE_OCI := CONTAINER_RUNTIME=$(CONTAINER_RUNTIME) $(REPO_ROOT)/scripts/dogfood/mise-oci-wrapper.sh
endif

.PHONY: build build-ubuntu-22.04 build-ubuntu-26.04 \
	build-base-ubuntu-22.04 build-base-ubuntu-26.04 \
	update-keys update-keys-ubuntu-22.04 update-keys-ubuntu-26.04

build: build-ubuntu-22.04 build-ubuntu-26.04

# Caveat: `build-ubuntu-*` requires the base image to be pullable from a
# registry that `mise oci`'s HTTPS client can reach (ghcr.io, a local
# `registry:2` sidecar, etc.). `--from coderdev/oss-dogfood-base:*-local`
# only resolves when a registry mirror is set up alongside; without it,
# `mise oci build` fails because the wrapper container cannot see the
# host's local image store. The `build-base-ubuntu-*` targets on their
# own work end to end without any registry. See
# scripts/dogfood/mise-oci-wrapper.sh for the full story.
build-base-ubuntu-22.04:
	$(CONTAINER_RUNTIME) build $(PLATFORM_ARG) \
		-f "$(REPO_ROOT)/dogfood/coder/ubuntu-22.04/Dockerfile.base" \
		-t "coderdev/oss-dogfood-base:22.04-local" \
		"$(REPO_ROOT)"

build-base-ubuntu-26.04:
	$(CONTAINER_RUNTIME) build $(PLATFORM_ARG) \
		-f "$(REPO_ROOT)/dogfood/coder/ubuntu-26.04/Dockerfile.base" \
		-t "coderdev/oss-dogfood-base:26.04-local" \
		"$(REPO_ROOT)"

build-ubuntu-22.04: build-base-ubuntu-22.04
	$(MISE_OCI) build \
		--from "coderdev/oss-dogfood-base:22.04-local" \
		--tag "codercom/oss-dogfood:22.04-$(build_tag)"

build-ubuntu-26.04: build-base-ubuntu-26.04
	$(MISE_OCI) build \
		--from "coderdev/oss-dogfood-base:26.04-local" \
		--tag "codercom/oss-dogfood:26.04-$(build_tag)"

update-keys: update-keys-ubuntu-22.04 update-keys-ubuntu-26.04

update-keys-ubuntu-22.04:
	./ubuntu-22.04/update-keys.sh

update-keys-ubuntu-26.04:
	./ubuntu-26.04/update-keys.sh
