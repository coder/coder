package proto

import (
	"github.com/coder/coder/v2/apiversion"
)

const (
	CurrentMajor = 2
	CurrentMinor = 0
)

var CurrentVersion = apiversion.New(CurrentMajor, CurrentMinor).WithBackwardCompat(1)
