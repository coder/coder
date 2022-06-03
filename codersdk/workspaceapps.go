package codersdk

import (
	"github.com/google/uuid"
)

type WorkspaceApp struct {
	ID uuid.UUID `json:"id"`
	// Name is a unique identifier attached to an agent.
	Name    string `json:"name"`
	Command string `json:"command,omitempty"`
	// Icon is a relative path or external URL that specifies
	// an icon to be displayed in the dashboard.
	Icon string `json:"icon"`
}
