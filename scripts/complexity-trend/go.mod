// Separate module so the analysis tooling does not add dependencies to the
// main coder module.
module github.com/coder/coder/scripts/complexity-trend

go 1.25

require (
	github.com/fzipp/gocyclo v0.6.0
	github.com/uudashr/gocognit v1.3.0
)

require golang.org/x/tools v0.42.0 // indirect
