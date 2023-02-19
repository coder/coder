package provisioner

import "regexp"

// AppSlugRegex is the regex used to validate the slug of a coder_app
// resource. It must be a valid hostname and cannot contain two consecutive
// hyphens or start/end with a hyphen.
//
// This regex is duplicated in the terraform provider code, so make sure to
// update it there as well.
//
// There are test cases for this regex in appslug_test.go.
var AppSlugRegex = regexp.MustCompile(`^[a-z0-9](-?[a-z0-9])*$`)
