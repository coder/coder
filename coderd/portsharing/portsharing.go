package portsharing

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

type PortSharer interface {
	AuthorizedPortSharingLevel(template database.Template, level codersdk.WorkspaceAgentPortShareLevel) error
	ValidateTemplateMaxPortSharingLevel(level codersdk.WorkspaceAgentPortShareLevel) error
	ConvertMaxPortSharingLevel(level database.AppSharingLevel) codersdk.WorkspaceAgentPortShareLevel
}

type AGPLPortSharer struct{}

func (AGPLPortSharer) AuthorizedPortSharingLevel(_ database.Template, _ codersdk.WorkspaceAgentPortShareLevel) error {
	return nil
}

func (AGPLPortSharer) ValidateTemplateMaxPortSharingLevel(_ codersdk.WorkspaceAgentPortShareLevel) error {
	return xerrors.New("Restricting port sharing level is an enterprise feature that is not enabled.")
}

func (AGPLPortSharer) ConvertMaxPortSharingLevel(_ database.AppSharingLevel) codersdk.WorkspaceAgentPortShareLevel {
	return codersdk.WorkspaceAgentPortShareLevelPublic
}

var DefaultPortSharer PortSharer = AGPLPortSharer{}
