# Makefile for building and pushing the Android dogfood image.
# See ../coder/Makefile for reference.

REGISTRY := codercom
IMAGE := oss-dogfood-android
TAG := latest

.PHONY: build push

build:
	docker build \
		--tag $(REGISTRY)/$(IMAGE):$(TAG) \
		.

push: build
	docker push $(REGISTRY)/$(IMAGE):$(TAG)
