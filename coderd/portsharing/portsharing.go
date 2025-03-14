package portsharing
import (
	"errors"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)
type PortSharer interface {
	AuthorizedLevel(template database.Template, level codersdk.WorkspaceAgentPortShareLevel) error
	ValidateTemplateMaxLevel(level codersdk.WorkspaceAgentPortShareLevel) error
	ConvertMaxLevel(level database.AppSharingLevel) codersdk.WorkspaceAgentPortShareLevel
}
type AGPLPortSharer struct{}
func (AGPLPortSharer) AuthorizedLevel(_ database.Template, _ codersdk.WorkspaceAgentPortShareLevel) error {
	return nil
}
func (AGPLPortSharer) ValidateTemplateMaxLevel(_ codersdk.WorkspaceAgentPortShareLevel) error {
	return errors.New("Restricting port sharing level is an enterprise feature that is not enabled.")
}
func (AGPLPortSharer) ConvertMaxLevel(_ database.AppSharingLevel) codersdk.WorkspaceAgentPortShareLevel {
	return codersdk.WorkspaceAgentPortShareLevelPublic
}
var DefaultPortSharer PortSharer = AGPLPortSharer{}
