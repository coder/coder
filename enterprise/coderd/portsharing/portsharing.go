package portsharing

import (
	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

type EnterprisePortSharer struct{}

func NewEnterprisePortSharer() *EnterprisePortSharer {
	return &EnterprisePortSharer{}
}

func (EnterprisePortSharer) ShareLevelAllowed(_ uuid.UUID, _ codersdk.WorkspacePortSharingLevel) bool {
	return false
}
