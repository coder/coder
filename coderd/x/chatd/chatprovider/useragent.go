package chatprovider

import (
	"fmt"
	"runtime"

	"github.com/coder/coder/v2/buildinfo"
)

// UserAgent returns the User-Agent string sent on all outgoing LLM
// API requests made by Coder's built-in chat (chatd). The format
// mirrors conventions used by other coding agents so that LLM
// providers can identify traffic originating from Coder.
//
// Example: coder-agents/v2.21.0 (linux/amd64)
func UserAgent() string {
	return fmt.Sprintf("coder-agents/%s (%s/%s)",
		buildinfo.Version(), runtime.GOOS, runtime.GOARCH)
}
