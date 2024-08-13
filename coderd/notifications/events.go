package notifications

import "github.com/google/uuid"

// These vars are mapped to UUIDs in the notification_templates table.
// TODO: autogenerate these: https://github.com/coder/team-coconut/issues/36

// Workspace-related events.
var (
	TemplateWorkspaceDeleted           = uuid.MustParse("f517da0b-cdc9-410f-ab89-a86107c420ed")
	TemplateWorkspaceAutobuildFailed   = uuid.MustParse("381df2a9-c0c0-4749-420f-80a9280c66f9")
	TemplateWorkspaceDormant           = uuid.MustParse("0ea69165-ec14-4314-91f1-69566ac3c5a0")
	TemplateWorkspaceAutoUpdated       = uuid.MustParse("c34a0c09-0704-4cac-bd1c-0c0146811c2b")
	TemplateWorkspaceMarkedForDeletion = uuid.MustParse("51ce2fdf-c9ca-4be1-8d70-628674f9bc42")
)

// Account-related events.
var (
	TemplateUserAccountCreated = uuid.MustParse("4e19c0ac-94e1-4532-9515-d1801aa283b2")
	TemplateUserAccountDeleted = uuid.MustParse("f44d9314-ad03-4bc8-95d0-5cad491da6b6")
)

// Template-related events.
var (
	TemplateTemplateDeleted = uuid.MustParse("29a09665-2a4c-403f-9648-54301670e7be")
)
