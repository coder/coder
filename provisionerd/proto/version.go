package proto

import "github.com/coder/coder/v2/apiversion"

const (
	CurrentMajor = 1
	CurrentMinor = 1
)

// CurrentVersion is the current provisionerd API version.
// Breaking changes to the provisionerd API **MUST** increment
// CurrentMajor above.
var CurrentVersion = apiversion.New(CurrentMajor, CurrentMinor)
