package proto

import "github.com/coder/coder/v2/apiversion"

// Version history:
//
// API v1.2:
//   - Add support for `open_in` parameters in the workspace apps.
const (
	CurrentMajor = 1
	CurrentMinor = 2
)

// CurrentVersion is the current provisionerd API version.
// Breaking changes to the provisionerd API **MUST** increment
// CurrentMajor above.
var CurrentVersion = apiversion.New(CurrentMajor, CurrentMinor)
