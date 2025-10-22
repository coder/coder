# Use a single bash shell for each job, and immediately exit on failure
SHELL := bash
.SHELLFLAGS = -ceu
.ONESHELL:

# This doesn't work on directories.
# See https://stackoverflow.com/questions/25752543/make-delete-on-error-for-directory-targets
.DELETE_ON_ERROR:

all: lint
.PHONY: all

lint: lint/helm
.PHONY: lint

lint/helm: lint/helm/coder lint/helm/provisioner
.PHONY: lint/helm

lint/helm/coder:
	helm dependency update --skip-refresh coder/
	helm lint --strict --set coder.image.tag=v0.0.1 coder/
.PHONY: lint/helm/coder

lint/helm/provisioner:
	helm dependency update --skip-refresh provisioner/
	helm lint --strict --set coder.image.tag=v0.0.1 provisioner/
.PHONY: lint/helm/provisioner
