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

func (EnterprisePortSharer) AuthorizedPortSharingLevel(template database.Template, level codersdk.WorkspaceAgentPortShareLevel) error {
	if level > codersdk.WorkspaceAgentPortShareLevel(template.MaxPortSharingLevel) {
		return xerrors.Errorf("port sharing level not allowed. Must not be greater than '%s'", template.MaxPortSharingLevel)
	}

	return nil
}

func (EnterprisePortSharer) ValidateTemplateMaxPortSharingLevel(level codersdk.WorkspaceAgentPortShareLevel) error {
	if !level.ValidMaxLevel() {
		return xerrors.New("invalid max port sharing level, value must be 'authenticated' or 'public'.")
	}

	return nil
}
