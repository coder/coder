package notifications

import "github.com/google/uuid"

// These vars are mapped to UUIDs in the notification_templates table.
// TODO: autogenerate these.

// Workspace-related events.
var (
	TemplateWorkspaceDeleted = uuid.MustParse("f517da0b-cdc9-410f-ab89-a86107c420ed")
	WorkspaceAutobuildFailed = uuid.MustParse("381df2a9-c0c0-4749-420f-80a9280c66f9")
)
