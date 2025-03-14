package portsharing
import (
	"fmt"
	"errors"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)
type EnterprisePortSharer struct{}
func NewEnterprisePortSharer() *EnterprisePortSharer {
	return &EnterprisePortSharer{}
}
func (EnterprisePortSharer) AuthorizedLevel(template database.Template, level codersdk.WorkspaceAgentPortShareLevel) error {
	max := codersdk.WorkspaceAgentPortShareLevel(template.MaxPortSharingLevel)
	switch level {
	case codersdk.WorkspaceAgentPortShareLevelPublic:
		if max != codersdk.WorkspaceAgentPortShareLevelPublic {
			return fmt.Errorf("port sharing level not allowed. Max level is '%s'", max)
		}
	case codersdk.WorkspaceAgentPortShareLevelAuthenticated:
		if max == codersdk.WorkspaceAgentPortShareLevelOwner {
			return fmt.Errorf("port sharing level not allowed. Max level is '%s'", max)
		}
	default:
		return errors.New("port sharing level is invalid.")
	}
	return nil
}
func (EnterprisePortSharer) ValidateTemplateMaxLevel(level codersdk.WorkspaceAgentPortShareLevel) error {
	if !level.ValidMaxLevel() {
		return errors.New("invalid max port sharing level, value must be 'authenticated' or 'public'.")
	}
	return nil
}
func (EnterprisePortSharer) ConvertMaxLevel(level database.AppSharingLevel) codersdk.WorkspaceAgentPortShareLevel {
	return codersdk.WorkspaceAgentPortShareLevel(level)
}
