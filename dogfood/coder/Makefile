# Use the branch name to differentiate test builds from actual pulled images,
# replacing forward slashes with hyphens, as forward slashes are not valid in
# tag names.
build_tag ?= $(shell git rev-parse --abbrev-ref HEAD | sed "s/\\//-/")

# The Dockerfiles consume the repo root as build context so they can
# reach the project mise.toml. Each variant still tracks its own
# files/ tree under dogfood/coder/ubuntu-<release>/.
REPO_ROOT := $(shell git rev-parse --show-toplevel)

# Mise's aqua backend exhausts GitHub's unauthenticated API quota
# quickly. Plumb a token through to the mise install layer when one
# is available. Two equivalent ways to supply it:
#   GITHUB_TOKEN=ghp_...        - taken straight from the environment
#                                 (matches GitHub Actions, where
#                                 secrets.GITHUB_TOKEN is auto-provided)
#   GITHUB_TOKEN_FILE=/path     - read the token from a file
# If neither is set the build still runs but may hit 403s.
ifneq ($(GITHUB_TOKEN_FILE),)
docker_secret_arg := --secret id=github_token,src="$(GITHUB_TOKEN_FILE)"
else ifneq ($(GITHUB_TOKEN),)
docker_secret_arg := --secret id=github_token,env=GITHUB_TOKEN
endif

build: build-ubuntu-22.04 build-ubuntu-26.04
.PHONY: build

build-ubuntu-22.04:
	DOCKER_BUILDKIT=1 docker build \
		-f dogfood/coder/ubuntu-22.04/Dockerfile \
		-t "codercom/oss-dogfood:22.04-$(build_tag)" \
		$(docker_secret_arg) \
		"$(REPO_ROOT)"
.PHONY: build-ubuntu-22.04

build-ubuntu-26.04:
	DOCKER_BUILDKIT=1 docker build \
		-f dogfood/coder/ubuntu-26.04/Dockerfile \
		-t "codercom/oss-dogfood:26.04-$(build_tag)" \
		$(docker_secret_arg) \
		"$(REPO_ROOT)"
.PHONY: build-ubuntu-26.04

push: push-ubuntu-22.04 push-ubuntu-26.04
.PHONY: push

push-ubuntu-22.04: build-ubuntu-22.04
	docker push ${build_tag}
.PHONY: push-ubuntu-22.04

push-ubuntu-26.04: build-ubuntu-26.04
	docker push ${build_tag}
.PHONY: push-ubuntu-26.04

update-keys: update-keys-ubuntu-22.04 update-keys-ubuntu-26.04
.PHONY: update-keys

update-keys-ubuntu-22.04:
	./ubuntu-22.04/update-keys.sh
.PHONY: update-keys-ubuntu-22.04

update-keys-ubuntu-26.04:
	./ubuntu-26.04/update-keys.sh
.PHONY: update-keys-ubuntu-26.04
