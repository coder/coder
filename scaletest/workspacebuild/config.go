package workspacebuild
import (
	"fmt"
	"errors"
	"github.com/google/uuid"
	"github.com/coder/coder/v2/codersdk"
)
type Config struct {
	// OrganizationID is the ID of the organization to create the workspace in.
	OrganizationID uuid.UUID `json:"organization_id"`
	// UserID is the ID of the user to run the test as.
	UserID string `json:"user_id"`
	// Request is the request to send to the Coder API to create the workspace.
	// request.template_id must be set. A name will be generated if not
	// specified.
	Request codersdk.CreateWorkspaceRequest `json:"request"`
	// NoWaitForAgents determines whether the test should wait for the workspace
	// agents to connect before returning.
	NoWaitForAgents bool `json:"no_wait_for_agents"`
	// Retry determines how many times to retry starting a workspace build if it
	// fails.
	Retry int `json:"retry"`
}
func (c Config) Validate() error {
	if c.OrganizationID == uuid.Nil {
		return errors.New("organization_id must be set")
	}
	if c.UserID == "" {
		return errors.New("user_id must be set")
	}
	if c.UserID != codersdk.Me {
		_, err := uuid.Parse(c.UserID)
		if err != nil {
			return fmt.Errorf("user_id must be %q or a valid UUID: %w", codersdk.Me, err)
		}
	}
	if c.Request.TemplateID == uuid.Nil {
		return errors.New("request.template_id must be set")
	}
	return nil
}
