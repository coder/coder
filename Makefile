fmt/prettier:
	@echo "--- prettier"
# Avoid writing files in CI to reduce file write activity
ifdef CI
	yarn run format:check
else
	yarn run format:write
endif
.PHONY: fmt/prettier

fmt: fmt/prettier
.PHONY: fmt