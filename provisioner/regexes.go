package provisioner

import "regexp"

var (
	// AgentNameRegex is the regex used to validate the name of a coder_agent
	// resource. It must be a valid hostname and cannot contain two consecutive
	// hyphens or start/end with a hyphen. Uppercase characters ARE permitted,
	// although duplicate agent names with different casing will be rejected.
	//
	// Previously, underscores were permitted, but this was changed in 2025-02.
	// App URLs never supported underscores, and proxy requests to apps on
	// agents with underscores in the name always failed.
	//
	// Due to terraform limitations, this cannot be validated at the provider
	// level as resource names cannot be read from the provider API, so this is
	// not duplicated in the terraform provider code.
	//
	// There are test cases for this regex in regexes_test.go.
	AgentNameRegex = regexp.MustCompile(`(?i)^[a-z0-9](-?[a-z0-9])*$`)

	// AppSlugRegex is the regex used to validate the slug of a coder_app
	// resource. It must be a valid hostname and cannot contain two consecutive
	// hyphens or start/end with a hyphen.
	//
	// This regex is duplicated in the terraform provider code, so make sure to
	// update it there as well.
	//
	// There are test cases for this regex in regexes_test.go.
	AppSlugRegex = regexp.MustCompile(`^[a-z0-9](-?[a-z0-9])*$`)
)
