package chattool

import (
	"context"
	"path"
	"strings"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// resolveHomePaths rewrites each path that is "~" or starts with "~/"
// against the workspace user's home directory, asking the agent for it at
// most once. The agent's file endpoints accept only absolute paths, while
// chat prompts name workspace attachments relative to the home directory.
func resolveHomePaths(ctx context.Context, conn workspacesdk.AgentConn, paths ...*string) error {
	var home string
	for _, p := range paths {
		if *p != "~" && !strings.HasPrefix(*p, "~/") {
			continue
		}
		if home == "" {
			var err error
			home, err = ResolveWorkspaceHome(ctx, conn)
			if err != nil {
				return err
			}
		}
		*p = path.Join(home, strings.TrimPrefix(*p, "~"))
	}
	return nil
}
