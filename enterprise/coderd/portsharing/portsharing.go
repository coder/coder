package portsharing

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

type EnterprisePortSharer struct{}

func NewEnterprisePortSharer() *EnterprisePortSharer {
	return &EnterprisePortSharer{}
}

func (EnterprisePortSharer) AuthorizedLevel(template database.Template, level codersdk.WorkspaceAgentPortShareLevel) error {
	maxLevel := codersdk.WorkspaceAgentPortShareLevel(template.MaxPortSharingLevel)
	return level.IsCompatibleWithMaxLevel(maxLevel)
}

func (EnterprisePortSharer) ValidateTemplateMaxLevel(level codersdk.WorkspaceAgentPortShareLevel) error {
	if !level.ValidMaxLevel() {
		return xerrors.New("invalid max port sharing level, value must be 'authenticated', 'organization', or 'public'.")
	}

	return nil
}

func (EnterprisePortSharer) ConvertMaxLevel(level database.AppSharingLevel) codersdk.WorkspaceAgentPortShareLevel {
	return codersdk.WorkspaceAgentPortShareLevel(level)
}
