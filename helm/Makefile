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

lint/helm:
	helm lint --strict --set coder.image.tag=v0.0.1 .
.PHONY: lint/helm
