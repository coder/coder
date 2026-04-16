package example

import (
	"context"

	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// Calls from _test.go files are never flagged. This mirrors the
// original ruleguard rule's `!m.File().Name.Matches(\`_test\.go$\`)`.
func testfileCallOK() {
	_ = dbauthz.AsSystemRestricted(context.Background())
	_ = dbauthz.AsNotifier(context.Background())
}
