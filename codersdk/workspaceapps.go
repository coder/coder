package codersdk

import "github.com/google/uuid"

type WorkspaceApp struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Command string    `json:"command,omitempty"`
	Target  string    `json:"target,omitempty"`
	Icon    string    `json:"icon"`
}
