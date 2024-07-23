package notifications

import "github.com/google/uuid"

// These vars are mapped to UUIDs in the notification_templates table.
// TODO: autogenerate these.

// Workspace-related events.
var (
	TemplateWorkspaceDeleted           = uuid.MustParse("f517da0b-cdc9-410f-ab89-a86107c420ed")
	WorkspaceAutobuildFailed           = uuid.MustParse("381df2a9-c0c0-4749-420f-80a9280c66f9")
	TemplateWorkspaceDormant           = uuid.MustParse("0ea69165-ec14-4314-91f1-69566ac3c5a0")
	WorkspaceAutoUpdated               = uuid.MustParse("c34a0c09-0704-4cac-bd1c-0c0146811c2b")
	TemplateWorkspaceMarkedForDeletion = uuid.MustParse("51ce2fdf-c9ca-4be1-8d70-628674f9bc42")
)
