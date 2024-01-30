package portsharing

import (
	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

type PortSharer interface {
	ShareLevelAllowed(workspaceID uuid.UUID, level codersdk.WorkspacePortSharingLevel) bool
}

type AGPLPortSharer struct{}

func (AGPLPortSharer) ShareLevelAllowed(_ uuid.UUID, _ codersdk.WorkspacePortSharingLevel) bool {
	return true
}

var DefaultPortSharer PortSharer = AGPLPortSharer{}
