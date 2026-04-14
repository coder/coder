# Use the branch name to differentiate test builds from actual pulled images,
# replacing forward slashes with hyphens, as forward slashes are not valid in
# tag names.
build_tag ?= $(shell git rev-parse --abbrev-ref HEAD | sed "s/\\//-/")

build: build-ubuntu-22.04 build-ubuntu-26.04
.PHONY: build

build-ubuntu-22.04:
	(cd ubuntu-22.04/ && DOCKER_BUILDKIT=1 docker build . -t "codercom/oss-dogfood:22.04-$(build_tag)")
.PHONY: build-ubuntu-22.04

build-ubuntu-26.04:
	(cd ubuntu-26.04/ && DOCKER_BUILDKIT=1 docker build . -t "codercom/oss-dogfood:26.04-$(build_tag)")
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
