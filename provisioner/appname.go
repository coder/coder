package provisioner

import "regexp"

var (
	// ValidAppNameRegex is the regex used to validate the name of a coder_app
	// resource. It must be a valid hostname and cannot contain two consecutive
	// hyphens or start/end with a hyphen.
	//
	// This regex looks complicated but it's written this way to avoid a
	// negative lookahead (which is not supported by Go). There are test cases
	// for this regex in appname_test.go.
	ValidAppNameRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-][a-z0-9]|[a-z0-9]([a-z0-9-]?[a-z0-9])?)*$`)
)
