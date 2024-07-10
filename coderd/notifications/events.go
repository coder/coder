package notifications

import "github.com/google/uuid"

// These vars are mapped to UUIDs in the notification_templates table.
// TODO: autogenerate these.

// Workspace-related events.
var TemplateWorkspaceDeleted = uuid.MustParse("f517da0b-cdc9-410f-ab89-a86107c420ed")
var TemplateWorkspaceDormant = uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
