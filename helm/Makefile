# Use a single bash shell for each job, and immediately exit on failure
SHELL := bash
.SHELLFLAGS = -ceu
.ONESHELL:

# This doesn't work on directories.
# See https://stackoverflow.com/questions/25752543/make-delete-on-error-for-directory-targets
.DELETE_ON_ERROR:

all: lint gen
.PHONY: all

lint: lint/helm
.PHONY: lint

lint/helm:
	helm lint --strict .
.PHONY: lint/helm

gen: README.md
.PHONY: gen

README.md: README.md.gotmpl values.yaml
	helm-docs --template-files=$<
	@printf "<!-- DO NOT EDIT. THIS IS GENERATED FROM README.md.gotmpl -->\n\n%s\n" "$$(cat README.md)" > README.md
.PHONY: README.md
