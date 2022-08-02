.PHONY: docker-build docker-push

build_tag=codercom/oss-dogfood

docker-build:
	DOCKER_BUILDKIT=1 docker build . -t ${build_tag}

docker-push: docker-build
	docker push ${build_tag}
