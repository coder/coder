.PHONY: docker-build docker-push

branch=$(shell git rev-parse --abbrev-ref HEAD)
build_tag=codercom/oss-dogfood:${branch}

build:
	DOCKER_BUILDKIT=1 docker build . -t ${build_tag}

push: build
	docker push ${build_tag}
