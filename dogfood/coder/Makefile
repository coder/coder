branch := $(shell git rev-parse --abbrev-ref HEAD)
build_tag := codercom/oss-dogfood:${branch}

build:
	DOCKER_BUILDKIT=1 docker build . -t ${build_tag}
.PHONY: build

push: build
	docker push ${build_tag}
.PHONY: push

update-keys: update-keys-ubuntu-22.04 update-keys-ubuntu-26.04
.PHONY: update-keys

update-keys-ubuntu-22.04:
	./ubuntu-22.04/update-keys.sh
.PHONY: update-keys-ubuntu-22.04

update-keys-ubuntu-26.04:
	./ubuntu-26.04/update-keys.sh
.PHONY: update-keys-ubuntu-26.04
