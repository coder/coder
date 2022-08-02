.PHONY: docker-build docker-push

branch=$(shell git rev-parse --abbrev-ref HEAD)
build_tag=codercom/oss-dogfood:${branch}

docker-build:
	DOCKER_BUILDKIT=1 docker build . -t ${build_tag}

docker-push: docker-build
	docker push ${build_tag}
